package sofa

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotSOFA reports that Open read an HDF5 file whose Conventions
// attribute is not "SOFA". Test for it with errors.Is.
var ErrNotSOFA = errors.New("not a SOFA file")

// ValidationError reports a File that Save refuses to write. Field names
// the File field at fault, such as "M", "ImpulseResponses",
// "SourcePositionType" or "Variables"; Err says what is wrong with it.
// Save returns every validation failure as a *ValidationError, which
// callers can extract with errors.As. The message starts with Field, as in
// "M: must be > 0, got 0" or "ImpulseResponses[0] length 1 does not match
// R=2".
type ValidationError struct {
	Field string
	Err   error
}

func (e *ValidationError) Error() string {
	msg := e.Err.Error()
	if strings.HasPrefix(msg, "[") { // an index into Field
		return e.Field + msg
	}
	return e.Field + ": " + msg
}

// Unwrap returns Err, so that errors.Is sees sentinels such as
// ErrUnsupportedDataType through a ValidationError.
func (e *ValidationError) Unwrap() error { return e.Err }

// invalid returns a *ValidationError for field whose message is format
// applied to args; %w wraps an error as with fmt.Errorf.
func invalid(field, format string, args ...any) *ValidationError {
	return &ValidationError{Field: field, Err: fmt.Errorf(format, args...)}
}
