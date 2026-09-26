package sofa

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"sync"

	hdf5 "github.com/cwbudde/go-hdf5"
)

// ErrNotLoaded reports an accessor such as IRAt that needs the whole audio
// array in memory, called on a File returned by OpenLazy. Read such a File
// with ReadMeasurement and its siblings, or open it with Open. Test for it
// with errors.Is.
var ErrNotLoaded = errors.New("audio data not loaded (file opened with OpenLazy)")

// OpenLazy reads a SOFA file like Open, except for the audio variables
// (Data.IR, Data.Real and Data.Imag, Data.SOS): it checks their shapes but
// leaves their values in the file, so the returned File's ImpulseResponses,
// TFReal, TFImag, TFRealE, TFImagE and SOSCoefficients are nil. Dimensions,
// positions, attributes, SamplingRate, Delay, Frequencies and the extra
// variables are loaded as by Open.
//
// Read the audio data one measurement at a time with ReadMeasurement,
// ReadMeasurementTF, ReadMeasurementTFE, ReadMeasurementSOS or the Range
// methods. Accessors that need the whole array in memory (IRAt, IRPeakdB)
// fail with ErrNotLoaded, and so does Save while the audio fields are
// empty.
//
// The File keeps the file open until Close, which the caller must call;
// after it, reading audio data fails with fs.ErrClosed. Reads on one File are
// serialised, so it may be shared between goroutines.
func OpenLazy(path string) (*File, error) {
	return open(path, true)
}

// OpenLazyReader is OpenLazy for a SOFA file of size bytes read from r,
// such as a bytes.Reader or an *os.File opened elsewhere. The File reads
// audio data from r until Close, so r must stay usable until then; Close
// does not close r.
func OpenLazyReader(r io.ReaderAt, size int64) (*File, error) {
	return openReader(r, size, true)
}

// Close releases the file handle of a File returned by OpenLazy; after it,
// the File's metadata stays usable but reading audio data fails with
// fs.ErrClosed. Closing again does nothing and returns nil. For any other
// File, Close does nothing and returns nil: Open already closes the file it
// reads, and the File holds all data in memory.
func (f *File) Close() error {
	if f.lazy == nil {
		return nil
	}
	return f.lazy.close()
}

// lazyAudio holds the audio variables of a File returned by OpenLazy.
type lazyAudio struct {
	mu   sync.Mutex
	h    *hdf5.File // nil after Close
	vars map[string]*lazyVariable
}

// lazyVariable is one audio variable left in the file: its dataset, the
// layout (dimension names) resolved for it and the matching shape.
//
// Each read is one ReadSlice of a whole measurement. go-hdf5 caches the
// dataset's parsed header and chunk index and keeps recently used chunks
// decompressed (8 chunks / 16 MiB per dataset by default), so sequential
// reads of a chunked file decompress each chunk once as long as the chunks
// one measurement spans fit in that cache.
type lazyVariable struct {
	ds     *hdf5.Dataset
	layout []string
	shape  []uint64
}

// Names of the audio variables.
const (
	varIR   = "Data.IR"
	varReal = "Data.Real"
	varImag = "Data.Imag"
	varSOS  = "Data.SOS"
)

// prepareLazyAudio resolves the layout of each audio variable of the
// file's DataType without reading it, reads the small per-measurement
// variables (SamplingRate, Delay, Frequencies) as Open does, and keeps h
// open for later reads.
func (f *File) prepareLazyAudio(h *hdf5.File, datasets map[string]*hdf5.Dataset, labels map[string][]string) error {
	var names []string
	var layouts [][]string
	switch f.DataType {
	case DataTypeFIR:
		names, layouts = []string{varIR}, [][]string{layoutMRN}
	case DataTypeTF:
		names, layouts = []string{varReal, varImag}, [][]string{layoutMRN}
	case DataTypeTFE:
		names, layouts = []string{varReal, varImag}, [][]string{layoutMREN, layoutMRNE}
	case DataTypeSOS:
		if f.N%6 != 0 {
			return fmt.Errorf("DataType=SOS expects N divisible by 6, got %d", f.N)
		}
		names, layouts = []string{varSOS}, [][]string{layoutMRN}
	default:
		return invalid("DataType", "%w", checkDataType(f.DataType))
	}

	vars := make(map[string]*lazyVariable, len(names))
	for _, name := range names {
		ds, ok := datasets[name]
		if !ok {
			return fmt.Errorf("%s dataset not found", name)
		}
		layout, err := f.resolveLayout(name, ds, labels[name], layouts...)
		if err != nil {
			return err
		}
		// Open's Dataset.Read rejects every other datatype, so OpenLazy
		// must too: a file OpenLazy accepts is one Open accepts.
		dt, err := ds.Datatype()
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if !dt.IsFloat64() && !dt.IsFloat32() && !dt.IsInt64() && !dt.IsInt32() {
			return fmt.Errorf("read %s: datatype %s is not a 4- or 8-byte float or integer", name, dt)
		}
		shape := make([]uint64, len(layout))
		for i, d := range layout {
			shape[i] = f.axisSize(d)
		}
		vars[name] = &lazyVariable{ds: ds, layout: layout, shape: shape}
	}

	switch f.DataType {
	case DataTypeFIR, DataTypeSOS:
		if err := f.readRateAndDelay(datasets, labels); err != nil {
			return err
		}
	default:
		if err := f.readFrequencyVector(datasets, labels); err != nil {
			return err
		}
	}
	f.lazy = &lazyAudio{h: h, vars: vars}
	return nil
}

// hasAudio reports whether the audio field of the file's DataType holds
// any data.
func (f *File) hasAudio() bool {
	switch f.DataType {
	case DataTypeFIR:
		return len(f.ImpulseResponses) > 0
	case DataTypeTF:
		return len(f.TFReal) > 0 || len(f.TFImag) > 0
	case DataTypeTFE:
		return len(f.TFRealE) > 0 || len(f.TFImagE) > 0
	case DataTypeSOS:
		return len(f.SOSCoefficients) > 0
	}
	return false
}

// close closes the HDF5 file once.
func (l *lazyAudio) close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.h == nil {
		return nil
	}
	err := l.h.Close()
	l.h = nil
	if err != nil {
		return fmt.Errorf("close HDF5: %w", err)
	}
	return nil
}

// readMeasurement reads measurement m of an audio variable in its stored
// layout into a new slice.
func (l *lazyAudio) readMeasurement(name string, m int) ([]float64, *lazyVariable, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	v := l.vars[name]
	if l.h == nil {
		return nil, v, fmt.Errorf("read %s: %w", name, fs.ErrClosed)
	}
	row := 1
	for _, n := range v.shape[1:] {
		row *= int(n) //nolint:gosec // bounded by dimProduct
	}
	flat, err := v.read(m, row)
	return flat, v, err
}

// read reads measurement m of the variable, row values long, as the
// hyperslab [m:m+1, 0:…, 0:…] that spans every trailing axis in full. On a
// contiguous dataset that is a single linear run, which is why whole
// measurements are read even when the caller needs only part of one.
func (v *lazyVariable) read(m, row int) ([]float64, error) {
	name := v.ds.Name()
	first := make([]uint64, len(v.shape))
	counts := slices.Clone(v.shape)
	first[0], counts[0] = uint64(m), 1 //nolint:gosec // checked against M by the caller
	raw, err := v.ds.ReadSlice(first, counts)
	if err != nil {
		return nil, fmt.Errorf("read %s measurement %d: %w", name, m, err)
	}
	flat, ok := raw.([]float64)
	if !ok || len(flat) != row {
		return nil, fmt.Errorf("read %s measurement %d: got %T of %d values, want %d float64",
			name, m, raw, len(flat), row)
	}
	return flat, nil
}

// checkMeasurement returns an ErrIndexOutOfRange-wrapping error unless m
// is in [0,M).
func (f *File) checkMeasurement(what string, m int) error {
	if m < 0 || m >= f.M {
		return fmt.Errorf("%s(%d) with M=%d: %w", what, m, f.M, ErrIndexOutOfRange)
	}
	return nil
}

// requireDataType returns an ErrUnsupportedDataType-wrapping error unless
// the file's DataType is dt.
func (f *File) requireDataType(what, dt string) error {
	if f.DataType != dt {
		return fmt.Errorf("%s: %w %q (needs %q)", what, ErrUnsupportedDataType, f.DataType, dt)
	}
	return nil
}

// readMRN returns measurement m of an [M][R][N] audio variable: from
// loaded for a File read by Open or built in memory, from the file for one
// returned by OpenLazy.
func (f *File) readMRN(what, name string, loaded [][][]float64, m int) ([][]float64, error) {
	if err := f.checkMeasurement(what, m); err != nil {
		return nil, err
	}
	if f.lazy == nil {
		if m >= len(loaded) {
			return nil, fmt.Errorf("%s(%d): no data stored: %w", what, m, ErrIndexOutOfRange)
		}
		if err := checkRN(loaded[m], f.R, f.N); err != nil {
			return nil, fmt.Errorf("%s(%d): %w", what, m, err)
		}
		return loaded[m], nil
	}
	flat, _, err := f.lazy.readMeasurement(name, m)
	if err != nil {
		return nil, fmt.Errorf("%s(%d): %w", what, m, err)
	}
	return reshapeIR(flat, 1, f.R, f.N)[0], nil
}

// checkRN reports whether rows has exactly rows×n shape, so a File built
// or edited in memory cannot hand out a ragged measurement.
func checkRN(rows [][]float64, nRows, n int) error {
	if len(rows) != nRows {
		return fmt.Errorf("%d rows stored, want %d: %w", len(rows), nRows, ErrIndexOutOfRange)
	}
	for i, row := range rows {
		if len(row) != n {
			return fmt.Errorf("row %d has %d values, want %d: %w", i, len(row), n, ErrIndexOutOfRange)
		}
	}
	return nil
}

// readMREN returns measurement m of a TF-E [M][R][E][N] audio variable,
// transposing the AES69 [M,R,N,E] storage order; see readMRN.
func (f *File) readMREN(what, name string, loaded [][][][]float64, m int) ([][][]float64, error) {
	if err := f.checkMeasurement(what, m); err != nil {
		return nil, err
	}
	if f.lazy == nil {
		if m >= len(loaded) {
			return nil, fmt.Errorf("%s(%d): no data stored: %w", what, m, ErrIndexOutOfRange)
		}
		row := loaded[m]
		if len(row) != f.R {
			return nil, fmt.Errorf("%s(%d): %d receivers stored, R=%d: %w", what, m, len(row), f.R, ErrIndexOutOfRange)
		}
		for r := range row {
			if err := checkRN(row[r], f.E, f.N); err != nil {
				return nil, fmt.Errorf("%s(%d) receiver %d: %w", what, m, r, err)
			}
		}
		return row, nil
	}
	flat, v, err := f.lazy.readMeasurement(name, m)
	if err != nil {
		return nil, fmt.Errorf("%s(%d): %w", what, m, err)
	}
	if slices.Equal(v.layout, layoutMRNE) {
		flat = swapLastAxes(flat, f.R, f.N, f.E)
	}
	return reshape4D(flat, 1, f.R, f.E, f.N)[0], nil
}

// ReadMeasurement returns the impulse responses of measurement m, shaped
// [R][N]. For a File returned by OpenLazy it reads just that measurement
// from the file into new slices; otherwise it returns
// ImpulseResponses[m], which shares memory with the File. It fails with
// ErrUnsupportedDataType for non-FIR files, with ErrIndexOutOfRange when
// m is outside [0,M) or no data is stored there, and with fs.ErrClosed after
// Close of a lazy File.
func (f *File) ReadMeasurement(m int) ([][]float64, error) {
	if err := f.requireDataType("ReadMeasurement", DataTypeFIR); err != nil {
		return nil, err
	}
	return f.readMRN("ReadMeasurement", varIR, f.ImpulseResponses, m)
}

// ReadMeasurementTF returns the real and imaginary parts of the transfer
// functions of measurement m of a TF file, each shaped [R][N]. It reads
// and fails like ReadMeasurement, with ErrUnsupportedDataType for files
// other than TF.
func (f *File) ReadMeasurementTF(m int) (re, im [][]float64, err error) {
	const what = "ReadMeasurementTF"
	if err := f.requireDataType(what, DataTypeTF); err != nil {
		return nil, nil, err
	}
	if re, err = f.readMRN(what, varReal, f.TFReal, m); err != nil {
		return nil, nil, err
	}
	if im, err = f.readMRN(what, varImag, f.TFImag, m); err != nil {
		return nil, nil, err
	}
	return re, im, nil
}

// ReadMeasurementTFE returns the real and imaginary parts of the transfer
// functions of measurement m of a TF-E file, each shaped [R][E][N] like
// TFRealE[m]. It reads and fails like ReadMeasurement, with
// ErrUnsupportedDataType for files other than TF-E.
func (f *File) ReadMeasurementTFE(m int) (re, im [][][]float64, err error) {
	const what = "ReadMeasurementTFE"
	if err := f.requireDataType(what, DataTypeTFE); err != nil {
		return nil, nil, err
	}
	if re, err = f.readMREN(what, varReal, f.TFRealE, m); err != nil {
		return nil, nil, err
	}
	if im, err = f.readMREN(what, varImag, f.TFImagE, m); err != nil {
		return nil, nil, err
	}
	return re, im, nil
}

// ReadMeasurementSOS returns the second-order-section coefficients of
// measurement m of an SOS file, shaped [R][N] like SOSCoefficients[m]. It
// reads and fails like ReadMeasurement, with ErrUnsupportedDataType for
// files other than SOS.
func (f *File) ReadMeasurementSOS(m int) ([][]float64, error) {
	if err := f.requireDataType("ReadMeasurementSOS", DataTypeSOS); err != nil {
		return nil, err
	}
	return f.readMRN("ReadMeasurementSOS", varSOS, f.SOSCoefficients, m)
}

// RangeMeasurements calls fn with the impulse responses ([R][N], as from
// ReadMeasurement) of each measurement in order. It stops at the first
// error, from reading or from fn, and returns it; an error of fn is
// returned unwrapped. For a File returned by OpenLazy only one measurement
// is held in memory at a time, unless fn keeps it.
func (f *File) RangeMeasurements(fn func(m int, ir [][]float64) error) error {
	if err := f.requireDataType("RangeMeasurements", DataTypeFIR); err != nil {
		return err
	}
	for m := range f.M {
		ir, err := f.ReadMeasurement(m)
		if err != nil {
			return err
		}
		if err := fn(m, ir); err != nil {
			return err
		}
	}
	return nil
}

// RangeMeasurementsTF calls fn with the real and imaginary parts ([R][N],
// as from ReadMeasurementTF) of each measurement of a TF file in order. It
// stops like RangeMeasurements.
func (f *File) RangeMeasurementsTF(fn func(m int, re, im [][]float64) error) error {
	if err := f.requireDataType("RangeMeasurementsTF", DataTypeTF); err != nil {
		return err
	}
	for m := range f.M {
		re, im, err := f.ReadMeasurementTF(m)
		if err != nil {
			return err
		}
		if err := fn(m, re, im); err != nil {
			return err
		}
	}
	return nil
}
