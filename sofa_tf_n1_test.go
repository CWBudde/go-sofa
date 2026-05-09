package sofa

import (
	"path/filepath"
	"testing"
)

// TestTFRoundTripNEqual1 verifies that go-sofa-written TF files with N==1
// correctly round-trip without misreading as FIR.
func TestTFRoundTripNEqual1(t *testing.T) {
	const M, R, N = 2, 1, 1

	src := &File{
		Conventions:            "SOFA",
		Version:                "2.0",
		SOFAConventions:        "GeneralTF",
		SOFAConventionsVersion: "1.0",
		DataType:               "TF",
		M:                      M,
		R:                      R,
		E:                      1,
		N:                      N,
		Frequencies:            []float64{1000.0},
		TFReal:                 [][][]float64{{{0.5}}, {{0.7}}},
		TFImag:                 [][][]float64{{{0.1}}, {{0.2}}},
		ListenerPositions:      []Vector3{{0, 0, 0}},
		ReceiverPositions:      []Vector3{{0, 0.09, 0}},
		SourcePositions:        []Vector3{{1, 0, 0}, {0, 1, 0}},
		EmitterPositions:       []Vector3{{0, 0, 0}},
	}

	tmp := filepath.Join(t.TempDir(), "tf_n1.sofa")
	if err := src.Save(tmp); err != nil {
		t.Fatalf("Save: %v", err)
	}
	dst, err := Open(tmp)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer dst.Close()

	if dst.DataType != "TF" {
		t.Errorf("DataType = %q, want TF", dst.DataType)
	}
	if dst.N != 1 {
		t.Errorf("N = %d, want 1", dst.N)
	}
	if len(dst.Frequencies) != 1 || dst.Frequencies[0] != 1000.0 {
		t.Errorf("Frequencies = %v, want [1000]", dst.Frequencies)
	}
	if len(dst.TFReal) != M {
		t.Errorf("TFReal M = %d, want %d", len(dst.TFReal), M)
	}
}
