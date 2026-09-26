package sofa

import (
	"testing"

	hdf5 "github.com/cwbudde/go-hdf5"
)

func TestElementCount(t *testing.T) {
	tests := []struct {
		shape []uint64
		want  uint64
	}{
		{[]uint64{5}, 5},
		{[]uint64{3, 4}, 12},
		{[]uint64{2, 3, 4}, 24},
		{[]uint64{}, 1},
		{[]uint64{0}, 0},
		{[]uint64{18446744073709551615}, maxDataElements + 1},
		{[]uint64{4294967296, 4294967296}, maxDataElements + 1},
	}
	for _, tt := range tests {
		if got := elementCount(tt.shape); got != tt.want {
			t.Errorf("elementCount(%v) = %d, want %d", tt.shape, got, tt.want)
		}
	}
}

// TestDatasetElementCountMatchesRead checks the dataspace-derived count
// against a real dataset.
func TestDatasetElementCountMatchesRead(t *testing.T) {
	path := writeCraftedFIR(t, map[string]craftedDim{
		"M": named("2"), "R": named("2"), "E": named("1"), "N": {value: 4},
	}, []uint64{2, 2, 4})
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
