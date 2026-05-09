package sofa

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// TestSaveValidation tests that Save() properly validates required fields.
func TestSaveValidation(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() *File
		wantErr string
	}{
		{
			name: "missing Conventions",
			setup: func() *File {
				return &File{
					Version:          "1.0",
					SOFAConventions:  "SimpleFreeFieldHRIR",
					DataType:         "FIR",
					M:                1,
					R:                2,
					E:                1,
					N:                64,
					ImpulseResponses: make([][][]float64, 1),
					SamplingRate:     []float64{44100},
				}
			},
			wantErr: "conventions must be \"SOFA\"",
		},
		{
			name: "invalid Conventions value",
			setup: func() *File {
				return &File{
					Conventions:      "HDF5",
					Version:          "1.0",
					SOFAConventions:  "SimpleFreeFieldHRIR",
					DataType:         "FIR",
					M:                1,
					R:                2,
					E:                1,
					N:                64,
					ImpulseResponses: make([][][]float64, 1),
					SamplingRate:     []float64{44100},
				}
			},
			wantErr: "conventions must be \"SOFA\"",
		},
		{
			name: "missing Version",
			setup: func() *File {
				return &File{
					Conventions:      "SOFA",
					SOFAConventions:  "SimpleFreeFieldHRIR",
					DataType:         "FIR",
					M:                1,
					R:                2,
					E:                1,
					N:                64,
					ImpulseResponses: make([][][]float64, 1),
					SamplingRate:     []float64{44100},
				}
			},
			wantErr: "version is required",
		},
		{
			name: "missing SOFAConventions",
			setup: func() *File {
				return &File{
					Conventions:      "SOFA",
					Version:          "1.0",
					DataType:         "FIR",
					M:                1,
					R:                2,
					E:                1,
					N:                64,
					ImpulseResponses: make([][][]float64, 1),
					SamplingRate:     []float64{44100},
				}
			},
			wantErr: "sofaConventions is required",
		},
		{
			name: "missing DataType",
			setup: func() *File {
				return &File{
					Conventions:      "SOFA",
					Version:          "1.0",
					SOFAConventions:  "SimpleFreeFieldHRIR",
					M:                1,
					R:                2,
					E:                1,
					N:                64,
					ImpulseResponses: make([][][]float64, 1),
					SamplingRate:     []float64{44100},
				}
			},
			wantErr: "dataType is required",
		},
		{
			name: "zero M dimension",
			setup: func() *File {
				return &File{
					Conventions:     "SOFA",
					Version:         "1.0",
					SOFAConventions: "SimpleFreeFieldHRIR",
					DataType:        "FIR",
					M:               0,
					R:               2,
					E:               1,
					N:               64,
					SamplingRate:    []float64{44100},
				}
			},
			wantErr: "m must be > 0",
		},
		{
			name: "zero R dimension",
			setup: func() *File {
				return &File{
					Conventions:     "SOFA",
					Version:         "1.0",
					SOFAConventions: "SimpleFreeFieldHRIR",
					DataType:        "FIR",
					M:               1,
					R:               0,
					E:               1,
					N:               64,
					SamplingRate:    []float64{44100},
				}
			},
			wantErr: "r must be > 0",
		},
		{
			name: "IR dimensions mismatch M",
			setup: func() *File {
				return &File{
					Conventions:      "SOFA",
					Version:          "1.0",
					SOFAConventions:  "SimpleFreeFieldHRIR",
					DataType:         "FIR",
					M:                2,
					R:                2,
					E:                1,
					N:                64,
					ImpulseResponses: make([][][]float64, 1), // M=2 but length=1
					SamplingRate:     []float64{44100, 44100},
				}
			},
			wantErr: "ImpulseResponses length 1 does not match M=2",
		},
		{
			name: "IR dimensions mismatch R",
			setup: func() *File {
				f := &File{
					Conventions:      "SOFA",
					Version:          "1.0",
					SOFAConventions:  "SimpleFreeFieldHRIR",
					DataType:         "FIR",
					M:                1,
					R:                2,
					E:                1,
					N:                64,
					ImpulseResponses: make([][][]float64, 1),
					SamplingRate:     []float64{44100},
				}
				f.ImpulseResponses[0] = make([][]float64, 1) // R=2 but length=1
				return f
			},
			wantErr: "ImpulseResponses[0] length 1 does not match R=2",
		},
		{
			name: "IR dimensions mismatch N",
			setup: func() *File {
				f := &File{
					Conventions:      "SOFA",
					Version:          "1.0",
					SOFAConventions:  "SimpleFreeFieldHRIR",
					DataType:         "FIR",
					M:                1,
					R:                2,
					E:                1,
					N:                64,
					ImpulseResponses: make([][][]float64, 1),
					SamplingRate:     []float64{44100},
				}
				f.ImpulseResponses[0] = make([][]float64, 2)
				f.ImpulseResponses[0][0] = make([]float64, 32) // N=64 but length=32
				return f
			},
			wantErr: "ImpulseResponses[0][0] length 32 does not match N=64",
		},
		{
			name: "SamplingRate wrong length",
			setup: func() *File {
				f := &File{
					Conventions:      "SOFA",
					Version:          "1.0",
					SOFAConventions:  "SimpleFreeFieldHRIR",
					DataType:         "FIR",
					M:                2,
					R:                2,
					E:                1,
					N:                64,
					ImpulseResponses: make([][][]float64, 2),
					SamplingRate:     []float64{44100, 44100, 44100}, // M=2 but length=3
				}
				for i := range f.ImpulseResponses {
					f.ImpulseResponses[i] = make([][]float64, 2)
					for j := range f.ImpulseResponses[i] {
						f.ImpulseResponses[i][j] = make([]float64, 64)
					}
				}
				return f
			},
			wantErr: "samplingRate length 3 must be M=2 or 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.setup()
			err := f.validate()
			if err == nil {
				t.Fatalf("validate() succeeded, want error containing %q", tt.wantErr)
			}
			if !contains(err.Error(), tt.wantErr) {
				t.Errorf("validate() error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

// TestSaveMinimal tests creating a minimal valid SOFA file from scratch.
func TestSaveMinimal(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "minimal.sofa")

	// Create minimal valid SOFA file
	f := &File{
		Conventions:            "SOFA",
		Version:                "1.0",
		SOFAConventions:        "SimpleFreeFieldHRIR",
		SOFAConventionsVersion: "1.0",
		DataType:               "FIR",
		M:                      1,
		R:                      2,
		E:                      1,
		N:                      64,
		Title:                  "Minimal Test SOFA",
		SamplingRate:           []float64{44100},
		Delay:                  []float64{0},
	}

	// Create IR data [M=1][R=2][N=64]
	f.ImpulseResponses = make([][][]float64, 1)
	f.ImpulseResponses[0] = make([][]float64, 2)
	for r := 0; r < 2; r++ {
		f.ImpulseResponses[0][r] = make([]float64, 64)
		// Simple impulse at sample 0
		f.ImpulseResponses[0][r][0] = 1.0
	}

	// Add minimal spatial data
	f.ListenerPositions = []Vector3{{0, 0, 0}}
	f.ReceiverPositions = []Vector3{{-0.09, 0, 0}, {0.09, 0, 0}}
	f.SourcePositions = []Vector3{{1, 0, 0}}
	f.EmitterPositions = []Vector3{{0, 0, 0}}
	f.ListenerUp = Vector3{0, 0, 1}
	f.ListenerView = Vector3{1, 0, 0}

	// Save to file
	if err := f.Save(tmpFile); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Verify file exists and has reasonable size
	info, err := os.Stat(tmpFile)
	if err != nil {
		t.Fatalf("Stat() failed: %v", err)
	}
	if info.Size() < 1000 {
		t.Errorf("File size %d too small, expected >1000 bytes", info.Size())
	}

	// Reopen and verify data
	f2, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer f2.Close()

	// Check dimensions
	if f2.M != 1 || f2.R != 2 || f2.E != 1 || f2.N != 64 {
		t.Errorf("Dimensions mismatch: got M=%d R=%d E=%d N=%d, want M=1 R=2 E=1 N=64",
			f2.M, f2.R, f2.E, f2.N)
	}

	// Check metadata
	if f2.Conventions != "SOFA" {
		t.Errorf("Conventions = %q, want \"SOFA\"", f2.Conventions)
	}
	if f2.Version != "1.0" {
		t.Errorf("Version = %q, want \"1.0\"", f2.Version)
	}
	if f2.SOFAConventions != "SimpleFreeFieldHRIR" {
		t.Errorf("SOFAConventions = %q, want \"SimpleFreeFieldHRIR\"", f2.SOFAConventions)
	}
	if f2.Title != "Minimal Test SOFA" {
		t.Errorf("Title = %q, want \"Minimal Test SOFA\"", f2.Title)
	}

	// Check IR data
	if len(f2.ImpulseResponses) != 1 {
		t.Fatalf("ImpulseResponses length = %d, want 1", len(f2.ImpulseResponses))
	}
	if len(f2.ImpulseResponses[0]) != 2 {
		t.Fatalf("ImpulseResponses[0] length = %d, want 2", len(f2.ImpulseResponses[0]))
	}
	for r := 0; r < 2; r++ {
		if len(f2.ImpulseResponses[0][r]) != 64 {
			t.Errorf("ImpulseResponses[0][%d] length = %d, want 64",
				r, len(f2.ImpulseResponses[0][r]))
		}
		if f2.ImpulseResponses[0][r][0] != 1.0 {
			t.Errorf("ImpulseResponses[0][%d][0] = %f, want 1.0",
				r, f2.ImpulseResponses[0][r][0])
		}
	}
}

// TestSaveRoundTrip tests reading, saving, and re-reading real SOFA files.
func TestSaveRoundTrip(t *testing.T) {
	testFiles := []string{
		"testdata/tester.sofa",
		"testdata/MIT_KEMAR_normal_pinna.sofa",
		"testdata/CIPIC_subject_003_hrir_final.sofa",
	}

	for _, testFile := range testFiles {
		t.Run(filepath.Base(testFile), func(t *testing.T) {
			// Open original file
			f1, err := Open(testFile)
			if err != nil {
				t.Fatalf("Open(%s) failed: %v", testFile, err)
			}
			defer f1.Close()

			// Save to temporary file
			tmpFile := filepath.Join(t.TempDir(), "roundtrip.sofa")
			if err := f1.Save(tmpFile); err != nil {
				t.Fatalf("Save() failed: %v", err)
			}

			// Reopen saved file
			f2, err := Open(tmpFile)
			if err != nil {
				t.Fatalf("Open(saved) failed: %v", err)
			}
			defer f2.Close()

			// Compare dimensions
			if f1.M != f2.M || f1.R != f2.R || f1.E != f2.E || f1.N != f2.N {
				t.Errorf("Dimensions mismatch: original M=%d R=%d E=%d N=%d, saved M=%d R=%d E=%d N=%d",
					f1.M, f1.R, f1.E, f1.N, f2.M, f2.R, f2.E, f2.N)
			}

			// Compare metadata
			compareStrings(t, "Conventions", f1.Conventions, f2.Conventions)
			compareStrings(t, "Version", f1.Version, f2.Version)
			compareStrings(t, "SOFAConventions", f1.SOFAConventions, f2.SOFAConventions)
			compareStrings(t, "DataType", f1.DataType, f2.DataType)
			compareStrings(t, "Title", f1.Title, f2.Title)

			// Compare IR data (sample a few points)
			if len(f1.ImpulseResponses) > 0 && len(f1.ImpulseResponses[0]) > 0 {
				for m := 0; m < min(2, f1.M); m++ {
					for r := 0; r < f1.R; r++ {
						for n := 0; n < min(10, f1.N); n++ {
							v1 := f1.ImpulseResponses[m][r][n]
							v2 := f2.ImpulseResponses[m][r][n]
							if math.Abs(v1-v2) > 1e-10 {
								t.Errorf("IR[%d][%d][%d] mismatch: %v != %v", m, r, n, v1, v2)
							}
						}
					}
				}
			}

			// Compare sampling rate
			if len(f1.SamplingRate) > 0 && len(f2.SamplingRate) > 0 {
				if math.Abs(f1.SamplingRate[0]-f2.SamplingRate[0]) > 1e-6 {
					t.Errorf("SamplingRate mismatch: %v != %v", f1.SamplingRate[0], f2.SamplingRate[0])
				}
			}
		})
	}
}

// TestSaveModifyRoundTrip tests modifying a SOFA file and saving changes.
func TestSaveModifyRoundTrip(t *testing.T) {
	// Open test file
	f1, err := Open("testdata/tester.sofa")
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer f1.Close()

	// Modify metadata
	f1.Title = "Modified SOFA File"
	f1.Comment = "Modified by go-sofa test suite"

	// Modify IR data (multiply all samples by 0.5)
	for m := range f1.ImpulseResponses {
		for r := range f1.ImpulseResponses[m] {
			for n := range f1.ImpulseResponses[m][r] {
				f1.ImpulseResponses[m][r][n] *= 0.5
			}
		}
	}

	// Save modified file
	tmpFile := filepath.Join(t.TempDir(), "modified.sofa")
	if err := f1.Save(tmpFile); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Reopen and verify modifications
	f2, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("Open(modified) failed: %v", err)
	}
	defer f2.Close()

	// Check modified metadata
	if f2.Title != "Modified SOFA File" {
		t.Errorf("Title = %q, want \"Modified SOFA File\"", f2.Title)
	}
	if f2.Comment != "Modified by go-sofa test suite" {
		t.Errorf("Comment = %q, want \"Modified by go-sofa test suite\"", f2.Comment)
	}

	// Check modified IR data (sample a few points)
	// Reopen original to compare
	fOrig, err := Open("testdata/tester.sofa")
	if err != nil {
		t.Fatalf("Open(original) failed: %v", err)
	}
	defer fOrig.Close()

	for m := 0; m < min(2, f2.M); m++ {
		for r := 0; r < f2.R; r++ {
			for n := 0; n < min(10, f2.N); n++ {
				expected := fOrig.ImpulseResponses[m][r][n] * 0.5
				actual := f2.ImpulseResponses[m][r][n]
				if math.Abs(expected-actual) > 1e-10 {
					t.Errorf("Modified IR[%d][%d][%d] = %v, want %v (original*0.5)",
						m, r, n, actual, expected)
				}
			}
		}
	}
}

// Helper functions

func compareStrings(t *testing.T, name, v1, v2 string) {
	t.Helper()
	if v1 != v2 {
		t.Errorf("%s mismatch: %q != %q", name, v1, v2)
	}
}
