package sofa

import (
	"errors"
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

// saveSynthetic writes syntheticFIRFile(m, r, n) to a temporary file and
// returns its path.
func saveSynthetic(t *testing.T, m, r, n int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic.sofa")
	if err := syntheticFIRFile(m, r, n).Save(path); err != nil {
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
	path := saveSynthetic(t, 700, 2, 1024)
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
	path := saveSynthetic(t, 3, 2, 4)

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
	f, err := OpenLazy(saveSynthetic(t, 3, 2, 4))
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
