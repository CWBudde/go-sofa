package sofa

import (
	"errors"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

// TestUnknownConventionStillReads checks that a convention with no entry in
// conventionRegistry gets only the generic checks: Save accepts it and Open
// reads it back unchanged.
func TestUnknownConventionStillReads(t *testing.T) {
	src := coordinateTestFile(CoordinateCartesian, UnitsCartesianMetres)
	src.SOFAConventions = "MyCustomConvention"
	if _, ok := conventionRegistry[src.SOFAConventions]; ok {
		t.Fatalf("test convention %q must not be registered", src.SOFAConventions)
	}

	path := filepath.Join(t.TempDir(), "custom.sofa")
	if err := src.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	dst, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = dst.Close() }()

	if dst.SOFAConventions != src.SOFAConventions {
		t.Errorf("SOFAConventions = %q, want %q", dst.SOFAConventions, src.SOFAConventions)
	}
	if !reflect.DeepEqual(dst.ImpulseResponses, src.ImpulseResponses) {
		t.Errorf("ImpulseResponses = %v, want %v", dst.ImpulseResponses, src.ImpulseResponses)
	}
}

// TestFIREConventionsUnregistered checks that conventions whose DataType is
// FIR-E or FIRE have no rules: Open rejects both, so such rules could never
// apply.
func TestFIREConventionsUnregistered(t *testing.T) {
	for _, name := range []string{"MultiSpeakerBRIR", "SingleRoomMIMOSRIR"} {
		if _, ok := conventionRegistry[name]; ok {
			t.Errorf("conventionRegistry has rules for %s, whose DataType Open rejects", name)
		}
	}
}

// TestConventionRulesDispatch checks that validate runs the rules registered
// for the file's SOFAConventions, and only those.
func TestConventionRulesDispatch(t *testing.T) {
	errRule := errors.New("rule rejected file")
	calls := 0
	registerConventionForTest(t, "TestConventionReject", conventionRules{
		validate: func(*File) error {
			calls++
			return errRule
		},
	})
	registerConventionForTest(t, "TestConventionNoValidator", conventionRules{})

	tests := []struct {
		name       string
		convention string
		wantErr    error
		wantCalls  int
	}{
		{"registered rule error propagates", "TestConventionReject", errRule, 1},
		{"nil validator is a no-op", "TestConventionNoValidator", nil, 0},
		{"unregistered convention is a no-op", "TestConventionUnregistered", nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls = 0
			f := coordinateTestFile(CoordinateCartesian, UnitsCartesianMetres)
			f.SOFAConventions = tt.convention

			err := f.Save(filepath.Join(t.TempDir(), "dispatch.sofa"))
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Save() error = %v, want %v", err, tt.wantErr)
			}
			if calls != tt.wantCalls {
				t.Errorf("rule called %d times, want %d", calls, tt.wantCalls)
			}
		})
	}
}

// registerConventionForTest adds rules to conventionRegistry for the duration
// of the test.
func registerConventionForTest(t *testing.T, name string, rules conventionRules) {
	t.Helper()
	prev, had := conventionRegistry[name]
	conventionRegistry[name] = rules
	t.Cleanup(func() {
		if had {
			conventionRegistry[name] = prev
		} else {
			delete(conventionRegistry, name)
		}
	})
}

// TestConventionWarnings checks that ConventionWarnings returns the advisory
// messages of the rules registered for the file's SOFAConventions, and nothing
// for conventions without a warnings function.
func TestConventionWarnings(t *testing.T) {
	registerConventionForTest(t, "TestConventionWarns", conventionRules{
		warnings: func(*File) []string { return []string{"first", "second"} },
	})
	registerConventionForTest(t, "TestConventionSilent", conventionRules{})

	tests := []struct {
		convention string
		want       []string
	}{
		{"TestConventionWarns", []string{"first", "second"}},
		{"TestConventionSilent", nil},
		{"TestConventionUnregistered", nil},
	}

	for _, tt := range tests {
		t.Run(tt.convention, func(t *testing.T) {
			f := &File{SOFAConventions: tt.convention}
			if got := f.ConventionWarnings(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ConventionWarnings() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSaveConventionMandatoryGlobals checks that Save writes the global
// attributes a convention's CSV table makes mandatory, as the empty default
// the table gives them when the File has none, and keeps a value the File
// sets. The File is not changed.
func TestSaveConventionMandatoryGlobals(t *testing.T) {
	for _, tc := range []struct {
		convention string
		set        []Attribute
		want       []Attribute
	}{
		{"SimpleFreeFieldHRIR", nil, []Attribute{{"DatabaseName", ""}, {"ListenerShortName", ""}}},
		{"SimpleFreeFieldHRIR", []Attribute{{"DatabaseName", "ARI"}}, []Attribute{{"DatabaseName", "ARI"}, {"ListenerShortName", ""}}},
		{"SimpleHeadphoneIR", nil, []Attribute{
			{"DatabaseName", ""}, {"EmitterDescription", ""}, {"ListenerShortName", ""}, {"ReceiverDescription", ""},
		}},
		{"GeneralFIR", nil, nil},
	} {
		t.Run(tc.convention, func(t *testing.T) {
			f := minimalFIRFile()
			f.SOFAConventions = tc.convention
			f.Attributes = tc.set
			back := roundTrip(t, f)
			if !reflect.DeepEqual(back.Attributes, tc.want) {
				t.Errorf("Attributes = %q, want %q", back.Attributes, tc.want)
			}
			if !reflect.DeepEqual(f.Attributes, tc.set) {
				t.Errorf("Save changed the File's Attributes to %q", f.Attributes)
			}
		})
	}
}

// tfFileTwoReceivers returns minimalTFFile with the two receivers the
// Simple* conventions require.
func tfFileTwoReceivers() *File {
	f := minimalTFFile()
	f.R = 2
	for m := range f.M {
		f.TFReal[m] = append(f.TFReal[m], make([]float64, f.N))
		f.TFImag[m] = append(f.TFImag[m], make([]float64, f.N))
	}
	return f
}

// TestSaveConventionDataType checks that Save accepts every official
// convention with the one DataType its CSV table allows and rejects any
// other with a DataType *ValidationError.
func TestSaveConventionDataType(t *testing.T) {
	for _, tc := range []struct {
		convention  string
		valid       func() *File
		wrongLayout func() *File // data of another DataType
	}{
		{"GeneralFIR", minimalFIRFile, minimalTFFile},
		{"GeneralTF", minimalTFFile, minimalFIRFile},
		{"GeneralTF-E", minimalTFEFile, minimalTFFile},
		{"SimpleFreeFieldHRIR", minimalFIRFile, tfFileTwoReceivers},
		{"SimpleFreeFieldHRTF", tfFileTwoReceivers, minimalFIRFile},
		{"SimpleFreeFieldHRSOS", minimalSOSFile, minimalFIRFile},
		{"SimpleFreeFieldSOS", minimalSOSFile, minimalFIRFile},
		{"FreeFieldHRTF", minimalTFEFile, tfFileTwoReceivers},
		{"SimpleHeadphoneIR", minimalFIRFile, minimalSOSFile},
		{"SingleRoomSRIR", func() *File { return srirTestFile(4) }, minimalTFFile},
		{"SingleRoomDRIR", brirTestFile, minimalTFFile},
		{"FreeFieldDirectivityTF", minimalTFFile, minimalFIRFile},
	} {
		t.Run(tc.convention, func(t *testing.T) {
			f := tc.valid()
			f.SOFAConventions = tc.convention
			if err := f.Save(filepath.Join(t.TempDir(), "valid.sofa")); err != nil {
				t.Errorf("Save of %s data: %v", f.DataType, err)
			}

			f = tc.wrongLayout()
			f.SOFAConventions = tc.convention
			err := f.Save(filepath.Join(t.TempDir(), "wrong.sofa"))
			var verr *ValidationError
			if !errors.As(err, &verr) || verr.Field != "DataType" {
				t.Errorf("Save of %s data: %v, want a DataType ValidationError", f.DataType, err)
			}
		})
	}
}

// TestSaveLegacySimpleFreeFieldSOS checks that the SOFA 1.0 name
// SimpleFreeFieldSOS is validated like SimpleFreeFieldHRSOS: R=2 and its
// mandatory global attributes, and errors name the file's own convention.
func TestSaveLegacySimpleFreeFieldSOS(t *testing.T) {
	f := minimalSOSFile()
	f.SOFAConventions = "SimpleFreeFieldSOS"
	f.R = 1
	f.SOSCoefficients[0] = f.SOSCoefficients[0][:1]
	err := f.Save(filepath.Join(t.TempDir(), "r1.sofa"))
	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Field != "R" {
		t.Errorf("Save with R=1: %v, want an R ValidationError", err)
	}

	f = minimalSOSFile()
	f.SOFAConventions = "SimpleFreeFieldSOS"
	back := roundTrip(t, f)
	want := []Attribute{{"DatabaseName", ""}, {"ListenerShortName", ""}}
	if !reflect.DeepEqual(back.Attributes, want) {
		t.Errorf("Attributes = %q, want %q", back.Attributes, want)
	}
}

// sourceOrientationDefault returns the variable Save writes for a missing
// SourceView or SourceUp.
func sourceOrientationDefault(name string, values ...float64) Variable {
	return Variable{
		Name: name, Dims: []string{"I", "C"}, Shape: []int{1, 3}, Values: values,
		Attributes: []Attribute{{"Type", "cartesian"}, {"Units", "metre"}},
	}
}

// variablesByName indexes vars by name, failing on a duplicate.
func variablesByName(t *testing.T, vars []Variable) map[string]Variable {
	t.Helper()
	byName := map[string]Variable{}
	for _, v := range vars {
		if _, dup := byName[v.Name]; dup {
			t.Errorf("variable %s written twice", v.Name)
		}
		byName[v.Name] = v
	}
	return byName
}

// TestSaveConventionMandatoryVariables checks that Save writes SourceView
// and SourceUp, mandatory in SingleRoomSRIR, SingleRoomDRIR and
// FreeFieldDirectivityTF, with the default of the convention's CSV table
// when Variables lacks them, keeps a caller's own, and leaves the File
// unchanged.
func TestSaveConventionMandatoryVariables(t *testing.T) {
	up := sourceOrientationDefault("SourceUp", 0, 0, 1)
	for _, tc := range []struct {
		convention string
		build      func() *File
		want       []Variable // nil: no source orientation is written
	}{
		{"SingleRoomSRIR", func() *File { return srirTestFile(4) }, []Variable{sourceOrientationDefault("SourceView", 1, 0, 0), up}},
		{"SingleRoomDRIR", brirTestFile, []Variable{sourceOrientationDefault("SourceView", -1, 0, 0), up}},
		{"FreeFieldDirectivityTF", minimalTFFile, []Variable{sourceOrientationDefault("SourceView", 1, 0, 0), up}},
		{"SimpleFreeFieldHRIR", minimalFIRFile, nil},
		{"GeneralFIR", minimalFIRFile, nil},
	} {
		t.Run(tc.convention, func(t *testing.T) {
			f := tc.build()
			f.SOFAConventions = tc.convention
			got := variablesByName(t, roundTrip(t, f).Variables)
			for _, w := range tc.want {
				if !reflect.DeepEqual(got[w.Name], w) {
					t.Errorf("%s = %+v, want %+v", w.Name, got[w.Name], w)
				}
			}
			if tc.want == nil && len(got) > 0 {
				t.Errorf("Variables = %+v, want none", got)
			}
			if f.Variables != nil {
				t.Errorf("Save changed the File's Variables to %+v", f.Variables)
			}
		})
	}

	t.Run("caller SourceView kept", func(t *testing.T) {
		f := srirTestFile(4)
		own := Variable{
			Name: "SourceView", Dims: []string{"M", "C"}, Shape: []int{f.M, 3},
			Values: []float64{0, 1, 0, 0, -1, 0}, Attributes: []Attribute{{"Type", "cartesian"}, {"Units", "metre"}},
		}
		f.Variables = []Variable{own}
		got := variablesByName(t, roundTrip(t, f).Variables)
		if !reflect.DeepEqual(got["SourceView"], own) {
			t.Errorf("SourceView = %+v, want the caller's %+v", got["SourceView"], own)
		}
		if !reflect.DeepEqual(got["SourceUp"], up) {
			t.Errorf("SourceUp = %+v, want %+v", got["SourceUp"], up)
		}
		if len(f.Variables) != 1 || cap(f.Variables) != 1 {
			t.Errorf("Save changed the File's Variables to %+v", f.Variables)
		}
	})

	t.Run("VariableAttributes on a default", func(t *testing.T) {
		f := srirTestFile(4)
		f.VariableAttributes = map[string][]Attribute{"SourceView": {{"Reference", "center"}}}
		got := variablesByName(t, roundTrip(t, f).Variables)
		want := sourceOrientationDefault("SourceView", 1, 0, 0)
		want.Attributes = append(want.Attributes, Attribute{"Reference", "center"})
		if !reflect.DeepEqual(got["SourceView"], want) {
			t.Errorf("SourceView = %+v, want %+v", got["SourceView"], want)
		}
		if !reflect.DeepEqual(got["SourceUp"], up) {
			t.Errorf("SourceUp = %+v, want %+v", got["SourceUp"], up)
		}
	})

	t.Run("VariableAttributes may not override Type", func(t *testing.T) {
		f := srirTestFile(4)
		f.VariableAttributes = map[string][]Attribute{"SourceView": {{"Type", "spherical"}}}
		err := f.Save(filepath.Join(t.TempDir(), "srir.sofa"))
		if ve := requireValidationError(t, err); ve.Field != "VariableAttributes" {
			t.Errorf("Field = %q, want VariableAttributes", ve.Field)
		}
	})
}

// TestSaveDRIRMandatoryGlobals checks that Save writes RoomDescription and
// DatabaseName, mandatory in SingleRoomDRIR, as "" when the File lacks them.
func TestSaveDRIRMandatoryGlobals(t *testing.T) {
	back := roundTrip(t, brirTestFile())
	want := []Attribute{{"DatabaseName", ""}, {"RoomDescription", ""}}
	if !reflect.DeepEqual(back.Attributes, want) {
		t.Errorf("Attributes = %q, want %q", back.Attributes, want)
	}
}

// TestSaveConventionRoomType checks the RoomType Save writes when the File
// has none: shoebox for SingleRoomSRIR only when both room corners are
// given (the corners define the shoebox), reverberant for SingleRoomDRIR,
// and free field otherwise. An explicit RoomType is written as is, and the
// File is not changed.
func TestSaveConventionRoomType(t *testing.T) {
	// A room corner is one cartesian point in metres, as sofar and the
	// interop generator write it.
	corner := func(name string) Variable {
		return Variable{
			Name: name, Dims: []string{"I", "C"}, Shape: []int{1, 3}, Values: []float64{0, 0, 0},
			Attributes: []Attribute{{"Type", "cartesian"}, {"Units", "metre"}},
		}
	}
	srir := func(roomType string, corners ...string) func() *File {
		return func() *File {
			f := srirTestFile(4)
			f.RoomType = roomType
			for _, name := range corners {
				f.Variables = append(f.Variables, corner(name))
			}
			return f
		}
	}
	drir := func(roomType string) func() *File {
		return func() *File {
			f := brirTestFile()
			f.RoomType = roomType
			return f
		}
	}
	for _, tc := range []struct {
		name  string
		build func() *File
		want  string
	}{
		{"SRIR without corners", srir(""), "free field"},
		{"SRIR with RoomCornerA only", srir("", "RoomCornerA"), "free field"},
		{"SRIR with both corners", srir("", "RoomCornerA", "RoomCornerB"), "shoebox"},
		{"SRIR explicit", srir("dae"), "dae"},
		{"SRIR explicit with corners", srir("reverberant", "RoomCornerA", "RoomCornerB"), "reverberant"},
		{"DRIR", drir(""), "reverberant"},
		{"DRIR explicit", drir("free field"), "free field"},
		{"SimpleFreeFieldHRIR", minimalFIRFile, "free field"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := tc.build()
			set := f.RoomType
			if got := roundTrip(t, f).RoomType; got != tc.want {
				t.Errorf("RoomType = %q, want %q", got, tc.want)
			}
			if f.RoomType != set {
				t.Errorf("Save changed the File's RoomType to %q", f.RoomType)
			}
		})
	}
}

// TestConventionVersionWarnings checks that ConventionWarnings flags a
// SOFAConventionsVersion the file's registered convention does not know,
// checking a legacy name against its own versions, and never a custom
// convention's or a missing one (Save rejects that).
func TestConventionVersionWarnings(t *testing.T) {
	for _, tc := range []struct {
		name, convention, version string
		want                      []string
	}{
		{"unknown", "SimpleFreeFieldHRIR", "9.9", []string{
			`SOFAConventionsVersion "9.9" is not a known version of SimpleFreeFieldHRIR (known: 0.4, 1.0, 1.1, 1.2)`,
		}},
		{"GeneralTF 2.0", "GeneralTF", "2.0", nil},
		{"GeneralTF 1.1", "GeneralTF", "1.1", []string{
			`SOFAConventionsVersion "1.1" is not a known version of GeneralTF (known: 1.0, 2.0)`,
		}},
		{"custom", "MyCustomConvention", "9.9", nil},
		{"legacy alias 1.0", "SimpleFreeFieldSOS", "1.0", nil},
		{"legacy alias 1.1", "SimpleFreeFieldSOS", "1.1", []string{
			`SOFAConventionsVersion "1.1" is not a known version of SimpleFreeFieldSOS (known: 1.0)`,
		}},
		{"alias target 1.1", "SimpleFreeFieldHRSOS", "1.1", nil},
		{"missing", "SimpleFreeFieldHRIR", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &File{Version: "2.1", SOFAConventions: tc.convention, SOFAConventionsVersion: tc.version}
			if got := f.ConventionWarnings(); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ConventionWarnings() = %q, want %q", got, tc.want)
			}
		})
	}

	known := map[string][]string{
		"SimpleFreeFieldHRIR":    {"0.4", "1.0", "1.1", "1.2"},
		"SimpleFreeFieldHRTF":    {"1.0", "1.1", "1.2"},
		"SimpleHeadphoneIR":      {"0.1", "0.2", "1.0", "1.1"},
		"SingleRoomDRIR":         {"0.1", "0.2", "0.3"},
		"FreeFieldDirectivityTF": {"1.0", "1.1"},
	}
	for convention, versions := range known {
		for _, version := range versions {
			f := &File{Version: "2.1", SOFAConventions: convention, SOFAConventionsVersion: version}
			if got := f.ConventionWarnings(); got != nil {
				t.Errorf("%s %s: ConventionWarnings() = %q, want none", convention, version, got)
			}
		}
	}
}

// TestConventionVersionsRegistered checks that every registered convention
// and legacy name lists the SOFAConventionsVersion values it knows.
func TestConventionVersionsRegistered(t *testing.T) {
	names := slices.Collect(maps.Keys(conventionRegistry))
	names = append(names, slices.Collect(maps.Keys(conventionAliases))...)
	for _, name := range names {
		if rules, _ := rulesFor(name); len(rules.versions) == 0 {
			t.Errorf("%s lists no known SOFAConventionsVersion", name)
		}
	}
}

// TestConventionSOFA2Warnings checks that ConventionWarnings flags SOFA 2.x
// features (the FreeFieldHRTF convention, DataType TF-E, spherical-harmonics
// positions) in a file whose Version is below 2.0, for any convention, and
// not in a SOFA 2.x file or one whose Version is no number.
func TestConventionSOFA2Warnings(t *testing.T) {
	sh := func(f *File) {
		f.EmitterPositions, f.EmitterPositionType = []Vector3{{}}, " Spherical Harmonics "
	}
	for _, tc := range []struct {
		name, version, convention, dataType string
		modify                              func(*File)
		want                                []string
	}{
		{"FreeFieldHRTF", "1.0", "FreeFieldHRTF", "TF-E", nil, []string{
			`FreeFieldHRTF is a SOFA 2.x convention, but Version is "1.0"`,
			`DataType TF-E is a SOFA 2.x feature, but Version is "1.0"`,
		}},
		{"GeneralTF-E", "1.0", "GeneralTF-E", "TF-E", nil, []string{
			`DataType TF-E is a SOFA 2.x feature, but Version is "1.0"`,
		}},
		{"TF-E in a custom convention", "0.6", "MyCustomConvention", "TF-E", nil, []string{
			`DataType TF-E is a SOFA 2.x feature, but Version is "0.6"`,
		}},
		{"SH emitters", "1.0", "GeneralTF", "TF", sh, []string{
			`EmitterPosition Type " Spherical Harmonics " is a SOFA 2.x feature, but Version is "1.0"`,
		}},
		{"SH receivers", "1.0", "GeneralTF", "TF", func(f *File) {
			f.ReceiverPositions, f.ReceiverPositionType = []Vector3{{}}, CoordinateSphericalHarmonics
		}, []string{
			`ReceiverPosition Type "spherical harmonics" is a SOFA 2.x feature, but Version is "1.0"`,
		}},
		{"SOS in SimpleFreeFieldSOS", "1.0", "SimpleFreeFieldSOS", "SOS", nil, nil},
		{"FIR", "1.0", "SimpleFreeFieldHRIR", "FIR", nil, nil},
		{"SOFA 2.1", "2.1", "FreeFieldHRTF", "TF-E", sh, nil},
		{"SOFA 2.0", "2.0", "GeneralTF-E", "TF-E", sh, nil},
		{"unparsable Version", "one", "FreeFieldHRTF", "TF-E", sh, nil},
		{"empty Version", "", "FreeFieldHRTF", "TF-E", sh, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &File{Version: tc.version, SOFAConventions: tc.convention, SOFAConventionsVersion: "1.0", DataType: tc.dataType}
			if tc.modify != nil {
				tc.modify(f)
			}
			if got := f.ConventionWarnings(); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ConventionWarnings() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestConventionVersionWarningsDoNotBlockSave checks that version warnings
// are advisory only: a SOFA 1.0 FreeFieldHRTF file with an unknown
// SOFAConventionsVersion saves and reads back as written.
func TestConventionVersionWarningsDoNotBlockSave(t *testing.T) {
	f := minimalTFEFile()
	f.SOFAConventions, f.SOFAConventionsVersion, f.Version = "FreeFieldHRTF", "9.9", "1.0"
	f.EmitterPositions = []Vector3{{0, 0, 0}, {1, -1, 0}}
	f.EmitterPositionType = CoordinateSphericalHarmonics
	if got := f.ConventionWarnings(); len(got) != 4 {
		t.Fatalf("ConventionWarnings() = %q, want 4 warnings", got)
	}
	back := roundTrip(t, f)
	if back.Version != "1.0" || back.SOFAConventionsVersion != "9.9" {
		t.Errorf("Version, SOFAConventionsVersion = %q, %q, want 1.0, 9.9", back.Version, back.SOFAConventionsVersion)
	}
	if got := back.ConventionWarnings(); len(got) != 4 {
		t.Errorf("ConventionWarnings() after Open = %q, want 4 warnings", got)
	}
}
