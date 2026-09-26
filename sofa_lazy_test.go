package sofa

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/cwbudde/go-hdf5"
)

// syntheticFIRFile returns a FIR File of m×r×n samples whose values encode
// their position, so that a misplaced sample is detected.
func syntheticFIRFile(m, r, n int) *File {
	f := minimalFIRFile()
	f.M, f.R, f.N = m, r, n
	f.ImpulseResponses = make([][][]float64, m)
	f.SourcePositions = make([]Vector3, m)
	for i := range m {
		f.ImpulseResponses[i] = make([][]float64, r)
		for j := range r {
			ir := make([]float64, n)
			for k := range ir {
				ir[k] = float64(i) + float64(j)/10 + float64(k)/1e6
			}
			f.ImpulseResponses[i][j] = ir
		}
		f.SourcePositions[i] = Vector3{float64(i % 360), 0, 1}
	}
	f.ReceiverPositions = make([]Vector3, r)
	f.Delay = []float64{0}
	return f
}

// saveSynthetic writes syntheticFIRFile(m, 2, n) to a temporary file and
// returns its path.
func saveSynthetic(t *testing.T, m, n int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic.sofa")
	if err := syntheticFIRFile(m, 2, n).Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return path
}

// allocatedBy returns the bytes allocated while fn runs.
func allocatedBy(fn func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// TestOpenLazyDoesNotAllocateAudio opens a file larger than 10 MB lazily and
// checks that the impulse responses are not loaded: OpenLazy allocates at
// most half of what Open does.
func TestOpenLazyDoesNotAllocateAudio(t *testing.T) {
	path := saveSynthetic(t, 700, 1024)
	if st, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if st.Size() <= 10<<20 {
		t.Fatalf("synthetic file is %d bytes, want > 10 MB", st.Size())
	}

	var eager, lazy *File
	var eagerErr, lazyErr error
	eagerBytes := allocatedBy(func() { eager, eagerErr = Open(path) })
	if eagerErr != nil {
		t.Fatalf("Open: %v", eagerErr)
	}
	lazyBytes := allocatedBy(func() { lazy, lazyErr = OpenLazy(path) })
	if lazyErr != nil {
		t.Fatalf("OpenLazy: %v", lazyErr)
	}
	t.Cleanup(func() { _ = lazy.Close() })

	if len(lazy.ImpulseResponses) != 0 {
		t.Errorf("OpenLazy loaded %d measurements, want none", len(lazy.ImpulseResponses))
	}
	if lazy.M != eager.M || lazy.R != eager.R || lazy.N != eager.N {
		t.Errorf("OpenLazy dimensions M=%d R=%d N=%d, Open M=%d R=%d N=%d",
			lazy.M, lazy.R, lazy.N, eager.M, eager.R, eager.N)
	}
	if len(lazy.SourcePositions) != eager.M {
		t.Errorf("OpenLazy read %d source positions, want %d", len(lazy.SourcePositions), eager.M)
	}
	if lazyBytes*2 > eagerBytes {
		t.Errorf("OpenLazy allocated %d bytes, Open %d: want at most half", lazyBytes, eagerBytes)
	}
	t.Logf("allocated: Open %d bytes, OpenLazy %d bytes", eagerBytes, lazyBytes)
}

// TestOpenLazyCloseReleasesHandle checks that a lazy File holds the file
// open until Close, and that Close is idempotent.
func TestOpenLazyCloseReleasesHandle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no /dev/fd on Windows")
	}
	path := saveSynthetic(t, 3, 4)

	before := openFDs(t)
	f, err := OpenLazy(path)
	if err != nil {
		t.Fatalf("OpenLazy: %v", err)
	}
	if during := openFDs(t); during <= before {
		t.Errorf("open descriptors: %d before OpenLazy, %d after; want the file held open", before, during)
	}
	if err := f.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if after := openFDs(t); after != before {
		t.Errorf("open descriptors: %d before OpenLazy, %d after Close", before, after)
	}
	if err := f.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// TestOpenLazyRejects checks that OpenLazy fails like Open on a non-SOFA
// file and does not keep it open.
func TestOpenLazyRejects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not_sofa.h5")
	fw, err := hdf5.CreateForWrite(path, hdf5.CreateTruncate,
		hdf5.WithRootAttribute("Conventions", "NotSOFA"))
	if err != nil {
		t.Fatalf("CreateForWrite: %v", err)
	}
	if err := fw.Close(); err != nil {
		t.Fatalf("Close writer: %v", err)
	}

	before := -1
	if runtime.GOOS != "windows" {
		before = openFDs(t)
	}
	f, err := OpenLazy(path)
	if !errors.Is(err, ErrNotSOFA) {
		t.Fatalf("OpenLazy error %v, want ErrNotSOFA", err)
	}
	if f != nil {
		t.Errorf("OpenLazy returned a File together with an error")
	}
	if before >= 0 {
		if after := openFDs(t); after != before {
			t.Errorf("open descriptors: %d before OpenLazy, %d after a failed OpenLazy", before, after)
		}
	}
}

// TestOpenLazySaveFails checks that a lazily opened File, whose audio is
// not loaded, cannot be saved.
func TestOpenLazySaveFails(t *testing.T) {
	f, err := OpenLazy(saveSynthetic(t, 3, 4))
	if err != nil {
		t.Fatalf("OpenLazy: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	err = f.Save(filepath.Join(t.TempDir(), "out.sofa"))
	if ve := requireValidationError(t, err); ve.Field != "ImpulseResponses" {
		t.Errorf("ValidationError.Field = %q, want ImpulseResponses", ve.Field)
	}
}

// ciFixtures are the reference files CI fetches (see AGENTS.md).
var ciFixtures = []string{
	"CIPIC_subject_003_hrir_final.sofa",
	"MIT_KEMAR_normal_pinna.sofa",
	"Mesh2HRTF.sofa",
	"Mesh2HRTF_HRTF_FourPointHorPlane_r100cm.sofa",
	"tester.sofa",
}

// TestOpenLazyMatchesOpen checks that OpenLazy accepts every CI fixture and
// fills the same fields as Open apart from the audio data.
func TestOpenLazyMatchesOpen(t *testing.T) {
	for _, name := range ciFixtures {
		t.Run(name, func(t *testing.T) {
			path := testdataPath(t, name)
			eager, err := Open(path)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			lazy, err := OpenLazy(path)
			if err != nil {
				t.Fatalf("OpenLazy: %v", err)
			}
			if err := lazy.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			eager.ImpulseResponses, eager.TFReal, eager.TFImag = nil, nil, nil
			eager.TFRealE, eager.TFImagE, eager.SOSCoefficients = nil, nil, nil
			lazy.lazy = false
			if !reflect.DeepEqual(lazy, eager) {
				t.Errorf("OpenLazy and Open differ beyond the audio data")
			}
		})
	}
}

// sampleMeasurements returns every measurement index of a small file and,
// for a larger one, the first, middle and last index and both sides of the
// chunk boundary of MIT_KEMAR's Data.IR (355). The CI fixtures store Data.IR
// in chunks spanning all measurements, so each lazy read decompresses them
// whole and reading every index would take minutes under -race.
func sampleMeasurements(m int) []int {
	if m <= 64 {
		idx := make([]int, m)
		for i := range idx {
			idx[i] = i
		}
		return idx
	}
	idx := []int{0, m / 2, m - 1}
	if m > 355 {
		idx = append(idx, 354, 355)
	}
	return idx
}

// TestReadMeasurementMatchesEager reads measurements of a synthetic file (all
// of them) and of the FIR CI fixtures (see sampleMeasurements) through a
// lazy File and compares them with Open's ImpulseResponses.
func TestReadMeasurementMatchesEager(t *testing.T) {
	paths := map[string]string{"synthetic": saveSynthetic(t, 60, 16)}
	for _, name := range ciFixtures {
		paths[name] = testdataPath(t, name)
	}
	for name, path := range paths {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			eager, err := Open(path)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			if eager.DataType != DataTypeFIR {
				t.Skipf("DataType %s", eager.DataType)
			}
			lazy, err := OpenLazy(path)
			if err != nil {
				t.Fatalf("OpenLazy: %v", err)
			}
			t.Cleanup(func() { _ = lazy.Close() })
			for _, m := range sampleMeasurements(eager.M) {
				got, err := lazy.ReadMeasurement(m)
				if err != nil {
					t.Fatalf("ReadMeasurement(%d): %v", m, err)
				}
				if !reflect.DeepEqual(got, eager.ImpulseResponses[m]) {
					t.Fatalf("ReadMeasurement(%d) differs from Open", m)
				}
			}
		})
	}
}

// TestReadMeasurementEagerCopies checks that ReadMeasurement on a File from
// Open returns a copy, not the File's own slices.
func TestReadMeasurementEagerCopies(t *testing.T) {
	f, err := Open(saveSynthetic(t, 3, 4))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := f.ReadMeasurement(1)
	if err != nil {
		t.Fatalf("ReadMeasurement: %v", err)
	}
	if !reflect.DeepEqual(got, f.ImpulseResponses[1]) {
		t.Fatalf("ReadMeasurement(1) = %v, want %v", got, f.ImpulseResponses[1])
	}
	got[0][0] = -42
	if f.ImpulseResponses[1][0][0] == -42 {
		t.Errorf("ReadMeasurement aliases ImpulseResponses")
	}
}

// TestReadMeasurementErrors checks the index, DataType and closed-file
// errors.
func TestReadMeasurementErrors(t *testing.T) {
	path := saveSynthetic(t, 3, 4)
	for _, lazy := range []bool{false, true} {
		f, err := open(path, lazy)
		if err != nil {
			t.Fatalf("open(lazy=%v): %v", lazy, err)
		}
		for _, m := range []int{-1, 3} {
			if _, err := f.ReadMeasurement(m); !errors.Is(err, ErrIndexOutOfRange) {
				t.Errorf("lazy=%v ReadMeasurement(%d) error %v, want ErrIndexOutOfRange", lazy, m, err)
			}
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}

	f, err := OpenLazy(path)
	if err != nil {
		t.Fatalf("OpenLazy: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := f.ReadMeasurement(0); !errors.Is(err, fs.ErrClosed) {
		t.Errorf("ReadMeasurement after Close: error %v, want fs.ErrClosed", err)
	}

	tf := &File{DataType: DataTypeTF, M: 1, R: 1, N: 1}
	if _, err := tf.ReadMeasurement(0); !errors.Is(err, ErrUnsupportedDataType) {
		t.Errorf("TF ReadMeasurement error %v, want ErrUnsupportedDataType", err)
	}
}
