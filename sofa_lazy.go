package sofa

import (
	"fmt"
	"io/fs"
	"slices"

	hdf5 "github.com/cwbudde/go-hdf5"
)

// OpenLazy opens a SOFA file like Open but leaves the audio data in the
// file: ImpulseResponses, TFReal/TFImag, TFRealE/TFImagE and
// SOSCoefficients stay empty, while the metadata, positions, sampling rate,
// delay and extras are read as by Open. The layout of the audio datasets is
// checked all the same, so a file OpenLazy accepts is one Open accepts.
//
// The returned File holds the file open until Close. Read measurements with
// ReadMeasurement or RangeMeasurements (FIR), ReadMeasurementTF,
// ReadMeasurementTFE or ReadMeasurementSOS; IRAt, IRPeakdB and Save need the
// audio in memory, so they fail on a lazy File.
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

// lazyAudio is an audio dataset OpenLazy leaves in the file, with the
// layout resolveLayout found for it.
type lazyAudio struct {
	ds     *hdf5.Dataset
	layout []string
}

// keepLazy records audio dataset name for later measurement reads. It
// rejects a datatype Dataset.Read cannot convert, so that OpenLazy fails
// where Open would.
func (f *File) keepLazy(name string, ds *hdf5.Dataset, layout []string) error {
	if !datasetIsNumeric(ds) {
		return fmt.Errorf("read %s: datatype is not a 4- or 8-byte float or integer", name)
	}
	if f.audio == nil {
		f.audio = map[string]lazyAudio{}
	}
	f.audio[name] = lazyAudio{ds: ds, layout: layout}
	return nil
}

// ReadMeasurement returns the impulse responses of measurement m as [R][N].
// A File from OpenLazy reads them from the file; a File from Open (or built
// in memory) returns a copy of ImpulseResponses[m]. It fails with
// ErrUnsupportedDataType for non-FIR files, ErrIndexOutOfRange when m is
// outside [0,M) or no complete measurement is stored, and fs.ErrClosed on a
// lazy File after Close. It must not be called concurrently with Close.
//
// A lazy read decompresses every HDF5 chunk the measurement touches. The
// SOFA Toolbox writes its audio data in chunks spanning all measurements,
// so on such files each call costs about as much as reading the whole
// dataset (10–30 ms for 1000–2000 measurements); use Open when most
// measurements are needed. The same holds for ReadMeasurementTF,
// ReadMeasurementTFE and ReadMeasurementSOS.
func (f *File) ReadMeasurement(m int) ([][]float64, error) {
	const what = "ReadMeasurement"
	if err := f.checkMeasurement(what, DataTypeFIR, m); err != nil {
		return nil, err
	}
	return f.measurement2D(what, "Data.IR", f.ImpulseResponses, m)
}

// ReadMeasurementTF returns the real and imaginary parts of measurement m
// of a TF file as [R][N], read from the file on a File from OpenLazy and
// copied from TFReal and TFImag otherwise. Errors and cost are those of
// ReadMeasurement; a non-TF file fails with ErrUnsupportedDataType.
func (f *File) ReadMeasurementTF(m int) (re, im [][]float64, err error) {
	const what = "ReadMeasurementTF"
	if err := f.checkMeasurement(what, DataTypeTF, m); err != nil {
		return nil, nil, err
	}
	if re, err = f.measurement2D(what, "Data.Real", f.TFReal, m); err != nil {
		return nil, nil, err
	}
	if im, err = f.measurement2D(what, "Data.Imag", f.TFImag, m); err != nil {
		return nil, nil, err
	}
	return re, im, nil
}

// ReadMeasurementTFE returns the real and imaginary parts of measurement m
// of a TF-E file as [R][E][N], like TFRealE and TFImagE, whichever axis
// order the file stores. It reads from the file on a File from OpenLazy and
// copies TFRealE and TFImagE otherwise. Errors and cost are those of
// ReadMeasurement; a non-TF-E file fails with ErrUnsupportedDataType.
func (f *File) ReadMeasurementTFE(m int) (re, im [][][]float64, err error) {
	const what = "ReadMeasurementTFE"
	if err := f.checkMeasurement(what, DataTypeTFE, m); err != nil {
		return nil, nil, err
	}
	if re, err = f.measurement3D(what, "Data.Real", f.TFRealE, m); err != nil {
		return nil, nil, err
	}
	if im, err = f.measurement3D(what, "Data.Imag", f.TFImagE, m); err != nil {
		return nil, nil, err
	}
	return re, im, nil
}

// ReadMeasurementSOS returns the second-order-section coefficients of
// measurement m of an SOS file as [R][N], read from the file on a File from
// OpenLazy and copied from SOSCoefficients otherwise. Errors and cost are
// those of ReadMeasurement; a non-SOS file fails with
// ErrUnsupportedDataType.
func (f *File) ReadMeasurementSOS(m int) ([][]float64, error) {
	const what = "ReadMeasurementSOS"
	if err := f.checkMeasurement(what, DataTypeSOS, m); err != nil {
		return nil, err
	}
	return f.measurement2D(what, "Data.SOS", f.SOSCoefficients, m)
}

// checkMeasurement checks the DataType a measurement reader needs and the
// measurement index.
func (f *File) checkMeasurement(what, dataType string, m int) error {
	if err := f.requireDataType(what, dataType); err != nil {
		return err
	}
	if m < 0 || m >= f.M {
		return fmt.Errorf("%s(%d) with M=%d: %w", what, m, f.M, ErrIndexOutOfRange)
	}
	return nil
}

// measurement2D returns measurement m of an [M][R][N] audio variable:
// read from dataset name on a lazy File, copied from eager otherwise.
func (f *File) measurement2D(what, name string, eager [][][]float64, m int) ([][]float64, error) {
	if f.lazy {
		flat, err := f.readLazy(what, name, m)
		if err != nil {
			return nil, err
		}
		return reshapeIR(flat, 1, f.R, f.N)[0], nil
	}
	if m >= len(eager) {
		return nil, fmt.Errorf("%s(%d): no measurement stored: %w", what, m, ErrIndexOutOfRange)
	}
	return copyRows(what, m, eager[m], f.R, f.N)
}

// measurement3D is measurement2D for the [M][R][E][N] TF-E variables.
func (f *File) measurement3D(what, name string, eager [][][][]float64, m int) ([][][]float64, error) {
	if f.lazy {
		flat, err := f.readLazy(what, name, m)
		if err != nil {
			return nil, err
		}
		return reshape4D(flat, 1, f.R, f.E, f.N)[0], nil
	}
	if m >= len(eager) || len(eager[m]) != f.R {
		return nil, fmt.Errorf("%s(%d): no measurement stored: %w", what, m, ErrIndexOutOfRange)
	}
	out := make([][][]float64, f.R)
	for r, rows := range eager[m] {
		var err error
		if out[r], err = copyRows(what, m, rows, f.E, f.N); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// copyRows copies rows, which must hold n rows of length size each.
func copyRows(what string, m int, rows [][]float64, n, size int) ([][]float64, error) {
	if len(rows) != n {
		return nil, fmt.Errorf("%s(%d): no measurement stored: %w", what, m, ErrIndexOutOfRange)
	}
	out := make([][]float64, n)
	for i, row := range rows {
		if len(row) != size {
			return nil, fmt.Errorf("%s(%d): row %d has %d values, want N=%d: %w",
				what, m, i, len(row), size, ErrIndexOutOfRange)
		}
		out[i] = append([]float64(nil), row...)
	}
	return out, nil
}

// readLazy reads measurement m of audio dataset name as one hyperslab
// [m,0,…]+[1,…] and returns it flat in go-sofa's axis order ([R][N], or
// [R][E][N] for TF-E, transposing a file stored [M,R,N,E]). The selection
// spans whole trailing axes, which go-hdf5 reads as one linear block (the
// row-by-row path has the E5 defect, see PLAN.md).
func (f *File) readLazy(what, name string, m int) ([]float64, error) {
	if f.audio == nil {
		return nil, fmt.Errorf("%s(%d): %w", what, m, fs.ErrClosed)
	}
	v, ok := f.audio[name]
	if !ok {
		return nil, fmt.Errorf("%s(%d): %s was not opened", what, m, name)
	}
	start := make([]uint64, len(v.layout))
	count := make([]uint64, len(v.layout))
	start[0], count[0] = uint64(m), 1 //nolint:gosec // m is non-negative, checked by checkMeasurement
	want := 1
	for i, d := range v.layout[1:] {
		count[i+1] = f.axisSize(d)
		want *= int(count[i+1]) //nolint:gosec // dimension sizes are bounded by readDimensions
	}
	raw, err := v.ds.ReadSlice(start, count)
	if err != nil {
		return nil, fmt.Errorf("%s(%d): read %s: %w", what, m, name, err)
	}
	flat, ok := raw.([]float64)
	if !ok {
		return nil, fmt.Errorf("%s(%d): read %s: got %T, want []float64", what, m, name, raw)
	}
	if len(flat) != want {
		return nil, fmt.Errorf("%s(%d): read %d values of %s, want %d", what, m, len(flat), name, want)
	}
	if slices.Equal(v.layout, layoutMRNE) {
		flat = swapLastAxes(flat, f.R, f.N, f.E)
	}
	return flat, nil
}

// RangeMeasurements calls fn for each measurement in order, with the
// impulse responses ReadMeasurement returns for it. It stops at the first
// error: an error from fn is returned as is, a read error names the
// measurement. See ReadMeasurement for the cost of lazy reads.
func (f *File) RangeMeasurements(fn func(m int, ir [][]float64) error) error {
	for m := range f.M {
		ir, err := f.ReadMeasurement(m)
		if err != nil {
			return fmt.Errorf("RangeMeasurements: measurement %d: %w", m, err)
		}
		if err := fn(m, ir); err != nil {
			return err
		}
	}
	return nil
}
