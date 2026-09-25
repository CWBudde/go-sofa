package sofa

import (
	"errors"
	"fmt"
	"math"
)

// ErrUnsupportedDataType reports a DataType this package cannot read or
// write: an empty or unknown value, GeneralFIR-E's "FIR-E" and the legacy
// "FIRE". Test for it with errors.Is.
var ErrUnsupportedDataType = errors.New("unsupported DataType")

// checkDataType returns an ErrUnsupportedDataType-wrapping error unless dt
// is one of the DataTypes this package reads and writes.
func checkDataType(dt string) error {
	switch dt {
	case dataTypeFIR, dataTypeTF, dataTypeTFE, dataTypeSOS:
		return nil
	case "":
		return fmt.Errorf("%w: DataType attribute is missing or empty", ErrUnsupportedDataType)
	case "FIR-E", "FIRE":
		return fmt.Errorf("%w %q: per-emitter impulse responses (GeneralFIR-E) are not supported",
			ErrUnsupportedDataType, dt)
	default:
		return fmt.Errorf("%w %q (want %q, %q, %q, or %q)",
			ErrUnsupportedDataType, dt, dataTypeFIR, dataTypeTF, dataTypeTFE, dataTypeSOS)
	}
}

// SamplingRateScalar returns the sampling rate as a scalar value.
// If multiple sampling rates are stored, it returns the first one.
// Returns 0 if no sampling rate is available.
func (f *File) SamplingRateScalar() float64 {
	if len(f.SamplingRate) > 0 {
		return f.SamplingRate[0]
	}
	return 0
}

// ErrIndexOutOfRange reports a measurement or receiver index outside the
// file's dimensions, or one for which no data is stored. Test for it with
// errors.Is.
var ErrIndexOutOfRange = errors.New("index out of range")

// requireFIR returns an ErrUnsupportedDataType-wrapping error unless the
// file holds impulse responses.
func (f *File) requireFIR(what string) error {
	if f.DataType != dataTypeFIR {
		return fmt.Errorf("%s: %w %q (needs %q)", what, ErrUnsupportedDataType, f.DataType, dataTypeFIR)
	}
	return nil
}

// Duration returns the length of the impulse responses in seconds, N
// divided by the (first) sampling rate. It fails with
// ErrUnsupportedDataType for non-FIR files, where N counts frequency bins
// or filter coefficients, and when no positive sampling rate is stored.
func (f *File) Duration() (float64, error) {
	if err := f.requireFIR("Duration"); err != nil {
		return 0, err
	}
	sr := f.SamplingRateScalar()
	if !(sr > 0) || math.IsInf(sr, 0) {
		return 0, fmt.Errorf("Duration: invalid sampling rate %v", sr)
	}
	if f.N <= 0 {
		return 0, fmt.Errorf("Duration: invalid N=%d", f.N)
	}
	return float64(f.N) / sr, nil
}

// IRAt returns the impulse response for measurement m, receiver r. It
// fails with ErrUnsupportedDataType for non-FIR files and with
// ErrIndexOutOfRange when m or r is outside [0,M)×[0,R) or no response is
// stored there.
func (f *File) IRAt(m, r int) ([]float64, error) {
	if err := f.requireFIR("IRAt"); err != nil {
		return nil, err
	}
	if m < 0 || m >= f.M || r < 0 || r >= f.R {
		return nil, fmt.Errorf("IRAt(%d, %d) with M=%d R=%d: %w", m, r, f.M, f.R, ErrIndexOutOfRange)
	}
	if m >= len(f.ImpulseResponses) || r >= len(f.ImpulseResponses[m]) {
		return nil, fmt.Errorf("IRAt(%d, %d): no impulse response stored: %w", m, r, ErrIndexOutOfRange)
	}
	return f.ImpulseResponses[m][r], nil
}

// IRPeakdB returns the peak level in dB (relative to 1.0) for measurement
// m, receiver r; a silent response yields -Inf. Errors are those of IRAt.
func (f *File) IRPeakdB(m, r int) (float64, error) {
	ir, err := f.IRAt(m, r)
	if err != nil {
		return 0, err
	}
	peak := 0.0
	for _, v := range ir {
		if abs := math.Abs(v); abs > peak {
			peak = abs
		}
	}
	if peak == 0 {
		return math.Inf(-1), nil
	}
	return 20 * math.Log10(peak), nil
}
