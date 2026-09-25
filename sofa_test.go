package sofa

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"testing"

	hdf5 "github.com/cwbudde/go-hdf5"
)

func TestOpen(t *testing.T) {
	files := []struct {
		path string
		m    int
		r    int
		n    int
		e    int
	}{
		{"MIT_KEMAR_normal_pinna.sofa", 710, 2, 512, 1},
		{"CIPIC_subject_003_hrir_final.sofa", 1250, 2, 200, 1},
		{"tester.sofa", 1250, 2, 256, 1},
		{"Mesh2HRTF.sofa", 1850, 2, 320, 1},
	}

	for _, tt := range files {
		t.Run(tt.path, func(t *testing.T) {
			f, err := Open(testdataPath(t, tt.path))
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer f.Close()

			if f.M != tt.m {
				t.Errorf("M = %d, want %d", f.M, tt.m)
			}
			if f.R != tt.r {
				t.Errorf("R = %d, want %d", f.R, tt.r)
			}
			if f.N != tt.n {
				t.Errorf("N = %d, want %d", f.N, tt.n)
			}
			if f.E != tt.e {
				t.Errorf("E = %d, want %d", f.E, tt.e)
			}
		})
	}
}

func TestOpenMetadata(t *testing.T) {
	f, err := Open(testdataPath(t, "MIT_KEMAR_normal_pinna.sofa"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	if f.Conventions != "SOFA" {
		t.Errorf("Conventions = %q, want SOFA", f.Conventions)
	}
	if f.Version != "1.0" {
		t.Errorf("Version = %q, want 1.0", f.Version)
	}
	if f.SOFAConventions != "SimpleFreeFieldHRIR" {
		t.Errorf("SOFAConventions = %q, want SimpleFreeFieldHRIR", f.SOFAConventions)
	}
	if f.DataType != "FIR" {
		t.Errorf("DataType = %q, want FIR", f.DataType)
	}
	if f.RoomType != "free field" {
		t.Errorf("RoomType = %q, want 'free field'", f.RoomType)
	}
	t.Logf("License: %s", f.License)
	t.Logf("ApplicationName: %s", f.ApplicationName)
}

func TestOpenImpulseResponses(t *testing.T) {
	f, err := Open(testdataPath(t, "MIT_KEMAR_normal_pinna.sofa"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	// Check IR dimensions.
	if len(f.ImpulseResponses) != f.M {
		t.Fatalf("IR M = %d, want %d", len(f.ImpulseResponses), f.M)
	}
	if len(f.ImpulseResponses[0]) != f.R {
		t.Fatalf("IR R = %d, want %d", len(f.ImpulseResponses[0]), f.R)
	}
	if len(f.ImpulseResponses[0][0]) != f.N {
		t.Fatalf("IR N = %d, want %d", len(f.ImpulseResponses[0][0]), f.N)
	}

	// IR data should not be all zeros.
	nonZero := false
	for _, v := range f.ImpulseResponses[0][0] {
		if v != 0 {
			nonZero = true
			break
		}
	}
	if !nonZero {
		t.Error("first IR is all zeros")
	}

	// Peak level should be reasonable for HRTF data.
	peak, err := f.IRPeakdB(0, 0)
	if err != nil {
		t.Fatalf("IRPeakdB: %v", err)
	}
	if peak < -60 || peak > 20 {
		t.Errorf("peak = %f dB, expected reasonable range", peak)
	}
	t.Logf("IR[0][0] peak: %.1f dB", peak)
}

func TestOpenSpatialData(t *testing.T) {
	f, err := Open(testdataPath(t, "MIT_KEMAR_normal_pinna.sofa"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	// Source positions should have M entries.
	if len(f.SourcePositions) != f.M {
		t.Errorf("SourcePositions = %d, want %d", len(f.SourcePositions), f.M)
	}

	// Receiver positions should have R entries.
	if len(f.ReceiverPositions) != f.R {
		t.Errorf("ReceiverPositions = %d, want %d", len(f.ReceiverPositions), f.R)
	}

	// Listener positions should exist.
	if len(f.ListenerPositions) == 0 {
		t.Error("no listener positions")
	}

	t.Logf("SourcePositions[0] = %+v", f.SourcePositions[0])
	t.Logf("ReceiverPositions = %+v", f.ReceiverPositions)
	t.Logf("ListenerUp = %+v", f.ListenerUp)
	t.Logf("ListenerView = %+v", f.ListenerView)
}

func TestOpenSamplingRate(t *testing.T) {
	f, err := Open(testdataPath(t, "MIT_KEMAR_normal_pinna.sofa"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	sr, err := f.SamplingRateScalar()
	if err != nil {
		t.Fatalf("SamplingRateScalar: %v", err)
	}
	if sr <= 0 {
		t.Fatalf("SamplingRate = %f, want > 0", sr)
	}
	t.Logf("SamplingRate = %.0f Hz", sr)

	dur, err := f.Duration()
	if err != nil {
		t.Fatalf("Duration: %v", err)
	}
	if dur <= 0 {
		t.Errorf("Duration = %f, want > 0", dur)
	}
	t.Logf("Duration = %.4f s (%.1f ms)", dur, dur*1000)
}

func TestParseDimensionSize(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"This is a netCDF dimension but not a netCDF variable.       710", 710},
		{"This is a netCDF dimension but not a netCDF variable.         2", 2},
		{"This is a netCDF dimension but not a netCDF variable.       512", 512},
		{"42", 42},
	}
	for _, tt := range tests {
		got, err := parseDimensionSize(tt.input)
		if err != nil {
			t.Errorf("parseDimensionSize(%q): %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseDimensionSize(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestIRAt(t *testing.T) {
	f, err := Open(testdataPath(t, "MIT_KEMAR_normal_pinna.sofa"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	// Valid access.
	ir, err := f.IRAt(0, 0)
	if err != nil {
		t.Fatalf("IRAt(0,0): %v", err)
	}
	if len(ir) != f.N {
		t.Errorf("IRAt(0,0) len = %d, want %d", len(ir), f.N)
	}

	// Out-of-range access.
	for _, idx := range [][2]int{{-1, 0}, {f.M, 0}, {0, -1}, {0, f.R}} {
		if _, err := f.IRAt(idx[0], idx[1]); !errors.Is(err, ErrIndexOutOfRange) {
			t.Errorf("IRAt(%d,%d) error = %v, want ErrIndexOutOfRange", idx[0], idx[1], err)
		}
		if _, err := f.IRPeakdB(idx[0], idx[1]); !errors.Is(err, ErrIndexOutOfRange) {
			t.Errorf("IRPeakdB(%d,%d) error = %v, want ErrIndexOutOfRange", idx[0], idx[1], err)
		}
	}
}

func TestOpenErrors(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{
			name:    "nonexistent file",
			path:    "testdata/nonexistent.sofa",
			wantErr: "open HDF5",
		},
		{
			name:    "invalid path",
			path:    "",
			wantErr: "open HDF5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Open(tt.path)
			if err == nil {
				f.Close()
				t.Fatalf("Open(%q) succeeded, want error", tt.path)
			}
			if tt.wantErr != "" && !contains(err.Error(), tt.wantErr) {
				t.Errorf("Open(%q) error = %v, want error containing %q", tt.path, err, tt.wantErr)
			}
		})
	}
}

// TestIRPeakdBZeroSilence covers the all-zero (silent) IR branch of
// IRPeakdB, where peak == 0 and the function should return -Inf.
func TestIRPeakdBZeroSilence(t *testing.T) {
	f := &File{
		DataType:         DataTypeFIR,
		M:                1,
		R:                1,
		N:                4,
		ImpulseResponses: [][][]float64{{{0, 0, 0, 0}}},
	}
	if got, err := f.IRPeakdB(0, 0); err != nil || !math.IsInf(got, -1) {
		t.Errorf("IRPeakdB(0,0) on silence = %v, %v; want -Inf, nil", got, err)
	}
}

// TestOpenNonSOFAHDF5 covers the "not a SOFA file" branch of Open by
// writing a valid HDF5 file with Conventions != "SOFA".
func TestOpenNonSOFAHDF5(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "not_sofa.h5")
	fw, err := hdf5.CreateForWrite(tmp, hdf5.CreateTruncate,
		hdf5.WithRootAttribute("Conventions", "NotSOFA"))
	if err != nil {
		t.Fatalf("CreateForWrite: %v", err)
	}
	if err := fw.Close(); err != nil {
		t.Fatalf("Close writer: %v", err)
	}

	f, err := Open(tmp)
	if err == nil {
		f.Close()
		t.Fatalf("Open(non-SOFA) succeeded, want error")
	}
	if !contains(err.Error(), "not a SOFA file") {
		t.Errorf("Open(non-SOFA) error = %v, want %q", err, "not a SOFA file")
	}
}

func TestParseDimensionSizeErrors(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"", true},                 // empty string
		{"not a number", true},     // invalid number
		{"abc def ghi", true},      // no number at end
		{"   ", true},              // whitespace only
		{"valid text 123", false},  // valid
		{"This is text 42", false}, // valid
	}

	for _, tt := range tests {
		_, err := parseDimensionSize(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseDimensionSize(%q) error = %v, wantErr = %v", tt.input, err, tt.wantErr)
		}
	}
}

func TestSamplingRateScalarEmpty(t *testing.T) {
	f := &File{SamplingRate: []float64{}}
	if got, err := f.SamplingRateScalar(); got != 0 || !errors.Is(err, ErrNoSamplingRate) {
		t.Errorf("SamplingRateScalar() on empty = (%f, %v), want (0, ErrNoSamplingRate)", got, err)
	}
}

func TestDurationEdgeCases(t *testing.T) {
	fir := func(sr []float64, n int) *File {
		return &File{DataType: DataTypeFIR, SamplingRate: sr, N: n}
	}
	tests := []struct {
		name    string
		f       *File
		want    float64
		wantErr error // nil: any error; checked only when wantOK is false
		wantOK  bool
	}{
		{name: "zero sample rate", f: fir([]float64{0}, 100)},
		{name: "negative sample rate", f: fir([]float64{-48000}, 100)},
		{name: "NaN sample rate", f: fir([]float64{math.NaN()}, 100)},
		{name: "zero samples", f: fir([]float64{44100}, 0)},
		{name: "empty sampling rate", f: fir([]float64{}, 100)},
		{
			name:    "TF counts frequency bins, not samples",
			f:       &File{DataType: DataTypeTF, SamplingRate: []float64{44100}, N: 441},
			wantErr: ErrUnsupportedDataType,
		},
		{
			name:    "SOS counts coefficients",
			f:       &File{DataType: DataTypeSOS, SamplingRate: []float64{44100}, N: 6},
			wantErr: ErrUnsupportedDataType,
		},
		{name: "valid", f: fir([]float64{44100}, 441), want: 0.01, wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.f.Duration()
			if !tt.wantOK {
				if err == nil {
					t.Fatalf("Duration() = %f, nil; want error", got)
				}
				if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
					t.Fatalf("Duration() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil || math.Abs(got-tt.want) > 0.0001 {
				t.Errorf("Duration() = %f, %v; want %f, nil", got, err, tt.want)
			}
		})
	}
}

func TestCloseNil(t *testing.T) {
	f := &File{}
	if err := f.Close(); err != nil {
		t.Errorf("Close() on a File built in memory = %v, want nil", err)
	}
}

// Helper function to check if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestReadReferenceValues pins real values from third-party files, read
// independently with h5py 3.11 / HDF5 1.14, so the reader is checked against
// the producers' own output rather than against a go-hdf5 round trip. The set
// covers files written by the SOFA API for Matlab/Octave (via netCDF-C), a
// file written by sofar (Python) through netCDF-C 4.8.1, and the libmysofa
// test corpus they are taken from; see testdata/PROVENANCE.md.
func TestReadReferenceValues(t *testing.T) {
	type sample struct {
		m, r, n int
		want    float64
	}
	cases := []struct {
		file         string
		apiName      string
		sr           float64
		m, r, n      int
		samples      []sample
		src0, src100 Vector3
		recv         []Vector3
		srcUnits     string
	}{
		{
			file:    "MIT_KEMAR_normal_pinna.sofa",
			apiName: "ARI SOFA API for Matlab/Octave",
			sr:      44100, m: 710, r: 2, n: 512,
			samples: []sample{
				{0, 0, 0, 6.103515625e-05},
				{0, 0, 3, 3.0517578125e-05},
				{100, 1, 20, 0.000518798828125},
				{547, 0, 37, -0.817657470703125},
			},
			src0:     Vector3{0, -40, 1.4},
			src100:   Vector3{264, -30, 1.4},
			recv:     []Vector3{{0, 0.09, 0}, {0, -0.09, 0}},
			srcUnits: "degree, degree, metre",
		},
		{
			file:    "CIPIC_subject_003_hrir_final.sofa",
			apiName: "ARI SOFA API for Matlab/Octave",
			sr:      44100, m: 1250, r: 2, n: 200,
			samples: []sample{
				{0, 0, 0, -1.3234222686339927e-07},
				{1, 1, 10, -0.0006885273230664886},
				{624, 0, 50, 0.04248973715168163},
				{172, 0, 34, -1.7692432757103778},
			},
			src0:     Vector3{82.89292388955351, -7.053022130283171, 1},
			src100:   Vector3{63.65899893141409, -23.927464720758916, 1},
			recv:     []Vector3{{0, -0.09, 0}, {0, 0.09, 0}},
			srcUnits: "degree, degree, meter",
		},
		{
			file:    "Mesh2HRTF.sofa",
			apiName: "sofar SOFA API for Python (pyfar.org)",
			sr:      48000, m: 1850, r: 2, n: 320,
			samples: []sample{
				{0, 0, 0, 0.024958510714037214},
				{0, 1, 5, 0.0005620974072178428},
				{100, 0, 20, -0.00543749012045571},
				{1329, 0, 41, -4.2614301601013365},
			},
			src0:     Vector3{0, -90, 1.5},
			src100:   Vector3{150, -65, 1.5},
			recv:     []Vector3{{0.005762, 0.062838, 0.001253}, {0.003341, -0.060828, 0.001562}},
			srcUnits: "degree, degree, meter",
		},
	}

	const tol = 1e-12
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			f, err := Open(testdataPath(t, tc.file))
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer f.Close()

			if f.APIName != tc.apiName {
				t.Errorf("APIName = %q, want %q", f.APIName, tc.apiName)
			}
			if f.DataType != "FIR" {
				t.Errorf("DataType = %q, want FIR", f.DataType)
			}
			if f.M != tc.m || f.R != tc.r || f.N != tc.n {
				t.Fatalf("dims = M%d R%d N%d, want M%d R%d N%d", f.M, f.R, f.N, tc.m, tc.r, tc.n)
			}
			sr, err := f.SamplingRateScalar()
			if err != nil {
				t.Fatalf("SamplingRateScalar: %v", err)
			}
			assertClose(t, "SamplingRate", sr, tc.sr, 0)

			for _, s := range tc.samples {
				got := f.ImpulseResponses[s.m][s.r][s.n]
				assertClose(t, fmt.Sprintf("IR[%d][%d][%d]", s.m, s.r, s.n), got, s.want, tol)
			}

			if len(f.SourcePositions) != tc.m {
				t.Fatalf("len(SourcePositions) = %d, want %d", len(f.SourcePositions), tc.m)
			}
			for i, want := range map[int]Vector3{0: tc.src0, 100: tc.src100} {
				got := f.SourcePositions[i]
				assertClose(t, fmt.Sprintf("SourcePositions[%d].X", i), got.X, want.X, tol)
				assertClose(t, fmt.Sprintf("SourcePositions[%d].Y", i), got.Y, want.Y, tol)
				assertClose(t, fmt.Sprintf("SourcePositions[%d].Z", i), got.Z, want.Z, tol)
			}
			if f.SourcePositionType != CoordinateSpherical {
				t.Errorf("SourcePositionType = %q, want %q", f.SourcePositionType, CoordinateSpherical)
			}
			if f.SourcePositionUnits != tc.srcUnits {
				t.Errorf("SourcePositionUnits = %q, want %q", f.SourcePositionUnits, tc.srcUnits)
			}

			if len(f.ReceiverPositions) != len(tc.recv) {
				t.Fatalf("len(ReceiverPositions) = %d, want %d", len(f.ReceiverPositions), len(tc.recv))
			}
			for i, want := range tc.recv {
				got := f.ReceiverPositions[i]
				assertClose(t, fmt.Sprintf("ReceiverPositions[%d].X", i), got.X, want.X, tol)
				assertClose(t, fmt.Sprintf("ReceiverPositions[%d].Y", i), got.Y, want.Y, tol)
				assertClose(t, fmt.Sprintf("ReceiverPositions[%d].Z", i), got.Z, want.Z, tol)
			}
		})
	}
}
