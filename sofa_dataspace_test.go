package sofa

import (
	"testing"

	hdf5 "github.com/cwbudde/go-hdf5"
)

func TestParseDataspaceElements(t *testing.T) {
	tests := []struct {
		info   string
		want   uint64
		wantOK bool
	}{
		{"Dataset: float64, 1D array [5], contiguous", 5, true},
		{"Dataset: float64, 2D array [3 x 4], chunked", 12, true},
		{"Dataset: float64, 3D array [2 3 4], contiguous", 24, true},
		{"Dataset: float64, scalar, compact", 1, true},
		{"Dataset: float64, 1D array [0], contiguous", 0, true},
		{"Dataset: float64, 1D array [18446744073709551615], contiguous", maxDataElements + 1, true},
		{"Dataset: float64, 2D array [4294967296 x 4294967296], contiguous", maxDataElements + 1, true},
		{"Dataset: float64, null, contiguous", 0, false},
		{"garbage", 0, false},
	}
	for _, tt := range tests {
		got, ok := parseDataspaceElements(tt.info)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("parseDataspaceElements(%q) = (%d, %v), want (%d, %v)", tt.info, got, ok, tt.want, tt.wantOK)
		}
	}
}

// TestDatasetElementCountMatchesRead checks the dataspace-derived count
// against a real dataset, so a format change in go-hdf5's Info output is
// caught instead of silently disabling the pre-read bound.
func TestDatasetElementCountMatchesRead(t *testing.T) {
	path := writeCraftedFIR(t, map[string]craftedDim{
		"M": named("2"), "R": named("2"), "E": named("1"), "N": {value: 4},
	}, 16)
	h, err := hdf5.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer h.Close()

	for _, child := range h.Root().Children() {
		ds, ok := child.(*hdf5.Dataset)
		if !ok || ds.Name() != "Data.IR" {
			continue
		}
		if n, ok := datasetElementCount(ds); !ok || n != 16 {
			t.Fatalf("datasetElementCount(Data.IR) = (%d, %v), want (16, true)", n, ok)
		}
		return
	}
	t.Fatal("Data.IR not found")
}
