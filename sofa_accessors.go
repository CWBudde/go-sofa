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

// ErrNoSamplingRate reports a file without Data.SamplingRate, as TF files
// have. Test for it with errors.Is.
var ErrNoSamplingRate = errors.New("no sampling rate stored")

// ErrVaryingSamplingRate reports that a file stores different sampling
// rates per measurement where a single rate was asked for. Test for it
// with errors.Is.
var ErrVaryingSamplingRate = errors.New("sampling rate varies across measurements")

// SamplingRateScalar returns the file's single sampling rate in Hz: the
// [I] value, or the [M] values when they are all equal. It fails with
// ErrNoSamplingRate when none is stored and with ErrVaryingSamplingRate
// when the per-measurement rates differ; use SamplingRateAt then.
func (f *File) SamplingRateScalar() (float64, error) {
	if len(f.SamplingRate) == 0 {
		return 0, ErrNoSamplingRate
	}
	sr := f.SamplingRate[0]
	for _, v := range f.SamplingRate[1:] {
		if v != sr {
			return 0, fmt.Errorf("%w: %v and %v", ErrVaryingSamplingRate, sr, v)
		}
	}
	return sr, nil
}

// SamplingRateAt returns the sampling rate in Hz of measurement m,
// broadcasting an [I] rate to every measurement. It fails with
// ErrIndexOutOfRange when m is outside [0,M) and with ErrNoSamplingRate
// when none is stored.
func (f *File) SamplingRateAt(m int) (float64, error) {
	if m < 0 || m >= f.M {
		return 0, fmt.Errorf("SamplingRateAt(%d) with M=%d: %w", m, f.M, ErrIndexOutOfRange)
	}
	v, err := broadcastM(len(f.SamplingRate), f.M, m)
	if err != nil {
		if len(f.SamplingRate) == 0 {
			return 0, ErrNoSamplingRate
		}
		return 0, fmt.Errorf("SamplingRateAt(%d): SamplingRate %w", m, err)
	}
	return f.SamplingRate[v], nil
}

// SourcePositionAt returns the source position of measurement m,
// broadcasting an [I,C] position to every measurement. It fails with
// ErrIndexOutOfRange when m is outside [0,M) or no position is stored.
func (f *File) SourcePositionAt(m int) (Vector3, error) {
	if m < 0 || m >= f.M {
		return Vector3{}, fmt.Errorf("SourcePositionAt(%d) with M=%d: %w", m, f.M, ErrIndexOutOfRange)
	}
	v, err := broadcastM(len(f.SourcePositions), f.M, m)
	if err != nil {
		return Vector3{}, fmt.Errorf("SourcePositionAt(%d): SourcePositions %w", m, err)
	}
	return f.SourcePositions[v], nil
}

// broadcastM maps measurement m to an index into a variable with n values
// stored as [I] (n == 1) or [M] (n == M).
func broadcastM(n, mSize, m int) (int, error) {
	switch n {
	case 1:
		return 0, nil
	case mSize:
		return m, nil
	default:
		return 0, fmt.Errorf("has %d values, want 1 or M=%d: %w", n, mSize, ErrIndexOutOfRange)
	}
}

// DelayAt returns the delay in samples of measurement m, receiver r,
// broadcasting Data.Delay stored as [I], [I,R], [R], [M] or [M,R]. A file
// without Data.Delay has no delay, so DelayAt returns 0. It fails with
// ErrIndexOutOfRange when m or r is outside [0,M)×[0,R) or the stored
// delay does not fit its layout.
func (f *File) DelayAt(m, r int) (float64, error) {
	if m < 0 || m >= f.M || r < 0 || r >= f.R {
		return 0, fmt.Errorf("DelayAt(%d, %d) with M=%d R=%d: %w", m, r, f.M, f.R, ErrIndexOutOfRange)
	}
	if len(f.Delay) == 0 {
		return 0, nil
	}
	axes := f.delayAxes()
	if f.layoutSize(axes) != len(f.Delay) {
		return 0, fmt.Errorf("DelayAt(%d, %d): %d delay values fit none of [I], [R], [M], [M,R] with M=%d R=%d: %w",
			m, r, len(f.Delay), f.M, f.R, ErrIndexOutOfRange)
	}
	i := 0
	for _, d := range axes {
		switch d {
		case dimM:
			i = i*f.M + m
		case dimR:
			i = i*f.R + r
		}
	}
	return f.Delay[i], nil
}

// delayAxes returns the layout of f.Delay: the one Open resolved, which
// tells [M] from [R] when M == R, or, for a File built in memory or a
// Delay changed since, the layout Save would write.
func (f *File) delayAxes() []string {
	if f.delayLayout != nil && f.layoutSize(f.delayLayout) == len(f.Delay) {
		return f.delayLayout
	}
	return delayDims(len(f.Delay), f.M, f.R)
}

// layoutSize returns the number of values a variable with the given
// dimensions holds.
func (f *File) layoutSize(layout []string) int {
	n := 1
	for _, d := range layout {
		n *= int(f.axisSize(d)) //nolint:gosec // dimensions validated by readDimensions
	}
	return n
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
	sr, err := f.SamplingRateScalar()
	if err != nil {
		return 0, fmt.Errorf("Duration: %w", err)
	}
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
// ErrIndexOutOfRange when m or r is outside [0,M)×[0,R) or no complete
// response (N samples) is stored there.
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
	ir := f.ImpulseResponses[m][r]
	if len(ir) != f.N {
		return nil, fmt.Errorf("IRAt(%d, %d): %d samples stored, want N=%d: %w", m, r, len(ir), f.N, ErrIndexOutOfRange)
	}
	return ir, nil
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
