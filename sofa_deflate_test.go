package sofa

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	hdf5 "github.com/cwbudde/go-hdf5"
)

// dataChunkShape returns the chunk shape of the root dataset name in the
// HDF5 file at path, and whether it is chunked.
func dataChunkShape(t *testing.T, path, name string) ([]uint64, bool) {
	t.Helper()
	h, err := hdf5.Open(path)
	if err != nil {
		t.Fatalf("hdf5.Open: %v", err)
	}
	defer h.Close()
	for _, child := range h.Root().Children() {
		if ds, ok := child.(*hdf5.Dataset); ok && ds.Name() == name {
			shape, chunked, err := ds.ChunkShape()
			if err != nil {
				t.Fatalf("%s: ChunkShape: %v", name, err)
			}
			return shape, chunked
		}
	}
	t.Fatalf("%s: no dataset %s", path, name)
	return nil, false
}

// TestSaveDeflateRoundTrip saves every DataType with WithDeflate and checks
// that the audio data reads back unchanged, stored in chunks of one
// receiver across all measurements.
func TestSaveDeflateRoundTrip(t *testing.T) {
	audio := func(f *File) []any {
		return []any{f.ImpulseResponses, f.TFReal, f.TFImag, f.TFRealE, f.TFImagE, f.SOSCoefficients}
	}
	dataset := map[string]string{"FIR": "Data.IR", "TF": "Data.Real", "TF-E": "Data.Real", "SOS": "Data.SOS"}
	for _, f := range saveFixtures() {
		t.Run(f.DataType, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "deflate.sofa")
			if err := f.Save(path, WithDeflate(4)); err != nil {
				t.Fatalf("Save: %v", err)
			}
			g, err := Open(path)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer g.Close()
			if !reflect.DeepEqual(audio(g), audio(f)) {
				t.Error("audio data differs after a deflated round trip")
			}

			chunk, chunked := dataChunkShape(t, path, dataset[f.DataType])
			want := []uint64{uint64(f.M), 1, uint64(f.N)} //nolint:gosec // small fixture sizes
			if f.DataType == DataTypeTFE {
				want = append(want, uint64(f.E)) //nolint:gosec // small fixture size
			}
			if !chunked || !slices.Equal(chunk, want) {
				t.Errorf("chunk shape = %v (chunked %v), want %v", chunk, chunked, want)
			}
		})
	}
}

// TestSaveDefaultDeflate checks that Save without options and WriteTo
// write the bytes of WithDeflate(DefaultDeflateLevel), and that
// WithDeflate(0) stores the data contiguous.
func TestSaveDefaultDeflate(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1790000000")
	dir := t.TempDir()
	f := robustFIRFile()
	save := func(name string, opts ...SaveOption) []byte {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := f.Save(path, opts...); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	plain := save("plain.sofa")
	if !slices.Equal(plain, save("level.sofa", WithDeflate(DefaultDeflateLevel))) {
		t.Error("Save without options differs from WithDeflate(DefaultDeflateLevel)")
	}
	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(plain, buf.Bytes()) {
		t.Error("WriteTo differs from Save without options")
	}
	if _, chunked := dataChunkShape(t, filepath.Join(dir, "plain.sofa"), "Data.IR"); !chunked {
		t.Error("Data.IR is not chunked by default")
	}
	save("zero.sofa", WithDeflate(0))
	if _, chunked := dataChunkShape(t, filepath.Join(dir, "zero.sofa"), "Data.IR"); chunked {
		t.Error("Data.IR is chunked with WithDeflate(0)")
	}
}

// TestDataChunk checks the chunk shapes of deflated audio data: one
// receiver across the measurements that fit in 4 MiB, but never more than
// 64 chunks, which libmysofa cannot index, and receivers grouped before
// chunks grow along M, so a lazy read of a measurement fits its cache.
func TestDataChunk(t *testing.T) {
	for _, tc := range []struct {
		name         string
		shape, chunk []uint64
	}{
		{"KEMAR", []uint64{710, 2, 512}, []uint64{710, 1, 512}},
		{"4 MiB cap", []uint64{1100, 2, 512}, []uint64{1024, 1, 512}},
		{"long measurements", []uint64{3, 2, 280000}, []uint64{1, 1, 280000}},
		{"TF-E", []uint64{4, 2, 3, 5}, []uint64{4, 1, 3, 5}},
		// 40 blocks of 1024 measurements: both receivers share a chunk.
		{"many measurements", []uint64{40000, 2, 512}, []uint64{1024, 2, 512}},
		{"64 receivers", []uint64{100, 64, 4096}, []uint64{100, 1, 4096}},
		{"many receivers", []uint64{100, 130, 1024}, []uint64{100, 3, 1024}},
		// 10 blocks of 512 measurements, 6 receiver chunks each.
		{"many receivers, many measurements", []uint64{5000, 40, 1024}, []uint64{512, 7, 1024}},
		// 100 blocks of 1024 even with all receivers in one chunk.
		{"huge", []uint64{102400, 2, 512}, []uint64{1600, 2, 512}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := dataChunk(tc.shape)
			if !slices.Equal(got, tc.chunk) {
				t.Errorf("dataChunk(%v) = %v, want %v", tc.shape, got, tc.chunk)
			}
			chunks := uint64(1)
			for i := range 2 {
				chunks *= (tc.shape[i] + got[i] - 1) / got[i]
			}
			if chunks > maxDataChunks {
				t.Errorf("%d chunks, more than %d", chunks, maxDataChunks)
			}
			// The chunks one measurement spans, all a lazy read caches.
			span := got[0] * (tc.shape[1] + got[1] - 1) / got[1] * got[1] * 8
			for _, n := range tc.shape[2:] {
				span *= n
			}
			if span > maxChunkCacheBytes {
				t.Errorf("one measurement spans %d MiB of chunks, more than the %d MiB lazy-read cache",
					span>>20, maxChunkCacheBytes>>20)
			}
		})
	}
}

// TestSaveDeflateLevelOutOfRange checks that an invalid level fails Save
// before anything is written.
func TestSaveDeflateLevelOutOfRange(t *testing.T) {
	for _, level := range []int{-1, 10} {
		dir := t.TempDir()
		if err := robustFIRFile().Save(filepath.Join(dir, "out.sofa"), WithDeflate(level)); err == nil {
			t.Errorf("WithDeflate(%d): Save succeeded", level)
		}
		assertOnlyFile(t, dir)
	}
}

// TestSaveDeflateChunkCap checks that chunks of one receiver hold only as
// many measurements as fit in maxDataChunkBytes, and at least one.
func TestSaveDeflateChunkCap(t *testing.T) {
	for _, tc := range []struct {
		name string
		m, n int
		want uint64 // measurements per chunk
	}{
		// 4 KiB per measurement, 4.3 MiB per receiver.
		{"many measurements", 1100, 512, 1024},
		// 2.1 MiB per measurement: two would exceed 4 MiB.
		{"long measurements", 3, 280000, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := robustBase("FIR", tc.m, 2, 1, tc.n)
			f.ImpulseResponses = ramp3D(tc.m, 2, tc.n, 0.001)
			f.SamplingRate = []float64{48000}
			f.Delay = []float64{0}
			path := filepath.Join(t.TempDir(), "large.sofa")
			if err := f.Save(path, WithDeflate(1)); err != nil {
				t.Fatalf("Save: %v", err)
			}
			want := []uint64{tc.want, 1, uint64(tc.n)} //nolint:gosec // small test size
			if chunk, _ := dataChunkShape(t, path, "Data.IR"); !slices.Equal(chunk, want) {
				t.Errorf("chunk shape = %v, want %v", chunk, want)
			}
		})
	}
}

// TestSaveDeflateKEMAR checks that a deflated re-save of MIT KEMAR is no
// larger than the netCDF-C original and holds the same impulse responses.
func TestSaveDeflateKEMAR(t *testing.T) {
	src := testdataPath(t, "MIT_KEMAR_normal_pinna.sofa")
	f, err := Open(src)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	path := filepath.Join(t.TempDir(), "kemar.sofa")
	if err := f.Save(path, WithDeflate(4)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	orig, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("original %d bytes, deflated re-save %d bytes", orig.Size(), saved.Size())
	if saved.Size() > orig.Size() {
		t.Errorf("deflated re-save is %d bytes, larger than the original's %d", saved.Size(), orig.Size())
	}
	g, err := Open(path)
	if err != nil {
		t.Fatalf("Open re-save: %v", err)
	}
	defer g.Close()
	if !reflect.DeepEqual(g.ImpulseResponses, f.ImpulseResponses) {
		t.Error("impulse responses differ after the deflated re-save")
	}
}
