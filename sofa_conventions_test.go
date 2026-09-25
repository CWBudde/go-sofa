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
