package sofa

import (
	"errors"
	"strings"
	"testing"

	hdf5 "github.com/cwbudde/go-hdf5"
)

// TestOpenPropagatesPositionReadError checks that a position dataset of the
// right shape but a datatype go-hdf5 cannot convert (int16) fails Open
// instead of leaving the positions silently empty.
func TestOpenPropagatesPositionReadError(t *testing.T) {
	spec := firSpec()
	spec.extra = func(t *testing.T, fw *hdf5.FileWriter) {
		t.Helper()
		ds, err := fw.CreateDataset("/ReceiverPosition", hdf5.Int16, []uint64{2, 3})
		if err != nil {
			t.Fatalf("create ReceiverPosition: %v", err)
		}
		if err := ds.Write([]int16{1, 2, 3, 4, 5, 6}); err != nil {
			t.Fatalf("write ReceiverPosition: %v", err)
		}
	}
	f, err := Open(writeCraftedSpec(t, spec))
	if err == nil {
		f.Close()
		t.Fatal("Open accepted an unreadable ReceiverPosition")
	}
	if !strings.Contains(err.Error(), "ReceiverPosition") {
		t.Errorf("error %q does not name ReceiverPosition", err)
	}
}

// TestSetGlobalAttributes checks that a known attribute that cannot be read
// is an error, an unknown one is listed in Dropped, and netCDF's own
// attributes are never read at all.
func TestSetGlobalAttributes(t *testing.T) {
	errBroken := errors.New("broken attribute")
	value := func(v string) func() (interface{}, error) {
		return func() (interface{}, error) { return v, nil }
	}
	broken := func() (interface{}, error) { return nil, errBroken }

	var f File
	err := f.setGlobalAttributes([]globalAttribute{
		{"Conventions", value("SOFA")},
		{"_NCProperties", broken},
		{"MyApplicationAttribute", broken},
		{"RoomVolume", value("103.5")},
	})
	if err != nil {
		t.Fatalf("setGlobalAttributes failed on an unknown attribute: %v", err)
	}
	if f.Conventions != "SOFA" || f.RoomVolume != 103.5 {
		t.Errorf("Conventions=%q RoomVolume=%v, want SOFA 103.5", f.Conventions, f.RoomVolume)
	}
	if len(f.Dropped) != 1 || !strings.Contains(f.Dropped[0], "MyApplicationAttribute") {
		t.Errorf("Dropped = %q, want only MyApplicationAttribute", f.Dropped)
	}
	if len(f.Attributes) != 0 {
		t.Errorf("Attributes = %v, want none", f.Attributes)
	}

	err = (&File{}).setGlobalAttributes([]globalAttribute{{"Title", broken}})
	if !errors.Is(err, errBroken) {
		t.Fatalf("setGlobalAttributes error = %v, want errBroken", err)
	}
	if !strings.Contains(err.Error(), "Title") {
		t.Errorf("error %q does not name the attribute", err)
	}
}

// TestIRAtRejectsIncompleteRow checks that an in-memory FIR whose selected
// row is nil or shorter than N reports ErrIndexOutOfRange instead of
// returning the partial row (and IRPeakdB then reporting silence).
func TestIRAtRejectsIncompleteRow(t *testing.T) {
	for _, row := range [][]float64{nil, {}, {0.5, 0.25}} {
		f := &File{DataType: dataTypeFIR, M: 1, R: 1, N: 4, ImpulseResponses: [][][]float64{{row}}}
		if _, err := f.IRAt(0, 0); !errors.Is(err, ErrIndexOutOfRange) {
			t.Errorf("IRAt with %d of 4 samples: error = %v, want ErrIndexOutOfRange", len(row), err)
		}
		if _, err := f.IRPeakdB(0, 0); !errors.Is(err, ErrIndexOutOfRange) {
			t.Errorf("IRPeakdB with %d of 4 samples: error = %v, want ErrIndexOutOfRange", len(row), err)
		}
	}
}

// TestOpenRejectsRank2FrequencyVector checks that the /N coordinate of a TF
// file must be one-dimensional: a [2,2] /N holding N=4 values is rejected
// rather than read as the frequency vector.
func TestOpenRejectsRank2FrequencyVector(t *testing.T) {
	spec := craftedSpec{
		dataType: dataTypeTF,
		dims:     map[string]int{dimM: 1, dimR: 1, dimE: 1, dimC: 3, dimI: 1},
		vars: map[string]craftedVar{
			"Data.Real": {shape: []uint64{1, 1, 4}},
			"Data.Imag": {shape: []uint64{1, 1, 4}},
		},
		extra: func(t *testing.T, fw *hdf5.FileWriter) {
			t.Helper()
			ds, err := fw.CreateDataset("/N", hdf5.Float64, []uint64{2, 2})
			if err != nil {
				t.Fatalf("create /N: %v", err)
			}
			if err := ds.Write([]float64{100, 200, 400, 800}); err != nil {
				t.Fatalf("write /N: %v", err)
			}
		},
	}
	f, err := Open(writeCraftedSpec(t, spec))
	if err == nil {
		f.Close()
		t.Fatal("Open accepted a [2,2] /N frequency vector")
	}
	if !strings.Contains(err.Error(), "N: shape [2 2]") {
		t.Errorf("error %q does not report the /N shape", err)
	}
}
