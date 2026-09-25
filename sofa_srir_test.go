package sofa

import (
	"path/filepath"
	"strings"
	"testing"
)

// srirTestFile builds a minimal valid SingleRoomSRIR file with r receivers.
func srirTestFile(r int) *File {
	f := coordinateTestFile(CoordinateCartesian, UnitsCartesianMetres)
	f.SOFAConventions = conventionSingleRoomSRIR
	f.RoomType = "shoebox"
	f.R = r
	f.ReceiverPositions = []Vector3{{}}
	f.ImpulseResponses = make([][][]float64, f.M)
	for m := range f.ImpulseResponses {
		f.ImpulseResponses[m] = make([][]float64, r)
		for i := range f.ImpulseResponses[m] {
			f.ImpulseResponses[m][i] = make([]float64, f.N)
		}
	}
	return f
}

func TestIsSRIR(t *testing.T) {
	for convention, want := range map[string]bool{
		conventionSingleRoomSRIR:     true,
		conventionSingleRoomMIMOSRIR: true,
		conventionSingleRoomDRIR:     false,
		"SimpleFreeFieldHRIR":        false,
	} {
		f := &File{SOFAConventions: convention}
		if got := f.IsSRIR(); got != want {
			t.Errorf("IsSRIR() for %q = %v, want %v", convention, got, want)
		}
	}
}

// TestAmbisonicsOrder checks the order detected from R = (order+1)², and that
// only SRIR files report one: a 4-receiver BRIR is not first-order Ambisonics.
func TestAmbisonicsOrder(t *testing.T) {
	tests := []struct {
		name       string
		convention string
		r          int
		wantOrder  int
		wantOK     bool
	}{
		{"order 0", conventionSingleRoomSRIR, 1, 0, true},
		{"order 1", conventionSingleRoomSRIR, 4, 1, true},
		{"order 2", conventionSingleRoomSRIR, 9, 2, true},
		{"order 3 MIMO", conventionSingleRoomMIMOSRIR, 16, 3, true},
		{"raw 32-capsule array", conventionSingleRoomSRIR, 32, 0, false},
		{"not SRIR", conventionSingleRoomDRIR, 4, 0, false},
		{"no receivers", conventionSingleRoomSRIR, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{SOFAConventions: tt.convention, R: tt.r}
			order, ok := f.AmbisonicsOrder()
			if order != tt.wantOrder || ok != tt.wantOK {
				t.Errorf("AmbisonicsOrder() = (%d, %v), want (%d, %v)", order, ok, tt.wantOrder, tt.wantOK)
			}
		})
	}
}

// TestSRIRWarnings checks the advisory messages for missing room metadata and
// for a receiver count that is no Ambisonics order.
func TestSRIRWarnings(t *testing.T) {
	tests := []struct {
		name  string
		file  *File
		wants []string
	}{
		{
			"complete",
			&File{SOFAConventions: conventionSingleRoomSRIR, R: 4, RoomVolume: 100, RoomTemperature: 293.15},
			nil,
		},
		{
			"missing room metadata",
			&File{SOFAConventions: conventionSingleRoomSRIR, R: 4},
			[]string{"RoomVolume", "RoomTemperature"},
		},
		{
			"raw capsules",
			&File{SOFAConventions: conventionSingleRoomSRIR, R: 32, RoomVolume: 100, RoomTemperature: 293.15},
			[]string{"R=32"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.file.ConventionWarnings()
			if len(got) != len(tt.wants) {
				t.Fatalf("ConventionWarnings() = %q, want %d messages", got, len(tt.wants))
			}
			for i, want := range tt.wants {
				if !strings.Contains(got[i], want) {
					t.Errorf("warning %d = %q, want it to mention %q", i, got[i], want)
				}
			}
		})
	}
}

// TestSRIRWarningsDoNotBlockSave checks that SRIR checks are advisory only.
func TestSRIRWarningsDoNotBlockSave(t *testing.T) {
	f := srirTestFile(32)
	if len(f.ConventionWarnings()) == 0 {
		t.Fatal("expected warnings for the test file")
	}
	if err := f.Save(filepath.Join(t.TempDir(), "srir.sofa")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
}

// TestSRIRReadKnownFile opens the SOFA Toolbox SRIR demo file. It has a
// single omnidirectional receiver (order 0) and no room volume or temperature.
func TestSRIRReadKnownFile(t *testing.T) {
	f, err := Open("testdata/SingleRoomSRIR_1.1.sofa")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = f.Close() }()

	if !f.IsSRIR() {
		t.Fatalf("IsSRIR() = false for SOFAConventions %q", f.SOFAConventions)
	}
	if order, ok := f.AmbisonicsOrder(); order != 0 || !ok {
		t.Errorf("AmbisonicsOrder() = (%d, %v), want (0, true)", order, ok)
	}
	if got := f.ConventionWarnings(); len(got) != 2 {
		t.Errorf("ConventionWarnings() = %q, want RoomVolume and RoomTemperature warnings", got)
	}
}

// TestRoomVolumeTemperatureRoundTrip checks that RoomVolume and
// RoomTemperature survive Save/Open, and are absent when zero.
func TestRoomVolumeTemperatureRoundTrip(t *testing.T) {
	tests := []struct {
		name         string
		volume, temp float64
	}{
		{"set", 103.5, 293.15},
		{"absent", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := srirTestFile(4)
			src.RoomVolume = tt.volume
			src.RoomTemperature = tt.temp

			path := filepath.Join(t.TempDir(), "room.sofa")
			if err := src.Save(path); err != nil {
				t.Fatalf("Save() error = %v", err)
			}
			dst, err := Open(path)
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			defer func() { _ = dst.Close() }()

			if dst.RoomVolume != tt.volume || dst.RoomTemperature != tt.temp {
				t.Errorf("room = (%g, %g), want (%g, %g)", dst.RoomVolume, dst.RoomTemperature, tt.volume, tt.temp)
			}
		})
	}
}

// TestParseRoomAttribute checks the parser for RoomVolume/RoomTemperature
// stored as root attributes (SimpleFreeFieldSOS files do this for RoomVolume).
func TestParseRoomAttribute(t *testing.T) {
	tests := []struct {
		in   string
		want float64
	}{
		{"103", 103},
		{" 293.5 ", 293.5},
		{"", 0},
		{"n/a", 0},
		{"NaN", 0},
		{"+Inf", 0},
	}
	for _, tt := range tests {
		if got := parseRoomAttribute(tt.in); got != tt.want {
			t.Errorf("parseRoomAttribute(%q) = %g, want %g", tt.in, got, tt.want)
		}
	}
}
