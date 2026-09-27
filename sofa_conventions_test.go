package sofa

import (
	"errors"
	"path/filepath"
	"reflect"
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
