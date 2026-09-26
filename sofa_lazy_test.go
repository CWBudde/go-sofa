package sofa

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
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

// TestRangeMeasurementsAbort checks that an error from the callback stops
// the iteration and is returned as is.
func TestRangeMeasurementsAbort(t *testing.T) {
	f, err := OpenLazy(saveSynthetic(t, 5, 4))
	if err != nil {
		t.Fatalf("OpenLazy: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	errStop := errors.New("stop")
	calls := 0
	err = f.RangeMeasurements(func(m int, _ [][]float64) error {
		calls++
		if m == 2 {
			return errStop
		}
		return nil
	})
	if !errors.Is(err, errStop) {
		t.Errorf("RangeMeasurements error %v, want the callback's error", err)
	}
	if calls != 3 {
		t.Errorf("callback called %d times, want 3", calls)
	}
}

// TestRangeMeasurementsVisitsAll checks that RangeMeasurements passes every
// measurement in order, equal to Open's ImpulseResponses, and reports read
// errors with the measurement index.
func TestRangeMeasurementsVisitsAll(t *testing.T) {
	path := saveSynthetic(t, 6, 3)
	eager, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	lazy, err := OpenLazy(path)
	if err != nil {
		t.Fatalf("OpenLazy: %v", err)
	}
	var got [][][]float64
	err = lazy.RangeMeasurements(func(m int, ir [][]float64) error {
		if m != len(got) {
			t.Errorf("measurement %d passed after %d others", m, len(got))
		}
		got = append(got, ir)
		return nil
	})
	if err != nil {
		t.Fatalf("RangeMeasurements: %v", err)
	}
	if !reflect.DeepEqual(got, eager.ImpulseResponses) {
		t.Errorf("RangeMeasurements differs from Open")
	}

	if err := lazy.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err = lazy.RangeMeasurements(func(int, [][]float64) error { return nil })
	if !errors.Is(err, fs.ErrClosed) {
		t.Errorf("RangeMeasurements after Close: error %v, want fs.ErrClosed", err)
	}
}

// iotaData returns 0, 1, …, n-1: every value of a crafted dataset differs,
// so a misplaced sample is detected.
func iotaData(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = float64(i)
	}
	return out
}

// tfSpec is a TF file with M=3, R=2, N=4 whose Data.Real and Data.Imag
// hold distinct values.
func tfSpec() craftedSpec {
	mrn := []string{dimM, dimR, dimN}
	return craftedSpec{
		dataType: DataTypeTF,
		dims:     map[string]int{dimM: 3, dimR: 2, dimE: 1, dimN: 4, dimC: 3, dimI: 1},
		vars: map[string]craftedVar{
			"Data.Real": {dims: mrn, data: iotaData(24)},
			"Data.Imag": {dims: mrn, data: iotaData(48)[24:]},
		},
	}
}

// sosSpec is an SOS file with M=3, R=2, N=12 (two biquads) of distinct
// coefficients.
func sosSpec() craftedSpec {
	return craftedSpec{
		dataType: DataTypeSOS,
		dims:     map[string]int{dimM: 3, dimR: 2, dimE: 1, dimN: 12, dimC: 3, dimI: 1},
		vars: map[string]craftedVar{
			"Data.SOS":          {dims: []string{dimM, dimR, dimN}, data: iotaData(72)},
			"Data.SamplingRate": {dims: []string{dimI}, data: []float64{48000}},
		},
	}
}

// readAnyMeasurement reads measurement m through the ReadMeasurement*
// method of f's DataType, in the form eagerMeasurement returns.
func readAnyMeasurement(f *File, m int) (any, error) {
	switch f.DataType {
	case DataTypeFIR:
		return f.ReadMeasurement(m)
	case DataTypeTF:
		re, im, err := f.ReadMeasurementTF(m)
		return [2][][]float64{re, im}, err
	case DataTypeTFE:
		re, im, err := f.ReadMeasurementTFE(m)
		return [2][][][]float64{re, im}, err
	case DataTypeSOS:
		return f.ReadMeasurementSOS(m)
	}
	return nil, fmt.Errorf("DataType %q", f.DataType)
}

// eagerMeasurement returns measurement m of an eager File's audio fields.
func eagerMeasurement(f *File, m int) any {
	switch f.DataType {
	case DataTypeFIR:
		return f.ImpulseResponses[m]
	case DataTypeTF:
		return [2][][]float64{f.TFReal[m], f.TFImag[m]}
	case DataTypeTFE:
		return [2][][][]float64{f.TFRealE[m], f.TFImagE[m]}
	case DataTypeSOS:
		return f.SOSCoefficients[m]
	}
	return nil
}

// TestReadMeasurementTypesMatchEager reads every measurement of crafted TF,
// TF-E (both axis orders, labelled and not) and SOS files, and sampled
// measurements of the non-FIR CI fixtures, through a lazy File and through
// an eager one, and compares both with Open's audio fields.
func TestReadMeasurementTypesMatchEager(t *testing.T) {
	mren := []string{dimM, dimR, dimE, dimN}
	mrne := []string{dimM, dimR, dimN, dimE}
	paths := map[string]string{
		"TF":                        writeCraftedSpec(t, tfSpec()),
		"TF-E [M,R,E,N] unlabelled": writeCraftedSpec(t, tfeSpec(3, 4, mren, false)),
		"TF-E [M,R,N,E] unlabelled": writeCraftedSpec(t, tfeSpec(3, 4, mrne, false)),
		"TF-E [M,R,E,N] labelled":   writeCraftedSpec(t, tfeSpec(3, 3, mren, true)),
		"TF-E [M,R,N,E] labelled":   writeCraftedSpec(t, tfeSpec(3, 3, mrne, true)),
		"SOS":                       writeCraftedSpec(t, sosSpec()),
	}
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
			if eager.DataType == DataTypeFIR {
				t.Skip("FIR is covered by TestReadMeasurementMatchesEager")
			}
			lazy, err := OpenLazy(path)
			if err != nil {
				t.Fatalf("OpenLazy: %v", err)
			}
			t.Cleanup(func() { _ = lazy.Close() })
			for _, m := range sampleMeasurements(eager.M) {
				want := eagerMeasurement(eager, m)
				for _, f := range []*File{lazy, eager} {
					got, err := readAnyMeasurement(f, m)
					if err != nil {
						t.Fatalf("lazy=%v measurement %d: %v", f.lazy, m, err)
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("lazy=%v measurement %d differs from Open", f.lazy, m)
					}
				}
			}
		})
	}
}

// TestReadMeasurementTypesEagerCopy checks that the TF, TF-E and SOS
// readers return copies on a File from Open.
func TestReadMeasurementTypesEagerCopy(t *testing.T) {
	tf, err := Open(writeCraftedSpec(t, tfSpec()))
	if err != nil {
		t.Fatalf("Open TF: %v", err)
	}
	re, im, err := tf.ReadMeasurementTF(1)
	if err != nil {
		t.Fatalf("ReadMeasurementTF: %v", err)
	}
	re[0][0], im[0][0] = -42, -42
	if tf.TFReal[1][0][0] == -42 || tf.TFImag[1][0][0] == -42 {
		t.Errorf("ReadMeasurementTF aliases TFReal/TFImag")
	}

	tfe, err := Open(writeCraftedSpec(t, tfeSpec(3, 4, []string{dimM, dimR, dimN, dimE}, false)))
	if err != nil {
		t.Fatalf("Open TF-E: %v", err)
	}
	reE, imE, err := tfe.ReadMeasurementTFE(1)
	if err != nil {
		t.Fatalf("ReadMeasurementTFE: %v", err)
	}
	reE[0][0][0], imE[0][0][0] = -42, -42
	if tfe.TFRealE[1][0][0][0] == -42 || tfe.TFImagE[1][0][0][0] == -42 {
		t.Errorf("ReadMeasurementTFE aliases TFRealE/TFImagE")
	}

	sos, err := Open(writeCraftedSpec(t, sosSpec()))
	if err != nil {
		t.Fatalf("Open SOS: %v", err)
	}
	c, err := sos.ReadMeasurementSOS(1)
	if err != nil {
		t.Fatalf("ReadMeasurementSOS: %v", err)
	}
	c[0][0] = -42
	if sos.SOSCoefficients[1][0][0] == -42 {
		t.Errorf("ReadMeasurementSOS aliases SOSCoefficients")
	}
}

// TestReadMeasurementTypesErrors checks the index, DataType and closed-file
// errors of the TF, TF-E and SOS readers.
func TestReadMeasurementTypesErrors(t *testing.T) {
	specs := []craftedSpec{tfSpec(), tfeSpec(3, 4, []string{dimM, dimR, dimN, dimE}, false), sosSpec()}
	for _, spec := range specs {
		path := writeCraftedSpec(t, spec)
		for _, lazy := range []bool{false, true} {
			f, err := open(path, lazy)
			if err != nil {
				t.Fatalf("%s open(lazy=%v): %v", spec.dataType, lazy, err)
			}
			for _, m := range []int{-1, f.M} {
				if _, err := readAnyMeasurement(f, m); !errors.Is(err, ErrIndexOutOfRange) {
					t.Errorf("%s lazy=%v measurement %d: error %v, want ErrIndexOutOfRange",
						spec.dataType, lazy, m, err)
				}
			}
			if err := f.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			if _, err := readAnyMeasurement(f, 0); lazy && !errors.Is(err, fs.ErrClosed) {
				t.Errorf("%s measurement after Close: error %v, want fs.ErrClosed", spec.dataType, err)
			}
		}
	}

	fir := &File{DataType: DataTypeFIR, M: 1, R: 1, N: 1}
	if _, _, err := fir.ReadMeasurementTF(0); !errors.Is(err, ErrUnsupportedDataType) {
		t.Errorf("FIR ReadMeasurementTF error %v, want ErrUnsupportedDataType", err)
	}
	if _, _, err := fir.ReadMeasurementTFE(0); !errors.Is(err, ErrUnsupportedDataType) {
		t.Errorf("FIR ReadMeasurementTFE error %v, want ErrUnsupportedDataType", err)
	}
	if _, err := fir.ReadMeasurementSOS(0); !errors.Is(err, ErrUnsupportedDataType) {
		t.Errorf("FIR ReadMeasurementSOS error %v, want ErrUnsupportedDataType", err)
	}
}

// TestOpenLazyRejectsNonNumericAudio checks that OpenLazy, which does not
// read the audio data, still rejects an audio dataset Open cannot read
// because of its datatype (here fixed-length strings).
func TestOpenLazyRejectsNonNumericAudio(t *testing.T) {
	for _, tc := range []struct {
		spec craftedSpec
		name string
	}{
		{firSpec(), "Data.IR"},
		{tfSpec(), "Data.Real"},
		{tfSpec(), "Data.Imag"},
		{tfeSpec(3, 4, []string{dimM, dimR, dimN, dimE}, false), "Data.Imag"},
		{sosSpec(), "Data.SOS"},
	} {
		t.Run(tc.spec.dataType+" "+tc.name, func(t *testing.T) {
			shape := make([]uint64, 0, 4)
			order := tc.spec.vars[tc.name].dims
			if order == nil {
				order = []string{dimM, dimR, dimN, dimE}
			}
			for _, d := range order {
				shape = append(shape, uint64(tc.spec.dims[d])) //nolint:gosec // small test sizes
			}
			delete(tc.spec.vars, tc.name)
			tc.spec.extra = func(t *testing.T, fw *hdf5.FileWriter) {
				t.Helper()
				n := 1
				for _, d := range shape {
					n *= int(d) //nolint:gosec // small test sizes
				}
				ds, err := fw.CreateDataset("/"+tc.name, hdf5.String, shape, hdf5.WithStringSize(4))
				if err != nil {
					t.Fatalf("create /%s: %v", tc.name, err)
				}
				if err := ds.Write(make([]string, n)); err != nil {
					t.Fatalf("write /%s: %v", tc.name, err)
				}
			}
			path := writeCraftedSpec(t, tc.spec)
			if f, err := Open(path); err == nil {
				f.Close()
				t.Fatalf("Open accepted a string %s", tc.name)
			}
			f, err := OpenLazy(path)
			if err == nil {
				f.Close()
				t.Fatalf("OpenLazy accepted a string %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.name) {
				t.Errorf("error %q does not name %s", err, tc.name)
			}
		})
	}
}
