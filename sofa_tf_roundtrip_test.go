package sofa

import (
	"math"
	"path/filepath"
	"strings"
	"testing"
)

// TestTFRoundTrip verifies that a TF File survives Save → Open with all
// frequencies, real, and imag values intact.
func TestTFRoundTrip(t *testing.T) {
	const M, R, E, N = 4, 1, 1, 5

	freqs := []float64{100, 200, 400, 800, 1600}

	tfReal := make([][][]float64, M)
	tfImag := make([][][]float64, M)
	for m := range M {
		tfReal[m] = make([][]float64, R)
		tfImag[m] = make([][]float64, R)
		for r := range R {
			tfReal[m][r] = make([]float64, N)
			tfImag[m][r] = make([]float64, N)
			for n := range N {
				phase := float64(m+1) * float64(n+1) * 0.1
				tfReal[m][r][n] = math.Cos(phase)
				tfImag[m][r][n] = math.Sin(phase)
			}
		}
	}

	src := &File{
		Conventions:            "SOFA",
		Version:                "2.0",
		SOFAConventions:        "FreeFieldDirectivityTF",
		SOFAConventionsVersion: "1.0",
		DataType:               "TF",
		M:                      M,
		R:                      R,
		E:                      E,
		N:                      N,
		Frequencies:            freqs,
		TFReal:                 tfReal,
		TFImag:                 tfImag,
		SourcePositions: []Vector3{
			{1, 0, 0}, {0, 1, 0}, {-1, 0, 0}, {0, -1, 0},
		},
		ListenerPositions: []Vector3{{0, 0, 0}},
		ReceiverPositions: []Vector3{{0, 0, 0}},
		EmitterPositions:  []Vector3{{0, 0, 0}},
		ListenerUp:        Vector3{0, 0, 1},
		ListenerView:      Vector3{1, 0, 0},
		Title:             "tf round-trip fixture",
		ApplicationName:   "go-sofa-test",
	}

	path := filepath.Join(t.TempDir(), "tf_roundtrip.sofa")
	if err := src.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	dst, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer dst.Close()

	if dst.DataType != "TF" {
		t.Errorf("DataType = %q, want %q", dst.DataType, "TF")
	}
	if dst.SOFAConventions != "FreeFieldDirectivityTF" {
		t.Errorf("SOFAConventions = %q, want %q", dst.SOFAConventions, "FreeFieldDirectivityTF")
	}
	if dst.M != M || dst.R != R || dst.E != E || dst.N != N {
		t.Errorf("dimensions = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
			dst.M, dst.R, dst.E, dst.N, M, R, E, N)
	}

	if len(dst.Frequencies) != N {
		t.Fatalf("len(Frequencies) = %d, want %d", len(dst.Frequencies), N)
	}
	for i, want := range freqs {
		if math.Abs(dst.Frequencies[i]-want) > 1e-9 {
			t.Errorf("Frequencies[%d] = %g, want %g", i, dst.Frequencies[i], want)
		}
	}

	for m := range M {
		for r := range R {
			for n := range N {
				if got, want := dst.TFReal[m][r][n], tfReal[m][r][n]; math.Abs(got-want) > 1e-12 {
					t.Errorf("TFReal[%d][%d][%d] = %g, want %g", m, r, n, got, want)
				}
				if got, want := dst.TFImag[m][r][n], tfImag[m][r][n]; math.Abs(got-want) > 1e-12 {
					t.Errorf("TFImag[%d][%d][%d] = %g, want %g", m, r, n, got, want)
				}
			}
		}
	}
}

// TestValidateTFRejectsMissingFields ensures the validator catches incomplete
// TF Files before they reach Save().
func TestValidateTFRejectsMissingFields(t *testing.T) {
	tests := []struct {
		name  string
		mut   func(*File)
		match string
	}{
		{
			name:  "missing Frequencies",
			mut:   func(f *File) { f.Frequencies = nil },
			match: "frequencies",
		},
		{
			name:  "wrong-shape TFReal",
			mut:   func(f *File) { f.TFReal = make([][][]float64, 1) },
			match: "TFReal",
		},
		{
			name:  "wrong-shape TFImag",
			mut:   func(f *File) { f.TFImag = make([][][]float64, 1) },
			match: "TFImag",
		},
		{
			name:  "unsupported DataType",
			mut:   func(f *File) { f.DataType = "BOGUS" },
			match: "unsupported DataType",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := minimalTFFile()
			tt.mut(f)
			err := f.validate()
			if err == nil {
				t.Fatalf("validate() = nil, want error containing %q", tt.match)
			}
			if !strings.Contains(err.Error(), tt.match) {
				t.Errorf("validate() error = %v, want substring %q", err, tt.match)
			}
		})
	}
}

// TestReadRealTFFile opens an upstream TF SOFA file shipped in testdata
// and confirms the high-level shape. Reading these files used to fail
// in go-hdf5 (V2 object header continuation chunks were ignored, so
// dataset layout messages were missing).
func TestReadRealTFFile(t *testing.T) {
	cases := []struct {
		path                            string
		wantSOFAConv                    string
		wantDataType                    string
		wantM, wantR, wantE, wantNAtMin int
	}{
		{
			path:         "testdata/GeneralTF_2.0.sofa",
			wantSOFAConv: "GeneralTF",
			wantDataType: "TF",
			wantM:        4, wantR: 4, wantE: 1, wantNAtMin: 1,
		},
		{
			path:         "testdata/FreeFieldHRTF_2.0.sofa",
			wantSOFAConv: "SimpleFreeFieldHRTF",
			wantDataType: "TF",
			wantM:        2354, wantR: 2, wantE: 1, wantNAtMin: 1,
		},
	}
	for _, tc := range cases {
		t.Run(filepath.Base(tc.path), func(t *testing.T) {
			f, err := Open(tc.path)
			if err != nil {
				t.Fatalf("Open(%q): %v", tc.path, err)
			}
			defer f.Close()

			if f.SOFAConventions != tc.wantSOFAConv {
				t.Errorf("SOFAConventions = %q, want %q", f.SOFAConventions, tc.wantSOFAConv)
			}
			if f.DataType != tc.wantDataType {
				t.Errorf("DataType = %q, want %q", f.DataType, tc.wantDataType)
			}
			if f.M != tc.wantM || f.R != tc.wantR || f.E != tc.wantE {
				t.Errorf("dims = M%d R%d E%d, want M%d R%d E%d",
					f.M, f.R, f.E, tc.wantM, tc.wantR, tc.wantE)
			}
			if f.N < tc.wantNAtMin {
				t.Errorf("N = %d, want >= %d", f.N, tc.wantNAtMin)
			}
			if len(f.TFReal) != f.M || len(f.TFImag) != f.M {
				t.Errorf("TFReal/TFImag length M dim = %d/%d, want %d",
					len(f.TFReal), len(f.TFImag), f.M)
			}
		})
	}
}

func minimalTFFile() *File {
	const M, R, E, N = 2, 1, 1, 3
	freqs := []float64{100, 200, 400}
	tfR := make([][][]float64, M)
	tfI := make([][][]float64, M)
	for m := range M {
		tfR[m] = [][]float64{make([]float64, N)}
		tfI[m] = [][]float64{make([]float64, N)}
	}
	return &File{
		Conventions:            "SOFA",
		Version:                "2.0",
		SOFAConventions:        "FreeFieldDirectivityTF",
		SOFAConventionsVersion: "1.0",
		DataType:               "TF",
		M:                      M,
		R:                      R,
		E:                      E,
		N:                      N,
		Frequencies:            freqs,
		TFReal:                 tfR,
		TFImag:                 tfI,
	}
}
