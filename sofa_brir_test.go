package sofa

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// brirTestFile builds a minimal valid SingleRoomDRIR file.
func brirTestFile() *File {
	f := coordinateTestFile(CoordinateCartesian, UnitsCartesianMetres)
	f.SOFAConventions = conventionSingleRoomDRIR
	f.RoomType = "shoebox"
	f.ListenerUp = Vector3{Z: 1}
	f.ListenerView = Vector3{X: 1}
	return f
}

func TestIsBRIR(t *testing.T) {
	tests := []struct {
		convention string
		want       bool
	}{
		{conventionSingleRoomDRIR, true},
		{conventionMultiSpeakerBRIR, true},
		{"SimpleFreeFieldHRIR", false},
		{conventionSingleRoomSRIR, false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.convention, func(t *testing.T) {
			f := &File{SOFAConventions: tt.convention}
			if got := f.IsBRIR(); got != tt.want {
				t.Errorf("IsBRIR() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestBRIRMissingRoomType checks that Save refuses a BRIR file without a
// RoomType and names the attribute in the error.
func TestBRIRMissingRoomType(t *testing.T) {
	for _, convention := range []string{conventionSingleRoomDRIR, conventionMultiSpeakerBRIR} {
		t.Run(convention, func(t *testing.T) {
			f := brirTestFile()
			f.SOFAConventions = convention
			f.RoomType = ""

			err := f.Save(filepath.Join(t.TempDir(), "brir.sofa"))
			if err == nil || !strings.Contains(err.Error(), "RoomType") {
				t.Fatalf("Save() error = %v, want error mentioning RoomType", err)
			}
			if ve := requireValidationError(t, err); ve.Field != "RoomType" {
				t.Errorf("Field = %q, want RoomType", ve.Field)
			}
		})
	}
}

// TestBRIRZeroListenerOrientation checks that Save refuses a BRIR file whose
// listener orientation is undefined, because a binaural response is
// meaningless without knowing where the head points.
func TestBRIRZeroListenerOrientation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(f *File)
	}{
		{"ListenerView", func(f *File) { f.ListenerView = Vector3{} }},
		{"ListenerUp", func(f *File) { f.ListenerUp = Vector3{} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := brirTestFile()
			tt.mutate(f)

			err := f.Save(filepath.Join(t.TempDir(), "brir.sofa"))
			if err == nil || !strings.Contains(err.Error(), tt.name) {
				t.Fatalf("Save() error = %v, want error mentioning %s", err, tt.name)
			}
		})
	}
}

// TestBRIRValidFileSaves checks that a complete BRIR file passes the rules.
func TestBRIRValidFileSaves(t *testing.T) {
	if err := brirTestFile().Save(filepath.Join(t.TempDir(), "brir.sofa")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
}

// TestBRIRRoundTripOfficeII round-trips a real BRIR database file: the Kayser
// 2009 Office II set (SingleRoomDRIR, 8 in-ear and behind-the-ear receivers).
func TestBRIRRoundTripOfficeII(t *testing.T) {
	src, err := Open(testdataPath(t, "OfficeII.sofa"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = src.Close() }()

	if !src.IsBRIR() {
		t.Fatalf("IsBRIR() = false for SOFAConventions %q", src.SOFAConventions)
	}

	path := filepath.Join(t.TempDir(), "office.sofa")
	if err := src.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	dst, err := Open(path)
	if err != nil {
		t.Fatalf("re-Open() error = %v", err)
	}
	defer func() { _ = dst.Close() }()

	if !dst.IsBRIR() {
		t.Errorf("IsBRIR() = false after round-trip")
	}
	compareStrings(t, "SOFAConventions", src.SOFAConventions, dst.SOFAConventions)
	compareStrings(t, "RoomType", src.RoomType, dst.RoomType)
	if dst.ListenerView != src.ListenerView || dst.ListenerUp != src.ListenerUp {
		t.Errorf("listener orientation = (%v, %v), want (%v, %v)",
			dst.ListenerView, dst.ListenerUp, src.ListenerView, src.ListenerUp)
	}
	if !reflect.DeepEqual(dst.ImpulseResponses, src.ImpulseResponses) {
		t.Errorf("ImpulseResponses differ after round-trip")
	}
}
