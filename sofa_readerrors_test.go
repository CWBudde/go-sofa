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
// is an error, while unknown attributes are never read at all.
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
		t.Fatalf("setGlobalAttributes read an unknown attribute: %v", err)
	}
	if f.Conventions != "SOFA" || f.RoomVolume != 103.5 {
		t.Errorf("Conventions=%q RoomVolume=%v, want SOFA 103.5", f.Conventions, f.RoomVolume)
	}

	err = (&File{}).setGlobalAttributes([]globalAttribute{{"Title", broken}})
	if !errors.Is(err, errBroken) {
		t.Fatalf("setGlobalAttributes error = %v, want errBroken", err)
	}
	if !strings.Contains(err.Error(), "Title") {
		t.Errorf("error %q does not name the attribute", err)
	}
}
