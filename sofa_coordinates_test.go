package sofa

import (
	"path/filepath"
	"testing"
)

// coordinateTestFile builds a minimal valid SimpleFreeFieldHRIR file whose
// position datasets carry the given coordinate type and units.
func coordinateTestFile(typ, units string) *File {
	return &File{
		Conventions:            "SOFA",
		Version:                "1.0",
		SOFAConventions:        "SimpleFreeFieldHRIR",
		SOFAConventionsVersion: "1.0",
		DataType:               "FIR",
		M:                      2,
		R:                      2,
		E:                      1,
		N:                      4,
		ImpulseResponses: [][][]float64{
			{{1, 0, 0, 0}, {0, 1, 0, 0}},
			{{0, 0, 1, 0}, {0, 0, 0, 1}},
		},
		SamplingRate:          []float64{48000},
		ListenerPositions:     []Vector3{{X: 0, Y: 0, Z: 0}},
		ReceiverPositions:     []Vector3{{X: 0, Y: 0.09, Z: 0}, {X: 0, Y: -0.09, Z: 0}},
		SourcePositions:       []Vector3{{X: 0, Y: 0, Z: 1.5}, {X: 90, Y: 0, Z: 1.5}},
		EmitterPositions:      []Vector3{{X: 0, Y: 0, Z: 0}},
		SourcePositionType:    typ,
		SourcePositionUnits:   units,
		ListenerPositionType:  typ,
		ListenerPositionUnits: units,
		ReceiverPositionType:  CoordinateCartesian,
		ReceiverPositionUnits: UnitsCartesianMetres,
		EmitterPositionType:   CoordinateCartesian,
		EmitterPositionUnits:  UnitsCartesianMetres,
	}
}

// TestPositionCoordinateAttributesRoundTrip checks that the Type and Units
// attributes naming each position dataset's coordinate system survive a
// Save/Open cycle. Without the write half, a caller could not tell spherical
// data from cartesian in a file this package produced.
func TestPositionCoordinateAttributesRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		typ   string
		units string
	}{
		{"spherical degrees", CoordinateSpherical, UnitsSphericalDegrees},
		{"spherical radians", CoordinateSpherical, "radian, radian, metre"},
		{"cartesian", CoordinateCartesian, UnitsCartesianMetres},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "coords.sofa")
			if err := coordinateTestFile(tt.typ, tt.units).Save(path); err != nil {
				t.Fatalf("Save() error = %v", err)
			}

			got, err := Open(path)
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			defer got.Close()

			compareStrings(t, "SourcePositionType", tt.typ, got.SourcePositionType)
			compareStrings(t, "SourcePositionUnits", tt.units, got.SourcePositionUnits)
			compareStrings(t, "ListenerPositionType", tt.typ, got.ListenerPositionType)
			compareStrings(t, "ReceiverPositionType", CoordinateCartesian, got.ReceiverPositionType)
			compareStrings(t, "ReceiverPositionUnits", UnitsCartesianMetres, got.ReceiverPositionUnits)
			compareStrings(t, "EmitterPositionType", CoordinateCartesian, got.EmitterPositionType)
		})
	}
}

// TestPositionCoordinateAttributesWhenEmpty checks that Save refuses a
// position without a Type (AES69 requires one) and omits an empty Units
// rather than writing an empty string, which then reads back empty.
func TestPositionCoordinateAttributesWhenEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-coords.sofa")
	if err := coordinateTestFile("", "").Save(path); err == nil {
		t.Fatal("Save() accepted positions without a Type")
	}

	if err := coordinateTestFile(CoordinateSpherical, "").Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer got.Close()

	if got.SourcePositionUnits != "" {
		t.Errorf("SourcePositionUnits = %q, want empty", got.SourcePositionUnits)
	}
}

// TestPositionCoordinateAttributesTrimmed checks that values are trimmed on
// read but keep their case, so a round trip does not rewrite the file's
// spelling; callers compare them case-insensitively.
func TestPositionCoordinateAttributesTrimmed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mixed-case.sofa")
	if err := coordinateTestFile("  Spherical ", " Degree, degree, metre ").Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer got.Close()

	compareStrings(t, "SourcePositionType", "Spherical", got.SourcePositionType)
	compareStrings(t, "SourcePositionUnits", "Degree, degree, metre", got.SourcePositionUnits)
}

// TestReadRealFileCoordinateType checks the attributes against a real measured
// dataset: CIPIC stores source positions in spherical coordinates.
func TestReadRealFileCoordinateType(t *testing.T) {
	f, err := Open(testdataPath(t, "CIPIC_subject_003_hrir_final.sofa"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer f.Close()

	if f.SourcePositionType != CoordinateSpherical {
		t.Errorf("SourcePositionType = %q, want %q", f.SourcePositionType, CoordinateSpherical)
	}
}
