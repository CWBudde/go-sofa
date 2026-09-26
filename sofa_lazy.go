package sofa

import (
	"fmt"
	"io/fs"
)

// OpenLazy opens a SOFA file like Open but leaves the audio data in the
// file: ImpulseResponses, TFReal/TFImag, TFRealE/TFImagE and
// SOSCoefficients stay empty, while the metadata, positions, sampling rate,
// delay and extras are read as by Open. The layout of the audio datasets is
// checked all the same, so a file OpenLazy accepts is one Open accepts.
//
// The returned File holds the file open until Close. Read FIR measurements
// with ReadMeasurement or RangeMeasurements; IRAt, IRPeakdB and Save need
// the audio in memory, so they fail on a lazy File.
func OpenLazy(path string) (*File, error) {
	return open(path, true)
}

// Close releases the file a File from OpenLazy holds open; further calls
// return nil. For a File from Open it does nothing and returns nil, since
// Open already closes the file it reads.
func (f *File) Close() error {
	if f.h5 == nil {
		return nil
	}
	h := f.h5
	f.h5, f.audio = nil, nil
	return h.Close()
}

// ReadMeasurement returns the impulse responses of measurement m as [R][N].
// A File from OpenLazy reads them from the file; a File from Open (or built
// in memory) returns a copy of ImpulseResponses[m]. It fails with
// ErrUnsupportedDataType for non-FIR files, ErrIndexOutOfRange when m is
// outside [0,M) or no complete measurement is stored, and fs.ErrClosed on a
// lazy File after Close. It must not be called concurrently with Close.
//
// A lazy read decompresses every HDF5 chunk the measurement touches. The
// SOFA Toolbox writes Data.IR in chunks spanning all measurements, so on
// such files each call costs about as much as reading the whole dataset
// (10–30 ms for 1000–2000 measurements); use Open when most measurements
// are needed.
func (f *File) ReadMeasurement(m int) ([][]float64, error) {
	if err := f.requireFIR("ReadMeasurement"); err != nil {
		return nil, err
	}
	if m < 0 || m >= f.M {
		return nil, fmt.Errorf("ReadMeasurement(%d) with M=%d: %w", m, f.M, ErrIndexOutOfRange)
	}
	if f.lazy {
		return f.readLazyMeasurement(m)
	}
	if m >= len(f.ImpulseResponses) || len(f.ImpulseResponses[m]) != f.R {
		return nil, fmt.Errorf("ReadMeasurement(%d): no measurement stored: %w", m, ErrIndexOutOfRange)
	}
	out := make([][]float64, f.R)
	for r, ir := range f.ImpulseResponses[m] {
		if len(ir) != f.N {
			return nil, fmt.Errorf("ReadMeasurement(%d): receiver %d has %d samples, want N=%d: %w",
				m, r, len(ir), f.N, ErrIndexOutOfRange)
		}
		out[r] = append([]float64(nil), ir...)
	}
	return out, nil
}

// readLazyMeasurement reads the hyperslab [m,0,0]+[1,R,N] of Data.IR. The
// selection spans whole trailing axes, which go-hdf5 reads as one linear
// block (the row-by-row path has the E5 defect, see PLAN.md).
func (f *File) readLazyMeasurement(m int) ([][]float64, error) {
	if f.audio == nil {
		return nil, fmt.Errorf("ReadMeasurement(%d): %w", m, fs.ErrClosed)
	}
	//nolint:gosec // m, R and N are non-negative, checked above and by readDimensions
	start, count := []uint64{uint64(m), 0, 0}, []uint64{1, uint64(f.R), uint64(f.N)}
	raw, err := f.audio.ReadSlice(start, count)
	if err != nil {
		return nil, fmt.Errorf("ReadMeasurement(%d): read Data.IR: %w", m, err)
	}
	flat, ok := raw.([]float64)
	if !ok {
		return nil, fmt.Errorf("ReadMeasurement(%d): read Data.IR: got %T, want []float64", m, raw)
	}
	if len(flat) != f.R*f.N {
		return nil, fmt.Errorf("ReadMeasurement(%d): read %d values of Data.IR, want R*N=%d", m, len(flat), f.R*f.N)
	}
	return reshapeIR(flat, 1, f.R, f.N)[0], nil
}
