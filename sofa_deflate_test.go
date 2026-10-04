package sofa

import (
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

// TestSaveDeflateZeroIsUncompressed checks that WithDeflate(0) writes the
// same bytes as Save without options.
func TestSaveDeflateZeroIsUncompressed(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1790000000")
	dir := t.TempDir()
	f := robustFIRFile()
	plain, zero := filepath.Join(dir, "plain.sofa"), filepath.Join(dir, "zero.sofa")
	if err := f.Save(plain); err != nil {
		t.Fatal(err)
	}
	if err := f.Save(zero, WithDeflate(0)); err != nil {
		t.Fatal(err)
	}
	a, err := os.ReadFile(plain)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(zero)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a, b) {
		t.Error("WithDeflate(0) output differs from Save without options")
	}
	if _, chunked := dataChunkShape(t, zero, "Data.IR"); chunked {
		t.Error("Data.IR is chunked without deflate")
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
