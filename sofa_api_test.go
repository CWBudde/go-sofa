package sofa

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// openFDs counts the file descriptors this process holds.
func openFDs(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/dev/fd")
	if err != nil {
		t.Skipf("cannot list /dev/fd: %v", err)
	}
	return len(entries)
}

// TestOpenReleasesHandle checks that Open reads everything eagerly and
// closes the underlying file before it returns, so a File that is never
// closed holds no descriptor.
func TestOpenReleasesHandle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no /dev/fd on Windows")
	}
	path := filepath.Join(t.TempDir(), "handle.sofa")
	if err := minimalFIRFile().Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	before := openFDs(t)
	f, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if after := openFDs(t); after != before {
		t.Errorf("open descriptors: %d before Open, %d after", before, after)
	}
	if err := f.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// requireValidationError fails unless err is a *ValidationError and
// returns it.
func requireValidationError(t *testing.T, err error) *ValidationError {
	t.Helper()
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error %v (%T) is not a *ValidationError", err, err)
	}
	return ve
}

// TestValidationErrorField checks that Save's validation errors are
// *ValidationError values naming the File field at fault, one case per
// family of checks.
func TestValidationErrorField(t *testing.T) {
	for _, tc := range []struct {
		name  string
		set   func(f *File)
		field string
	}{
		{"required attribute", func(f *File) { f.Version = "" }, "Version"},
		{"dimension", func(f *File) { f.M = 0 }, "M"},
		{"audio shape", func(f *File) { f.ImpulseResponses = f.ImpulseResponses[:0] }, "ImpulseResponses"},
		{"position rows", func(f *File) { f.SourcePositions = make([]Vector3, f.M+1) }, "SourcePositions"},
		{"coordinate type", func(f *File) { f.SourcePositionType = "polar" }, "SourcePositionType"},
		{"value", func(f *File) { f.SamplingRate = []float64{-1} }, "SamplingRate"},
		{"global attribute", func(f *File) { f.Attributes = []Attribute{{"Title", "x"}} }, "Attributes"},
		{"variable attribute", func(f *File) { f.VariableAttributes = map[string][]Attribute{"X": nil} }, "VariableAttributes"},
		{"variable", func(f *File) { f.Variables = []Variable{{Name: "X"}} }, "Variables"},
		{"convention layout", func(f *File) { f.SOFAConventions = "SimpleFreeFieldHRTF" }, "DataType"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := minimalFIRFile()
			tc.set(f)
			err := f.Save(filepath.Join(t.TempDir(), "invalid.sofa"))
			ve := requireValidationError(t, err)
			if ve.Field != tc.field {
				t.Errorf("Field = %q, want %q (error %v)", ve.Field, tc.field, err)
			}
			if !strings.HasPrefix(ve.Error(), tc.field) {
				t.Errorf("Error() = %q, want it to start with %q", ve.Error(), tc.field)
			}
		})
	}
}

// TestValidationErrorUnwraps checks that an unknown DataType is both a
// ValidationError and ErrUnsupportedDataType.
func TestValidationErrorUnwraps(t *testing.T) {
	f := minimalFIRFile()
	f.DataType = "FIR-E"
	err := f.Save(filepath.Join(t.TempDir(), "invalid.sofa"))
	if ve := requireValidationError(t, err); ve.Field != "DataType" {
		t.Errorf("Field = %q, want DataType", ve.Field)
	}
	if !errors.Is(err, ErrUnsupportedDataType) {
		t.Errorf("errors.Is(%v, ErrUnsupportedDataType) = false", err)
	}
}
