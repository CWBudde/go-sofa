package sofa

import (
	"math"
	"path/filepath"
	"strings"
	"testing"
)

// TestTFERoundTrip verifies that a TF-E File survives Save → Open with
// frequencies and complex transfer functions intact across the
// emitter dimension.
func TestTFERoundTrip(t *testing.T) {
	const M, R, E, N = 3, 2, 2, 4

	freqs := []float64{125, 500, 2000, 8000}

	tfReal := make([][][][]float64, M)
	tfImag := make([][][][]float64, M)
	for m := range M {
		tfReal[m] = make([][][]float64, R)
		tfImag[m] = make([][][]float64, R)
		for r := range R {
			tfReal[m][r] = make([][]float64, E)
			tfImag[m][r] = make([][]float64, E)
			for e := range E {
				tfReal[m][r][e] = make([]float64, N)
				tfImag[m][r][e] = make([]float64, N)
				for n := range N {
					phase := float64(m+1) * float64(r+1) * float64(e+1) * float64(n+1) * 0.05
					tfReal[m][r][e][n] = math.Cos(phase)
					tfImag[m][r][e][n] = math.Sin(phase)
				}
			}
		}
	}

	src := &File{
		Conventions:            "SOFA",
		Version:                "2.0",
		SOFAConventions:        "GeneralTF-E",
		SOFAConventionsVersion: "1.0",
		DataType:               "TF-E",
		M:                      M,
		R:                      R,
		E:                      E,
		N:                      N,
		Frequencies:            freqs,
		TFRealE:                tfReal,
		TFImagE:                tfImag,
		ListenerPositions:      []Vector3{{0, 0, 0}},
		ReceiverPositions:      []Vector3{{0, 0.09, 0}, {0, -0.09, 0}},
		SourcePositions: []Vector3{
			{1, 0, 0}, {0, 1, 0}, {-1, 0, 0},
		},
		EmitterPositions: []Vector3{{0, 0, 0}, {0.5, 0, 0}},
	}

	tmp := filepath.Join(t.TempDir(), "tfe.sofa")
	if err := src.Save(tmp); err != nil {
		t.Fatalf("Save: %v", err)
	}
	dst, err := Open(tmp)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer dst.Close()

	if dst.DataType != "TF-E" || dst.M != M || dst.R != R || dst.E != E || dst.N != N {
		t.Fatalf("round-trip dims/DataType mismatch: %+v", dst)
	}
	for i, want := range freqs {
		if math.Abs(dst.Frequencies[i]-want) > 1e-12 {
			t.Errorf("Frequencies[%d] = %g, want %g", i, dst.Frequencies[i], want)
		}
	}
	for m := range M {
		for r := range R {
			for e := range E {
				for n := range N {
					if d := math.Abs(dst.TFRealE[m][r][e][n] - tfReal[m][r][e][n]); d > 1e-12 {
						t.Errorf("TFRealE[%d][%d][%d][%d] mismatch (Δ=%g)", m, r, e, n, d)
					}
					if d := math.Abs(dst.TFImagE[m][r][e][n] - tfImag[m][r][e][n]); d > 1e-12 {
						t.Errorf("TFImagE[%d][%d][%d][%d] mismatch (Δ=%g)", m, r, e, n, d)
					}
				}
			}
		}
	}
}

// TestSOSRoundTrip verifies that an SOS File (one biquad per
// measurement-receiver pair, N=6) survives Save → Open.
func TestSOSRoundTrip(t *testing.T) {
	const M, R, E, N = 4, 2, 1, 6

	sos := make([][][]float64, M)
	for m := range M {
		sos[m] = make([][]float64, R)
		for r := range R {
			// Distinguishable values per (m,r): b0..b2, a0..a2.
			sos[m][r] = []float64{
				1.0 + 0.01*float64(m),
				0.5 + 0.02*float64(r),
				0.25,
				1.0,
				-0.5 + 0.03*float64(m+r),
				0.125,
			}
		}
	}

	src := &File{
		Conventions:            "SOFA",
		Version:                "2.0",
		SOFAConventions:        "SimpleFreeFieldHRSOS",
		SOFAConventionsVersion: "1.0",
		DataType:               "SOS",
		M:                      M,
		R:                      R,
		E:                      E,
		N:                      N,
		SOSCoefficients:        sos,
		SamplingRate:           []float64{48000},
		Delay:                  []float64{0, 0},
		ListenerPositions:      []Vector3{{0, 0, 0}},
		ReceiverPositions:      []Vector3{{0, 0.09, 0}, {0, -0.09, 0}},
		SourcePositions: []Vector3{
			{1, 0, 0}, {0, 1, 0}, {-1, 0, 0}, {0, -1, 0},
		},
		EmitterPositions: []Vector3{{0, 0, 0}},
	}

	tmp := filepath.Join(t.TempDir(), "sos.sofa")
	if err := src.Save(tmp); err != nil {
		t.Fatalf("Save: %v", err)
	}
	dst, err := Open(tmp)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer dst.Close()

	if dst.DataType != "SOS" || dst.M != M || dst.R != R || dst.E != E || dst.N != N {
		t.Fatalf("round-trip dims/DataType mismatch: %+v", dst)
	}
	for m := range M {
		for r := range R {
			for n := range N {
				if d := math.Abs(dst.SOSCoefficients[m][r][n] - sos[m][r][n]); d > 1e-12 {
					t.Errorf("SOSCoefficients[%d][%d][%d] mismatch (Δ=%g)", m, r, n, d)
				}
			}
		}
	}
	if len(dst.SamplingRate) == 0 || dst.SamplingRate[0] != 48000 {
		t.Errorf("SamplingRate not round-tripped: %v", dst.SamplingRate)
	}
}

// TestValidateTFEAndSOS exercises the new validators.
func TestValidateTFEAndSOS(t *testing.T) {
	t.Run("TF-E missing Frequencies", func(t *testing.T) {
		f := minimalTFEFile()
		f.Frequencies = nil
		err := f.validate()
		if err == nil || !strings.Contains(err.Error(), "frequencies") {
			t.Fatalf("validate() = %v, want substring \"frequencies\"", err)
		}
	})
	t.Run("TF-E wrong-shape TFRealE", func(t *testing.T) {
		f := minimalTFEFile()
		f.TFRealE = make([][][][]float64, 1) // M wants 2
		err := f.validate()
		if err == nil || !strings.Contains(err.Error(), "TFRealE") {
			t.Fatalf("validate() = %v, want substring \"TFRealE\"", err)
		}
	})
	t.Run("SOS N not divisible by 6", func(t *testing.T) {
		f := minimalSOSFile()
		f.N = 5 // not divisible by 6
		err := f.validate()
		if err == nil || !strings.Contains(err.Error(), "divisible by 6") {
			t.Fatalf("validate() = %v, want substring \"divisible by 6\"", err)
		}
	})
}

func minimalTFEFile() *File {
	const M, R, E, N = 2, 1, 2, 3
	freqs := []float64{100, 200, 400}
	tfR := make([][][][]float64, M)
	tfI := make([][][][]float64, M)
	for m := range M {
		tfR[m] = make([][][]float64, R)
		tfI[m] = make([][][]float64, R)
		for r := range R {
			tfR[m][r] = [][]float64{make([]float64, N), make([]float64, N)}
			tfI[m][r] = [][]float64{make([]float64, N), make([]float64, N)}
		}
	}
	return &File{
		Conventions:            "SOFA",
		Version:                "2.0",
		SOFAConventions:        "GeneralTF-E",
		SOFAConventionsVersion: "1.0",
		DataType:               "TF-E",
		M:                      M, R: R, E: E, N: N,
		Frequencies: freqs,
		TFRealE:     tfR,
		TFImagE:     tfI,
	}
}

func minimalSOSFile() *File {
	const M, R, E, N = 1, 1, 1, 6
	sos := [][][]float64{{{1, 0, 0, 1, 0, 0}}}
	return &File{
		Conventions:            "SOFA",
		Version:                "2.0",
		SOFAConventions:        "SimpleFreeFieldHRSOS",
		SOFAConventionsVersion: "1.0",
		DataType:               "SOS",
		M:                      M, R: R, E: E, N: N,
		SOSCoefficients: sos,
		SamplingRate:    []float64{48000},
	}
}

// TestReadRealTFEAndSOS opens upstream-toolbox TF-E and SOS files and
// confirms the high-level shape parses correctly.
func TestReadRealTFEAndSOS(t *testing.T) {
	cases := []struct {
		path              string
		wantConv          string
		wantDataType      string
		wantM, wantR      int
		wantE, wantNAtMin int
	}{
		{
			path:         "testdata/GeneralTF-E_1.0.sofa",
			wantConv:     "GeneralTF-E",
			wantDataType: "TF-E",
			// This particular GeneralTF-E demo has M=4, R=4800, E=1, N=1.
			wantM: 4, wantR: 4800, wantE: 1, wantNAtMin: 1,
		},
		{
			path:         "testdata/FreeFieldHRTF_1.0.sofa",
			wantConv:     "FreeFieldHRTF",
			wantDataType: "TF-E",
			wantM:        1, wantR: 2, wantE: 1, wantNAtMin: 1,
		},
		{
			path:         "testdata/SimpleFreeFieldHRSOS_1.0.sofa",
			wantConv:     "SimpleFreeFieldHRSOS",
			wantDataType: "SOS",
			wantM:        1, wantR: 2, wantE: 1, wantNAtMin: 6,
		},
	}
	for _, tc := range cases {
		t.Run(filepath.Base(tc.path), func(t *testing.T) {
			f, err := Open(tc.path)
			if err != nil {
				t.Fatalf("Open(%q): %v", tc.path, err)
			}
			defer f.Close()

			if f.SOFAConventions != tc.wantConv {
				t.Errorf("SOFAConventions = %q, want %q", f.SOFAConventions, tc.wantConv)
			}
			if f.DataType != tc.wantDataType {
				t.Errorf("DataType = %q, want %q", f.DataType, tc.wantDataType)
			}
			if f.M < tc.wantM || f.R != tc.wantR || f.E < tc.wantE || f.N < tc.wantNAtMin {
				t.Errorf("dims = M%d R%d E%d N%d, want >= M%d R%d E%d N%d",
					f.M, f.R, f.E, f.N, tc.wantM, tc.wantR, tc.wantE, tc.wantNAtMin)
			}

			switch tc.wantDataType {
			case "TF-E":
				if len(f.TFRealE) != f.M || len(f.TFImagE) != f.M {
					t.Errorf("TFRealE/TFImagE M dim = %d/%d, want %d",
						len(f.TFRealE), len(f.TFImagE), f.M)
				}
			case "SOS":
				if len(f.SOSCoefficients) != f.M {
					t.Errorf("SOSCoefficients M dim = %d, want %d",
						len(f.SOSCoefficients), f.M)
				}
			}
		})
	}
}
