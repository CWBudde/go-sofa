package sofa

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// streamFIRFile is an FIR file with M measurements whose samples are
// distinct: 1000m + 10r + k/N.
func streamFIRFile(m, r, n int) *File {
	f := robustBase(DataTypeFIR, m, r, 1, n)
	f.ImpulseResponses = make([][][]float64, m)
	for i := range m {
		f.ImpulseResponses[i] = make([][]float64, r)
		for j := range r {
			row := make([]float64, n)
			for k := range row {
				row[k] = float64(1000*i+10*j) + float64(k)/float64(n)
			}
			f.ImpulseResponses[i][j] = row
		}
	}
	f.SamplingRate = []float64{48000}
	return f
}

// saveTemp saves f into a new temporary directory and returns its path.
func saveTemp(t testing.TB, f *File, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := f.Save(path); err != nil {
		t.Fatalf("Save %s: %v", name, err)
	}
	return path
}

// openLazy opens path with OpenLazy and closes it when the test ends.
func openLazy(t testing.TB, path string) *File {
	t.Helper()
	f, err := OpenLazy(path)
	if err != nil {
		t.Fatalf("OpenLazy %s: %v", path, err)
	}
	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return f
}

// sameBits reports whether a and b hold bit-identical values (so that NaN
// payloads compare equal too).
func sameBits(a, b [][]float64) error {
	if len(a) != len(b) {
		return fmt.Errorf("length %d, want %d", len(a), len(b))
	}
	for i := range a {
		if len(a[i]) != len(b[i]) {
			return fmt.Errorf("[%d] length %d, want %d", i, len(a[i]), len(b[i]))
		}
		for j := range a[i] {
			if math.Float64bits(a[i][j]) != math.Float64bits(b[i][j]) {
				return fmt.Errorf("[%d][%d] = %v, want %v", i, j, a[i][j], b[i][j])
			}
		}
	}
	return nil
}

// sameBits3 is sameBits for [R][E][N] values.
func sameBits3(a, b [][][]float64) error {
	if len(a) != len(b) {
		return fmt.Errorf("length %d, want %d", len(a), len(b))
	}
	for i := range a {
		if err := sameBits(a[i], b[i]); err != nil {
			return fmt.Errorf("[%d]%w", i, err)
		}
	}
	return nil
}

// checkLazyMatchesEager compares every measurement read through OpenLazy
// with the eagerly opened file, for any DataType.
func checkLazyMatchesEager(t *testing.T, path string) {
	t.Helper()
	eager, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	lazy := openLazy(t, path)
	if lazy.M != eager.M || lazy.R != eager.R || lazy.E != eager.E || lazy.N != eager.N {
		t.Fatalf("lazy dims M=%d R=%d E=%d N=%d, eager M=%d R=%d E=%d N=%d",
			lazy.M, lazy.R, lazy.E, lazy.N, eager.M, eager.R, eager.E, eager.N)
	}
	if lazy.hasAudio() {
		t.Fatalf("OpenLazy loaded the %s audio data", lazy.DataType)
	}
	if len(lazy.SourcePositions) != len(eager.SourcePositions) ||
		len(lazy.SamplingRate) != len(eager.SamplingRate) ||
		len(lazy.Frequencies) != len(eager.Frequencies) {
		t.Errorf("lazy metadata differs from eager")
	}

	for m := range eager.M {
		var err error
		switch eager.DataType {
		case DataTypeFIR:
			var got, want [][]float64
			if got, err = lazy.ReadMeasurement(m); err == nil {
				if want, err = eager.ReadMeasurement(m); err == nil {
					err = errors.Join(sameBits(got, want), sameBits(want, eager.ImpulseResponses[m]))
				}
			}
		case DataTypeSOS:
			var got, want [][]float64
			if got, err = lazy.ReadSOSMeasurement(m); err == nil {
				if want, err = eager.ReadSOSMeasurement(m); err == nil {
					err = errors.Join(sameBits(got, want), sameBits(want, eager.SOSCoefficients[m]))
				}
			}
		case DataTypeTF:
			var re, im, wantRe, wantIm [][]float64
			if re, im, err = lazy.ReadTFMeasurement(m); err == nil {
				if wantRe, wantIm, err = eager.ReadTFMeasurement(m); err == nil {
					err = errors.Join(sameBits(re, wantRe), sameBits(im, wantIm),
						sameBits(wantRe, eager.TFReal[m]), sameBits(wantIm, eager.TFImag[m]))
				}
			}
		case DataTypeTFE:
			var re, im, wantRe, wantIm [][][]float64
			if re, im, err = lazy.ReadTFEMeasurement(m); err == nil {
				if wantRe, wantIm, err = eager.ReadTFEMeasurement(m); err == nil {
					err = errors.Join(sameBits3(re, wantRe), sameBits3(im, wantIm),
						sameBits3(wantRe, eager.TFRealE[m]), sameBits3(wantIm, eager.TFImagE[m]))
				}
			}
		}
		if err != nil {
			t.Fatalf("measurement %d: %v", m, err)
		}
	}
}

func TestReadMeasurementMatchesEager(t *testing.T) {
	synthetic := map[string]*File{
		"FIR": streamFIRFile(7, 2, 16),
		"TF":  robustTFFile(),
		"TF-E": func() *File {
			f := robustTFEFile()
			f.TFRealE[0][1][0][2] = 42 // distinct values per emitter
			return f
		}(),
		"SOS": robustSOSFile(),
		"FIR R=1": func() *File {
			f := streamFIRFile(3, 1, 5)
			f.ReceiverPositions = make([]Vector3, 1)
			return f
		}(),
	}
	for name, f := range synthetic {
		t.Run(name, func(t *testing.T) {
			checkLazyMatchesEager(t, saveTemp(t, f, "stream.sofa"))
		})
	}
	// TF-E stored [M,R,E,N], as older go-sofa versions wrote it.
	t.Run("TF-E MREN", func(t *testing.T) {
		checkLazyMatchesEager(t, writeCraftedSpec(t, tfeSpec(3, 4, layoutMREN, true)))
	})
	t.Run("TF-E MRNE crafted", func(t *testing.T) {
		checkLazyMatchesEager(t, writeCraftedSpec(t, tfeSpec(3, 4, layoutMRNE, false)))
	})
	for _, name := range []string{
		"CIPIC_subject_003_hrir_final.sofa",
		"MIT_KEMAR_normal_pinna.sofa",
		"Mesh2HRTF_HRTF_FourPointHorPlane_r100cm.sofa",
		"tester.sofa",
		"FreeFieldHRTF_1.0.sofa",
		"SimpleFreeFieldHRSOS_1.0.sofa",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			checkLazyMatchesEager(t, testdataPath(t, name))
		})
	}
}

func TestReadMeasurementErrors(t *testing.T) {
	fir := saveTemp(t, streamFIRFile(3, 2, 4), "fir.sofa")
	for _, tc := range []struct {
		name string
		open func(string) (*File, error)
	}{{"eager", Open}, {"lazy", OpenLazy}} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := tc.open(fir)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer f.Close()
			for _, m := range []int{-1, 3} {
				if _, err := f.ReadMeasurement(m); !errors.Is(err, ErrIndexOutOfRange) {
					t.Errorf("ReadMeasurement(%d): %v, want ErrIndexOutOfRange", m, err)
				}
			}
			if _, _, err := f.ReadTFMeasurement(0); !errors.Is(err, ErrUnsupportedDataType) {
				t.Errorf("ReadTFMeasurement on FIR: %v, want ErrUnsupportedDataType", err)
			}
			if _, _, err := f.ReadTFEMeasurement(0); !errors.Is(err, ErrUnsupportedDataType) {
				t.Errorf("ReadTFEMeasurement on FIR: %v, want ErrUnsupportedDataType", err)
			}
			if _, err := f.ReadSOSMeasurement(0); !errors.Is(err, ErrUnsupportedDataType) {
				t.Errorf("ReadSOSMeasurement on FIR: %v, want ErrUnsupportedDataType", err)
			}
			if err := f.RangeTFMeasurements(func(int, [][]float64, [][]float64) error { return nil }); !errors.Is(err, ErrUnsupportedDataType) {
				t.Errorf("RangeTFMeasurements on FIR: %v, want ErrUnsupportedDataType", err)
			}
		})
	}

	tf := saveTemp(t, robustTFFile(), "tf.sofa")
	f := openLazy(t, tf)
	if _, err := f.ReadMeasurement(0); !errors.Is(err, ErrUnsupportedDataType) {
		t.Errorf("ReadMeasurement on TF: %v, want ErrUnsupportedDataType", err)
	}
	if err := f.RangeMeasurements(func(int, [][]float64) error { return nil }); !errors.Is(err, ErrUnsupportedDataType) {
		t.Errorf("RangeMeasurements on TF: %v, want ErrUnsupportedDataType", err)
	}
	if _, _, err := f.ReadTFMeasurement(2); !errors.Is(err, ErrIndexOutOfRange) {
		t.Errorf("ReadTFMeasurement(2) with M=2: %v, want ErrIndexOutOfRange", err)
	}

	// A File built in memory without data for every measurement.
	short := streamFIRFile(3, 2, 4)
	short.ImpulseResponses = short.ImpulseResponses[:1]
	if _, err := short.ReadMeasurement(2); !errors.Is(err, ErrIndexOutOfRange) {
		t.Errorf("ReadMeasurement past stored data: %v, want ErrIndexOutOfRange", err)
	}
	tfe := robustTFEFile()
	tfe.TFRealE = nil
	if _, _, err := tfe.ReadTFEMeasurement(0); !errors.Is(err, ErrIndexOutOfRange) {
		t.Errorf("ReadTFEMeasurement without data: %v, want ErrIndexOutOfRange", err)
	}
}

func TestOpenLazyNeedsExplicitReads(t *testing.T) {
	f := openLazy(t, saveTemp(t, streamFIRFile(3, 2, 4), "fir.sofa"))
	if f.ImpulseResponses != nil {
		t.Fatalf("OpenLazy filled ImpulseResponses")
	}
	if _, err := f.IRAt(0, 0); !errors.Is(err, ErrNotLoaded) {
		t.Errorf("IRAt on lazy file: %v, want ErrNotLoaded", err)
	}
	if _, err := f.IRPeakdB(0, 0); !errors.Is(err, ErrNotLoaded) {
		t.Errorf("IRPeakdB on lazy file: %v, want ErrNotLoaded", err)
	}
	if _, err := f.IRAt(3, 0); !errors.Is(err, ErrIndexOutOfRange) {
		t.Errorf("IRAt(3, 0): %v, want ErrIndexOutOfRange", err)
	}
	out := filepath.Join(t.TempDir(), "out.sofa")
	if err := f.Save(out); !errors.Is(err, ErrNotLoaded) {
		t.Errorf("Save of lazy file: %v, want ErrNotLoaded", err)
	}

	// Filling the audio field in memory makes the File savable again.
	f.ImpulseResponses = streamFIRFile(3, 2, 4).ImpulseResponses
	if err := f.Save(out); err != nil {
		t.Errorf("Save of lazy file with loaded data: %v", err)
	}
	g, err := Open(out)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := sameBits(g.ImpulseResponses[2], f.ImpulseResponses[2]); err != nil {
		t.Errorf("round trip: %v", err)
	}
	for _, dt := range []string{DataTypeTF, DataTypeTFE, DataTypeSOS, "bogus"} {
		if (&File{DataType: dt}).hasAudio() {
			t.Errorf("hasAudio of empty %s file", dt)
		}
	}
}

func TestOpenLazyClose(t *testing.T) {
	path := saveTemp(t, streamFIRFile(3, 2, 4), "fir.sofa")
	var before int
	checkFDs := runtime.GOOS != "windows"
	if checkFDs {
		before = openFDs(t)
	}
	f, err := OpenLazy(path)
	if err != nil {
		t.Fatalf("OpenLazy: %v", err)
	}
	if checkFDs && openFDs(t) != before+1 {
		t.Errorf("OpenLazy should hold one descriptor: %d before, %d after", before, openFDs(t))
	}
	if _, err := f.ReadMeasurement(1); err != nil {
		t.Fatalf("ReadMeasurement: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if checkFDs && openFDs(t) != before {
		t.Errorf("Close left descriptors open: %d before, %d after", before, openFDs(t))
	}
	if err := f.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if _, err := f.ReadMeasurement(1); !errors.Is(err, ErrClosed) {
		t.Errorf("ReadMeasurement after Close: %v, want ErrClosed", err)
	}
	if err := f.RangeMeasurements(func(int, [][]float64) error { return nil }); !errors.Is(err, ErrClosed) {
		t.Errorf("RangeMeasurements after Close: %v, want ErrClosed", err)
	}
	// Metadata stays usable.
	if sr, err := f.SamplingRateScalar(); err != nil || sr != 48000 {
		t.Errorf("SamplingRateScalar after Close = %v, %v", sr, err)
	}
}

// TestOpenLazyFailureReleasesHandle checks that a failed OpenLazy closes
// the file again.
func TestOpenLazyFailureReleasesHandle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no /dev/fd on Windows")
	}
	// Data.IR [2,2,3] does not match N=4.
	bad := writeCraftedFIR(t, map[string]craftedDim{
		"M": {value: 2}, "R": {value: 2}, "E": {value: 1}, "N": {value: 4},
	}, []uint64{2, 2, 3})
	for _, path := range []string{bad, filepath.Join(t.TempDir(), "missing.sofa")} {
		before := openFDs(t)
		if f, err := OpenLazy(path); err == nil {
			_ = f.Close()
			t.Fatalf("OpenLazy(%s) succeeded", path)
		}
		if after := openFDs(t); after != before {
			t.Errorf("failed OpenLazy(%s) left descriptors open: %d before, %d after", path, before, after)
		}
	}
}

func TestRangeMeasurementsAbort(t *testing.T) {
	path := saveTemp(t, streamFIRFile(5, 2, 4), "fir.sofa")
	stop := errors.New("stop")
	for _, tc := range []struct {
		name string
		open func(string) (*File, error)
	}{{"eager", Open}, {"lazy", OpenLazy}} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := tc.open(path)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer f.Close()

			var seen []int
			err = f.RangeMeasurements(func(m int, ir [][]float64) error {
				seen = append(seen, m)
				if ir[1][0] != float64(1000*m+10) {
					t.Errorf("measurement %d: ir[1][0] = %v", m, ir[1][0])
				}
				if m == 2 {
					return stop
				}
				return nil
			})
			if err != stop { //nolint:errorlint // the callback's error must come back unwrapped
				t.Errorf("RangeMeasurements = %v, want the callback's error", err)
			}
			if len(seen) != 3 {
				t.Errorf("callback saw %v, want [0 1 2]", seen)
			}

			seen = seen[:0]
			if err := f.RangeMeasurements(func(m int, _ [][]float64) error {
				seen = append(seen, m)
				return nil
			}); err != nil || len(seen) != f.M {
				t.Errorf("full range: %v after %v", err, seen)
			}
		})
	}

	tf := openLazy(t, saveTemp(t, robustTFFile(), "tf.sofa"))
	calls := 0
	err := tf.RangeTFMeasurements(func(m int, re, im [][]float64) error {
		calls++
		if re[0][1] != 0.5*float64(100*m+1) || im[0][1] != -0.25*float64(100*m+1) {
			t.Errorf("measurement %d: re %v im %v", m, re[0][1], im[0][1])
		}
		return stop
	})
	if !errors.Is(err, stop) || calls != 1 {
		t.Errorf("RangeTFMeasurements = %v after %d calls, want stop after 1", err, calls)
	}
	calls = 0
	if err := tf.RangeTFMeasurements(func(int, [][]float64, [][]float64) error { calls++; return nil }); err != nil || calls != tf.M {
		t.Errorf("full TF range: %v after %d calls", err, calls)
	}
	if err := tf.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := tf.RangeTFMeasurements(func(int, [][]float64, [][]float64) error { return nil }); !errors.Is(err, ErrClosed) {
		t.Errorf("RangeTFMeasurements after Close: %v, want ErrClosed", err)
	}
}

// TestReadMeasurementConcurrent reads a lazy File from several goroutines
// (run with -race).
func TestReadMeasurementConcurrent(t *testing.T) {
	src := streamFIRFile(16, 2, 8)
	f := openLazy(t, saveTemp(t, src, "fir.sofa"))
	var wg sync.WaitGroup
	errs := make(chan error, f.M)
	for m := range f.M {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ir, err := f.ReadMeasurement(m)
			if err == nil {
				err = sameBits(ir, src.ImpulseResponses[m])
			}
			if err != nil {
				errs <- fmt.Errorf("measurement %d: %w", m, err)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// TestOpenLazyDoesNotAllocateAudio opens a file of more than 10 MB of
// impulse responses both ways and checks that OpenLazy allocates less than
// half of what Open does.
func TestOpenLazyDoesNotAllocateAudio(t *testing.T) {
	const m, r, n = 700, 2, 1024 // 700·2·1024·8 B ≈ 11.5 MB of Data.IR
	path := saveTemp(t, streamFIRFile(m, r, n), "large.sofa")

	allocated := func(open func(string) (*File, error)) (uint64, *File) {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		f, err := open(path)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		runtime.ReadMemStats(&after)
		return after.TotalAlloc - before.TotalAlloc, f
	}
	eagerBytes, eager := allocated(Open)
	lazyBytes, lazy := allocated(OpenLazy)
	defer lazy.Close()

	if len(lazy.ImpulseResponses) != 0 {
		t.Errorf("OpenLazy loaded %d measurements", len(lazy.ImpulseResponses))
	}
	if len(eager.ImpulseResponses) != m {
		t.Fatalf("Open loaded %d measurements, want %d", len(eager.ImpulseResponses), m)
	}
	t.Logf("allocated: Open %d B, OpenLazy %d B", eagerBytes, lazyBytes)
	if eagerBytes < m*r*n*8 {
		t.Errorf("Open allocated %d B, less than the %d B of data", eagerBytes, m*r*n*8)
	}
	if lazyBytes*2 > eagerBytes {
		t.Errorf("OpenLazy allocated %d B, not below half of Open's %d B", lazyBytes, eagerBytes)
	}
}

// TestReadMeasurementChunkCache reads a chunked fixture (chunks of 355
// measurements) out of order, across and back over chunk boundaries.
func TestReadMeasurementChunkCache(t *testing.T) {
	t.Parallel()
	path := testdataPath(t, "MIT_KEMAR_normal_pinna.sofa")
	eager, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	lazy := openLazy(t, path)
	if v := lazy.lazy.vars[varIR]; v.block != 355 {
		t.Errorf("Data.IR read block = %d, want the chunk size 355", v.block)
	}
	for _, m := range []int{709, 0, 354, 355, 354, 709} {
		ir, err := lazy.ReadMeasurement(m)
		if err != nil {
			t.Fatalf("ReadMeasurement(%d): %v", m, err)
		}
		if err := sameBits(ir, eager.ImpulseResponses[m]); err != nil {
			t.Errorf("measurement %d: %v", m, err)
		}
		ir[0][0] = math.NaN() // must not reach the cache
	}
	ir, err := lazy.ReadMeasurement(709)
	if err != nil || math.IsNaN(ir[0][0]) {
		t.Errorf("ReadMeasurement(709) after caller write = %v, %v", ir[0][0], err)
	}
}

func TestParseChunkShape(t *testing.T) {
	for _, tc := range []struct {
		info string
		want []uint64
	}{
		{"Dataset: float (size=8 bytes), 3D array [710 2 512], chunked (chunks=[355 1 256 8])", []uint64{355, 1, 256}},
		{"Dataset: float (size=8 bytes), 3D array [710 2 512], contiguous", nil},
		{"chunked (chunks=[8])", nil},
		{"chunked (chunks=[0 8])", nil},
	} {
		got, ok := parseChunkShape(tc.info)
		if ok != (tc.want != nil) || fmt.Sprint(got) != fmt.Sprint(tc.want) {
			t.Errorf("parseChunkShape(%q) = %v, %v; want %v", tc.info, got, ok, tc.want)
		}
	}
}
