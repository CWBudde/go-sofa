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

// TestPositionCoordinateAttributesOmittedWhenEmpty checks that an empty type
// or units writes no attribute at all, so that a reader can still distinguish
// "the file does not say" from "the file says cartesian".
func TestPositionCoordinateAttributesOmittedWhenEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-coords.sofa")
	if err := coordinateTestFile("", "").Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer got.Close()

	if got.SourcePositionType != "" {
		t.Errorf("SourcePositionType = %q, want empty", got.SourcePositionType)
	}
	if got.SourcePositionUnits != "" {
		t.Errorf("SourcePositionUnits = %q, want empty", got.SourcePositionUnits)
	}
}

// TestPositionCoordinateAttributesNormalized checks that values are lowercased
// and trimmed on read, so callers can compare against the exported constants
// without normalizing every file's spelling themselves.
func TestPositionCoordinateAttributesNormalized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mixed-case.sofa")
	if err := coordinateTestFile("  Spherical ", " Degree, degree, metre ").Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer got.Close()

	compareStrings(t, "SourcePositionType", CoordinateSpherical, got.SourcePositionType)
	compareStrings(t, "SourcePositionUnits", UnitsSphericalDegrees, got.SourcePositionUnits)
}

// TestReadRealFileCoordinateType checks the attributes against a real measured
// dataset: CIPIC stores source positions in spherical coordinates.
func TestReadRealFileCoordinateType(t *testing.T) {
	f, err := Open("testdata/CIPIC_subject_003_hrir_final.sofa")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer f.Close()

	if f.SourcePositionType != CoordinateSpherical {
		t.Errorf("SourcePositionType = %q, want %q", f.SourcePositionType, CoordinateSpherical)
	}
}
