package sofa

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadSHEncodedTFE(t *testing.T) {
	const path = "testdata/demo_FreeFieldHRTF_4_SH.sofa"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("SH testdata file not present (download from https://www.sofaconventions.org/data/sofatoolbox_test/demo_FreeFieldHRTF_4_SH.sofa)")
	}
	f, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	if f.DataType != dataTypeTFE {
		t.Errorf("DataType = %q, want %q", f.DataType, dataTypeTFE)
	}
	if f.SOFAConventions != "FreeFieldHRTF" {
		t.Errorf("SOFAConventions = %q, want FreeFieldHRTF", f.SOFAConventions)
	}
	if f.E != 1156 {
		t.Errorf("E = %d, want 1156", f.E)
	}
	if f.N != 129 {
		t.Errorf("N = %d, want 129", f.N)
	}
	// History attribute carries "Converted to Spherical Harmonics", so
	// IsSHEncoded picks the file up via the History detection path.
	if !f.IsSHEncoded() {
		t.Errorf("IsSHEncoded() = false, want true (History should match)")
	}
	if lmax, ok := f.SHOrder(); !ok || lmax != 33 {
		t.Errorf("SHOrder() = (%d, %v), want (33, true)", lmax, ok)
	}
	if got := f.SHCoefficientCount(); got != 1156 {
		t.Errorf("SHCoefficientCount() = %d, want 1156", got)
	}
	if w := f.SHWarnings(); len(w) != 0 {
		t.Errorf("SHWarnings() = %v, want none on a clean SH file", w)
	}
	// go-sofa stores TF-E as [M][R][E][N].
	if got := len(f.TFRealE); got != f.M {
		t.Fatalf("len(TFRealE) = %d, want M=%d", got, f.M)
	}
	if got := len(f.TFRealE[0]); got != f.R {
		t.Fatalf("len(TFRealE[0]) = %d, want R=%d", got, f.R)
	}
	if got := len(f.TFRealE[0][0]); got != f.E {
		t.Errorf("len(TFRealE[0][0]) = %d, want E=%d", got, f.E)
	}
	if got := len(f.TFRealE[0][0][0]); got != f.N {
		t.Errorf("len(TFRealE[0][0][0]) = %d, want N=%d", got, f.N)
	}
}

func TestSHDetection(t *testing.T) {
	cases := []struct {
		name       string
		convention string
		dataType   string
		e          int
		wantSH     bool
		wantLmax   int
		wantOK     bool
		wantCoeffs int
	}{
		{
			name:       "plain HRTF E=1 not SH",
			convention: "SimpleFreeFieldHRTF",
			dataType:   dataTypeTF,
			e:          1,
			wantSH:     false,
		},
		{
			name:       "HRSH Lmax=1 (E=4)",
			convention: "FreeFieldHRSH",
			dataType:   dataTypeTFE,
			e:          4,
			wantSH:     true,
			wantLmax:   1,
			wantOK:     true,
			wantCoeffs: 4,
		},
		{
			name:       "HRSH Lmax=33 (E=1156, real demo file)",
			convention: "FreeFieldHRSH",
			dataType:   dataTypeTFE,
			e:          1156,
			wantSH:     true,
			wantLmax:   33,
			wantOK:     true,
			wantCoeffs: 1156,
		},
		{
			name:       "HRSH but E=5 not perfect square",
			convention: "FreeFieldHRSH",
			dataType:   dataTypeTFE,
			e:          5,
			wantSH:     false,
		},
		{
			name:       "convention without SH suffix even if E=4",
			convention: "GeneralTF-E",
			dataType:   dataTypeTFE,
			e:          4,
			wantSH:     false,
		},
		{
			name:       "case-insensitive HRsh detection",
			convention: "MyFreeFieldHRsh",
			dataType:   dataTypeTFE,
			e:          9,
			wantSH:     true,
			wantLmax:   2,
			wantOK:     true,
			wantCoeffs: 9,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &File{
				SOFAConventions: tc.convention,
				DataType:        tc.dataType,
				E:               tc.e,
			}
			if got := f.IsSHEncoded(); got != tc.wantSH {
				t.Errorf("IsSHEncoded() = %v, want %v", got, tc.wantSH)
			}
			lmax, ok := f.SHOrder()
			if ok != tc.wantOK {
				t.Errorf("SHOrder() ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && lmax != tc.wantLmax {
				t.Errorf("SHOrder() lmax = %d, want %d", lmax, tc.wantLmax)
			}
			if got := f.SHCoefficientCount(); got != tc.wantCoeffs {
				t.Errorf("SHCoefficientCount() = %d, want %d", got, tc.wantCoeffs)
			}
		})
	}
}

func TestSHDetectionViaHistory(t *testing.T) {
	f := &File{
		SOFAConventions: "FreeFieldHRTF",
		DataType:        dataTypeTFE,
		E:               1156,
		History:         "Converted from miro / Converted to TF / Converted to TFE / Converted to Spherical Harmonics",
	}
	if !f.IsSHEncoded() {
		t.Fatalf("IsSHEncoded() = false, want true (History should pick up SH)")
	}
	lmax, ok := f.SHOrder()
	if !ok || lmax != 33 {
		t.Errorf("SHOrder() = (%d, %v), want (33, true)", lmax, ok)
	}
}

func TestSHWarnings(t *testing.T) {
	cases := []struct {
		name     string
		f        *File
		wantSubs []string // each substring must appear in some warning
		wantNone bool
	}{
		{
			name: "clean SH file produces no warnings",
			f: &File{
				SOFAConventions: "FreeFieldHRTF",
				DataType:        dataTypeTFE,
				E:               1156,
				History:         "Converted to Spherical Harmonics",
			},
			wantNone: true,
		},
		{
			name: "plain HRTF E=1 produces no warnings",
			f: &File{
				SOFAConventions: "SimpleFreeFieldHRTF",
				DataType:        dataTypeTF,
				E:               1,
			},
			wantNone: true,
		},
		{
			name: "convention claims SH but DataType wrong",
			f: &File{
				SOFAConventions: "FreeFieldHRSH",
				DataType:        dataTypeFIR,
				E:               16,
			},
			wantSubs: []string{"DataType is FIR"},
		},
		{
			name: "convention claims SH but E not perfect square",
			f: &File{
				SOFAConventions: "FreeFieldHRSH",
				DataType:        dataTypeTFE,
				E:               7,
			},
			wantSubs: []string{"E is not (L+1)²"},
		},
		{
			name: "false positive: TF-E with perfect-square E but no SH claim",
			f: &File{
				SOFAConventions: "GeneralTF-E",
				DataType:        dataTypeTFE,
				E:               9,
			},
			wantSubs: []string{"perfect square consistent with SH order 2",
				"neither SOFAConventions nor History"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.f.SHWarnings()
			if tc.wantNone {
				if len(got) != 0 {
					t.Errorf("SHWarnings() = %v, want none", got)
				}
				return
			}
			joined := strings.Join(got, " | ")
			for _, sub := range tc.wantSubs {
				if !strings.Contains(joined, sub) {
					t.Errorf("SHWarnings() missing substring %q in %q", sub, joined)
				}
			}
		})
	}
}

// TestWriteSHEncodedRoundTrip writes an SH-encoded TF-E file (E=9 →
// Lmax=2) and verifies the reopened copy is detected as SH. No new
// write code path is exercised — the existing TF-E writer is enough
// because SH is a convention-level interpretation of TF-E.
func TestWriteSHEncodedRoundTrip(t *testing.T) {
	const M, R, E, N = 1, 2, 9, 4

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

	emitterPositions := make([]Vector3, E)
	for e := range E {
		emitterPositions[e] = Vector3{X: float64(e), Y: 0, Z: 0}
	}

	src := &File{
		Conventions:            "SOFA",
		Version:                "2.1",
		SOFAConventions:        "FreeFieldHRSH",
		SOFAConventionsVersion: "1.0",
		DataType:               dataTypeTFE,
		M:                      M,
		R:                      R,
		E:                      E,
		N:                      N,
		Frequencies:            freqs,
		TFRealE:                tfReal,
		TFImagE:                tfImag,
		ListenerPositions:      []Vector3{{0, 0, 0}},
		ReceiverPositions:      []Vector3{{0, 0.09, 0}, {0, -0.09, 0}},
		SourcePositions:        []Vector3{{1, 0, 0}},
		EmitterPositions:       emitterPositions,
	}

	tmp := filepath.Join(t.TempDir(), "sh_roundtrip.sofa")
	if err := src.Save(tmp); err != nil {
		t.Fatalf("Save: %v", err)
	}
	dst, err := Open(tmp)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer dst.Close()

	if dst.SOFAConventions != "FreeFieldHRSH" {
		t.Errorf("SOFAConventions = %q, want FreeFieldHRSH", dst.SOFAConventions)
	}
	if dst.DataType != dataTypeTFE {
		t.Errorf("DataType = %q, want %q", dst.DataType, dataTypeTFE)
	}
	if dst.E != E {
		t.Errorf("E = %d, want %d", dst.E, E)
	}
	if !dst.IsSHEncoded() {
		t.Errorf("IsSHEncoded() = false, want true")
	}
	if lmax, ok := dst.SHOrder(); !ok || lmax != 2 {
		t.Errorf("SHOrder() = (%d, %v), want (2, true)", lmax, ok)
	}
	if got := dst.SHCoefficientCount(); got != 9 {
		t.Errorf("SHCoefficientCount() = %d, want 9", got)
	}
	if w := dst.SHWarnings(); len(w) != 0 {
		t.Errorf("SHWarnings() = %v, want none", w)
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
