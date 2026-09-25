// Package sofa provides reading and writing support for SOFA files
// (Spatially Oriented Format for Acoustics, AES69).
//
// SOFA is a file format for storing spatially oriented acoustic data
// like head-related transfer functions (HRTFs), binaural room impulse
// responses (BRIRs), and directional room impulse responses (DRIRs).
// The format is based on HDF5 and follows the netCDF-4 conventions.
//
// See https://www.sofaconventions.org/ for specifications and documentation.
package sofa

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"

	hdf5 "github.com/cwbudde/go-hdf5"
)

// SOFA file format constants.
const (
	// conventionSOFA is the required value of the Conventions attribute
	// for any AES69 SOFA file.
	conventionSOFA = "SOFA"

	// DataType values defined by the AES69 specification.
	dataTypeFIR = "FIR"  // time-domain impulse responses
	dataTypeTF  = "TF"   // complex frequency-domain transfer functions
	dataTypeTFE = "TF-E" // TF with active emitter dimension ([M][R][E][N]); also carries SH-encoded HRTFs with E as SH coefficient index
	dataTypeSOS = "SOS"  // second-order section (biquad) filter coefficients

	// datasetSourcePosition is the SOFA dataset name for source-position data.
	datasetSourcePosition = "SourcePosition"

	// Coordinate systems a position dataset's Type attribute may name.
	CoordinateCartesian = "cartesian"
	CoordinateSpherical = "spherical"

	// UnitsSphericalDegrees is the conventional Units value for spherical
	// positions measured in degrees.
	UnitsSphericalDegrees = "degree, degree, metre"

	// UnitsCartesianMetres is the conventional Units value for cartesian
	// positions.
	UnitsCartesianMetres = "metre, metre, metre"
)

// Vector3 represents a 3D coordinate (X, Y, Z) in meters.
// Used for positions and orientations in SOFA files.
type Vector3 struct {
	X, Y, Z float64
}

// File represents an open SOFA file with all its data and metadata.
// It provides access to spatial audio data including impulse responses,
// positions, and AES69 standardized attributes.
type File struct {
	// Dimensions (M=measurements, R=receivers, E=emitters, N=samples)
	M int // number of measurements
	R int // number of receivers (e.g., 2 for binaural)
	E int // number of emitters (typically 1; for SH-encoded HRTFs this is the SH coefficient index, with E = (Lmax+1)² — see (*File).SHOrder)
	N int // number of samples per impulse response

	// Spatial data
	ListenerPositions []Vector3 // [M] listener positions for each measurement
	ListenerUp        Vector3   // listener's up vector
	ListenerView      Vector3   // listener's view direction
	ReceiverPositions []Vector3 // [R] receiver positions (e.g., left/right ear)
	SourcePositions   []Vector3 // [M] source positions for each measurement
	EmitterPositions  []Vector3 // [E] emitter positions

	// Coordinate system of each position dataset, from its Type and Units
	// attributes. Type is "cartesian" or "spherical"; for spherical data the
	// components are (azimuth, elevation, radius) and Units names their units,
	// conventionally "degree, degree, metre". Both are stored lowercased and
	// trimmed, and are empty when the file omits the attribute — absence is
	// distinguishable from a value, because a reader that must know the
	// coordinate system should say so rather than guess.
	ListenerPositionType  string
	ListenerPositionUnits string
	ReceiverPositionType  string
	ReceiverPositionUnits string
	SourcePositionType    string
	SourcePositionUnits   string
	EmitterPositionType   string
	EmitterPositionUnits  string

	// Audio data — FIR (used when DataType == "FIR")
	ImpulseResponses [][][]float64 // [M][R][N] the actual IR data
	SamplingRate     []float64     // [M] sampling rate in Hz (may be scalar)
	Delay            []float64     // [M] delay in samples

	// Audio data — TF (used when DataType == "TF")
	// Frequencies has length N. TFReal and TFImag have shape [M][R][N] and
	// together encode the complex transfer function per measurement/receiver.
	Frequencies []float64     // [N] frequency vector, Hz
	TFReal      [][][]float64 // [M][R][N] real part of complex TF
	TFImag      [][][]float64 // [M][R][N] imaginary part of complex TF

	// Audio data — TF-E (used when DataType == "TF-E")
	// Same Frequencies vector as TF, but with an active emitter dimension.
	TFRealE [][][][]float64 // [M][R][E][N] real part of complex TF
	TFImagE [][][][]float64 // [M][R][E][N] imaginary part of complex TF

	// Audio data — SOS (used when DataType == "SOS")
	// Second-order-section filter coefficients. Storage shape is [M][R][N]
	// where N is 6 × (number of biquad sections); each biquad contributes
	// six coefficients (b0, b1, b2, a0, a1, a2). SamplingRate and Delay
	// (above) carry their FIR-style meaning.
	SOSCoefficients [][][]float64 // [M][R][N]

	// AES69 Metadata (global attributes)
	Conventions            string  // "SOFA" for SOFA files
	Version                string  // SOFA version (e.g., "1.0")
	SOFAConventions        string  // specific convention (e.g., "SimpleFreeFieldHRIR", "SimpleFreeFieldHRSH" for SH-encoded HRTFs)
	SOFAConventionsVersion string  // convention version
	DataType               string  // data type (e.g., "FIR")
	RoomType               string  // room type if applicable
	RoomVolume             float64 // room volume in cubic metres; 0 when absent
	RoomTemperature        float64 // room temperature in kelvin; 0 when absent
	Title                  string  // descriptive title
	DateCreated            string  // ISO 8601 date
	DateModified           string  // ISO 8601 date
	APIName                string  // API used to create the file
	APIVersion             string  // API version
	AuthorContact          string  // author contact information
	Organization           string  // organization
	License                string  // license information
	ApplicationName        string  // application name
	ApplicationVersion     string  // application version
	Comment                string  // additional comments
	History                string  // processing history
	References             string  // references
	Origin                 string  // origin of the data

	// Internal
	hdf5File *hdf5.File // underlying HDF5 file handle
}

// Open opens a SOFA file for reading.
// It validates that the file is a valid SOFA file and reads all data and metadata.
// The caller must call Close() when done with the file.
func Open(path string) (*File, error) {
	h, err := hdf5.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open HDF5: %w", err)
	}

	f := &File{hdf5File: h}
	root := h.Root()

	// Read global attributes from root group.
	if err := f.readGlobalAttributes(root); err != nil {
		h.Close()
		return nil, fmt.Errorf("read attributes: %w", err)
	}

	// Validate SOFA convention.
	if f.Conventions != conventionSOFA {
		h.Close()
		return nil, fmt.Errorf("not a SOFA file: Conventions=%q", f.Conventions)
	}

	// Build dataset index for quick lookup.
	datasets := make(map[string]*hdf5.Dataset)
	for _, child := range root.Children() {
		if ds, ok := child.(*hdf5.Dataset); ok {
			datasets[ds.Name()] = ds
		}
	}

	// Read dimensions from dimension-scale datasets.
	if err := f.readDimensions(datasets); err != nil {
		h.Close()
		return nil, fmt.Errorf("read dimensions: %w", err)
	}

	// Read audio data.
	if err := f.readAudioData(datasets); err != nil {
		h.Close()
		return nil, fmt.Errorf("read audio data: %w", err)
	}

	// Read spatial data (best effort; missing datasets are skipped).
	f.readSpatialData(datasets)
	f.readRoomScalars(datasets)

	return f, nil
}

// Close closes the SOFA file and releases associated resources.
func (f *File) Close() error {
	if f.hdf5File != nil {
		return f.hdf5File.Close()
	}
	return nil
}

// readGlobalAttributes reads AES69 global attributes from the root group.
func (f *File) readGlobalAttributes(root *hdf5.Group) error {
	attrs, err := root.Attributes()
	if err != nil {
		return err
	}

	for _, attr := range attrs {
		val, err := attr.ReadValue()
		if err != nil {
			continue
		}
		s := fmt.Sprintf("%v", val)

		switch attr.Name {
		case "Conventions":
			f.Conventions = s
		case "Version":
			f.Version = s
		case "SOFAConventions":
			f.SOFAConventions = s
		case "SOFAConventionsVersion":
			f.SOFAConventionsVersion = s
		case "DataType":
			f.DataType = s
		case "RoomType":
			f.RoomType = s
		case datasetRoomVolume:
			f.RoomVolume = parseRoomAttribute(s)
		case datasetRoomTemperature:
			f.RoomTemperature = parseRoomAttribute(s)
		case "Title":
			f.Title = s
		case "DateCreated":
			f.DateCreated = s
		case "DateModified":
			f.DateModified = s
		case "APIName":
			f.APIName = s
		case "APIVersion":
			f.APIVersion = s
		case "AuthorContact":
			f.AuthorContact = s
		case "Organization":
			f.Organization = s
		case "License":
			f.License = s
		case "ApplicationName":
			f.ApplicationName = s
		case "ApplicationVersion":
			f.ApplicationVersion = s
		case "Comment":
			f.Comment = s
		case "History":
			f.History = s
		case "References":
			f.References = s
		case "Origin":
			f.Origin = s
		}
	}
	return nil
}

// maxDataElements caps the number of elements (product of dimensions) a
// SOFA data array may declare. It bounds the size arithmetic so that
// crafted dimension values cannot overflow int or request absurd
// allocations; 1<<30 float64 values is 8 GiB, well beyond any real
// HRTF/BRIR data set.
const maxDataElements = 1 << 30

// readDimensions extracts M, R, E, N from dimension-scale datasets.
// These datasets have a NAME attribute containing the dimension size.
// Every dimension is validated (1 ≤ size, product ≤ maxDataElements)
// before any audio data is read or allocated.
func (f *File) readDimensions(datasets map[string]*hdf5.Dataset) error {
	// Fixed order so error messages are deterministic.
	dims := []struct {
		name string
		dst  *int
	}{
		{"M", &f.M},
		{"R", &f.R},
		{"E", &f.E},
		{"N", &f.N},
	}

	for _, d := range dims {
		n, err := readDimension(datasets, d.name)
		if err != nil {
			return err
		}
		*d.dst = n
	}

	if _, err := dimProduct(f.M, f.R, f.E, f.N); err != nil {
		return fmt.Errorf("dimensions M=%d R=%d E=%d N=%d: %w", f.M, f.R, f.E, f.N, err)
	}
	return nil
}

// readDimension resolves and validates the size of one dimension.
func readDimension(datasets map[string]*hdf5.Dataset, name string) (int, error) {
	ds, ok := datasets[name]
	if !ok {
		return 0, fmt.Errorf("dimension dataset %q not found", name)
	}

	// Try to read the size from the netCDF-4 NAME attribute. The
	// classic "dimension but not variable" form is
	//   "This is a netCDF dimension but not a netCDF variable.   <size>"
	// When the dataset is itself a coordinate variable (NAME holds
	// just the dimension label, e.g. "N"), parsing fails and the
	// real size lives in the dataset's dataspace shape.
	hasCoordNAME := false
	if val, err := ds.ReadAttribute("NAME"); err == nil {
		if s, ok := val.(string); ok {
			n, perr := parseDimensionSize(s)
			switch {
			case perr != nil:
				hasCoordNAME = true
			case n < 0:
				return 0, fmt.Errorf("dimension %q: negative size %d in NAME attribute", name, n)
			case n > 0:
				return checkDimension(name, n)
			}
			// n == 0: netCDF records 0 for an unlimited dimension that
			// was empty when defined; fall back to the dataset below.
		}
	}

	// Bound the read: a crafted dataspace must not make Read allocate
	// the whole dataset before checkDimension sees its length.
	if count, ok := datasetElementCount(ds); ok && count > maxDataElements {
		return 0, fmt.Errorf("dimension %q: size %d out of range [1, %d]", name, count, maxDataElements)
	}
	data, err := ds.Read()
	if err != nil {
		return 0, fmt.Errorf("dimension %q: read dataset: %w", name, err)
	}
	switch {
	case len(data) == 0:
		return 0, fmt.Errorf("dimension %q: empty dataset", name)
	case len(data) > 1, hasCoordNAME:
		// Either /N as a TF frequency vector or any coordinate variable:
		// the dataspace length is the size; values are unrelated.
		return checkDimension(name, len(data))
	case name == "N":
		// Scalar /N from go-sofa-written FIR files: the value is the
		// sample count.
		n, err := floatDimension(name, data[0])
		if err != nil {
			return 0, err
		}
		return checkDimension(name, n)
	default:
		// /M, /R, /E from go-sofa-written files: scalar carrying the
		// count. Zero is rejected like any other out-of-range size.
		n, err := floatDimension(name, data[0])
		if err != nil {
			return 0, err
		}
		return checkDimension(name, n)
	}
}

// floatDimension converts a dimension size stored as a float64 value to
// int, rejecting NaN, ±Inf, non-integral and out-of-range values.
func floatDimension(name string, v float64) (int, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("dimension %q: non-finite size %v", name, v)
	}
	if v != math.Trunc(v) {
		return 0, fmt.Errorf("dimension %q: non-integer size %v", name, v)
	}
	if v < 1 || v > maxDataElements {
		return 0, fmt.Errorf("dimension %q: size %v out of range [1, %d]", name, v, maxDataElements)
	}
	return int(v), nil
}

// checkDimension validates an integer dimension size.
func checkDimension(name string, n int) (int, error) {
	if n < 1 || n > maxDataElements {
		return 0, fmt.Errorf("dimension %q: size %d out of range [1, %d]", name, n, maxDataElements)
	}
	return n, nil
}

// dimProduct multiplies positive dimension sizes, failing if the product
// exceeds maxDataElements (which also rules out int overflow, since each
// partial product is checked before the next multiplication).
func dimProduct(dims ...int) (int, error) {
	p := 1
	for _, d := range dims {
		if d < 1 {
			return 0, fmt.Errorf("dimension size %d must be positive", d)
		}
		if p > maxDataElements/d {
			return 0, fmt.Errorf("element count exceeds limit %d", maxDataElements)
		}
		p *= d
	}
	return p, nil
}

// parseDimensionSize extracts the size from a netCDF dimension-scale NAME string.
// The format is: "This is a netCDF dimension but not a netCDF variable.     <size>"
func parseDimensionSize(s string) (int, error) {
	s = strings.TrimSpace(s)
	// The dimension size is the last whitespace-separated token.
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return 0, fmt.Errorf("empty dimension NAME")
	}
	return strconv.Atoi(parts[len(parts)-1])
}

// readAudioData dispatches based on DataType. For TF, reads /Data.Real,
// /Data.Imag, and the frequency vector from /N. For FIR (default), reads
// /Data.IR, /Data.SamplingRate, and /Data.Delay.
func (f *File) readAudioData(datasets map[string]*hdf5.Dataset) error {
	switch f.DataType {
	case dataTypeTF:
		return f.readTFAudioData(datasets)
	case dataTypeTFE:
		return f.readTFEAudioData(datasets)
	case dataTypeSOS:
		return f.readSOSAudioData(datasets)
	default:
		return f.readFIRAudioData(datasets)
	}
}

func (f *File) readFIRAudioData(datasets map[string]*hdf5.Dataset) error {
	// Data.IR — [M][R][N] float64
	irDS, ok := datasets["Data.IR"]
	if !ok {
		return fmt.Errorf("Data.IR dataset not found")
	}
	irFlat, err := irDS.Read()
	if err != nil {
		return fmt.Errorf("read Data.IR: %w", err)
	}
	expected := f.M * f.R * f.N
	if len(irFlat) != expected {
		return fmt.Errorf("Data.IR size %d, want %d (M=%d R=%d N=%d)",
			len(irFlat), expected, f.M, f.R, f.N)
	}
	f.ImpulseResponses = reshapeIR(irFlat, f.M, f.R, f.N)

	if ds, ok := datasets["Data.SamplingRate"]; ok {
		f.SamplingRate, err = ds.Read()
		if err != nil {
			return fmt.Errorf("read Data.SamplingRate: %w", err)
		}
	}

	if ds, ok := datasets["Data.Delay"]; ok {
		f.Delay, err = ds.Read()
		if err != nil {
			return fmt.Errorf("read Data.Delay: %w", err)
		}
	}

	return nil
}

func (f *File) readTFAudioData(datasets map[string]*hdf5.Dataset) error {
	if err := f.readFrequencyVector(datasets); err != nil {
		return err
	}

	expected := f.M * f.R * f.N

	realDS, ok := datasets["Data.Real"]
	if !ok {
		return fmt.Errorf("Data.Real dataset not found")
	}
	realFlat, err := realDS.Read()
	if err != nil {
		return fmt.Errorf("read Data.Real: %w", err)
	}
	if len(realFlat) != expected {
		return fmt.Errorf("Data.Real size %d, want %d (M=%d R=%d N=%d)",
			len(realFlat), expected, f.M, f.R, f.N)
	}
	f.TFReal = reshapeIR(realFlat, f.M, f.R, f.N)

	imagDS, ok := datasets["Data.Imag"]
	if !ok {
		return fmt.Errorf("Data.Imag dataset not found")
	}
	imagFlat, err := imagDS.Read()
	if err != nil {
		return fmt.Errorf("read Data.Imag: %w", err)
	}
	if len(imagFlat) != expected {
		return fmt.Errorf("Data.Imag size %d, want %d (M=%d R=%d N=%d)",
			len(imagFlat), expected, f.M, f.R, f.N)
	}
	f.TFImag = reshapeIR(imagFlat, f.M, f.R, f.N)

	return nil
}

// readTFEAudioData reads /Data.Real and /Data.Imag as 4D arrays of
// shape [M][R][E][N], plus the frequency vector from /N. Used for
// DataType == "TF-E".
func (f *File) readTFEAudioData(datasets map[string]*hdf5.Dataset) error {
	if err := f.readFrequencyVector(datasets); err != nil {
		return err
	}

	expected := f.M * f.R * f.E * f.N

	realDS, ok := datasets["Data.Real"]
	if !ok {
		return fmt.Errorf("Data.Real dataset not found")
	}
	realFlat, err := realDS.Read()
	if err != nil {
		return fmt.Errorf("read Data.Real: %w", err)
	}
	if len(realFlat) != expected {
		return fmt.Errorf("Data.Real size %d, want %d (M=%d R=%d E=%d N=%d)",
			len(realFlat), expected, f.M, f.R, f.E, f.N)
	}
	f.TFRealE = reshape4D(realFlat, f.M, f.R, f.E, f.N)

	imagDS, ok := datasets["Data.Imag"]
	if !ok {
		return fmt.Errorf("Data.Imag dataset not found")
	}
	imagFlat, err := imagDS.Read()
	if err != nil {
		return fmt.Errorf("read Data.Imag: %w", err)
	}
	if len(imagFlat) != expected {
		return fmt.Errorf("Data.Imag size %d, want %d (M=%d R=%d E=%d N=%d)",
			len(imagFlat), expected, f.M, f.R, f.E, f.N)
	}
	f.TFImagE = reshape4D(imagFlat, f.M, f.R, f.E, f.N)

	return nil
}

// readSOSAudioData reads /Data.SOS as [M][R][N] biquad coefficients,
// plus SamplingRate and Delay (FIR-style). Used for DataType == "SOS".
func (f *File) readSOSAudioData(datasets map[string]*hdf5.Dataset) error {
	sosDS, ok := datasets["Data.SOS"]
	if !ok {
		return fmt.Errorf("Data.SOS dataset not found")
	}
	flat, err := sosDS.Read()
	if err != nil {
		return fmt.Errorf("read Data.SOS: %w", err)
	}
	expected := f.M * f.R * f.N
	if len(flat) != expected {
		return fmt.Errorf("Data.SOS size %d, want %d (M=%d R=%d N=%d)",
			len(flat), expected, f.M, f.R, f.N)
	}
	if f.N%6 != 0 {
		return fmt.Errorf("DataType=SOS expects N divisible by 6, got %d", f.N)
	}
	f.SOSCoefficients = reshapeIR(flat, f.M, f.R, f.N)

	if ds, ok := datasets["Data.SamplingRate"]; ok {
		f.SamplingRate, err = ds.Read()
		if err != nil {
			return fmt.Errorf("read Data.SamplingRate: %w", err)
		}
	}
	if ds, ok := datasets["Data.Delay"]; ok {
		f.Delay, err = ds.Read()
		if err != nil {
			return fmt.Errorf("read Data.Delay: %w", err)
		}
	}
	return nil
}

// readFrequencyVector reads /N for TF / TF-E DataTypes. Shared between
// readTFAudioData and readTFEAudioData.
func (f *File) readFrequencyVector(datasets map[string]*hdf5.Dataset) error {
	ds, ok := datasets["N"]
	if !ok {
		return nil
	}
	freqs, err := ds.Read()
	if err != nil {
		return fmt.Errorf("read /N (frequencies): %w", err)
	}
	switch {
	case len(freqs) == f.N:
		f.Frequencies = freqs
	case len(freqs) == 1:
		return fmt.Errorf("/N is scalar but DataType=%s expects frequency vector of length %d",
			f.DataType, f.N)
	default:
		return fmt.Errorf("/N length %d does not match N=%d", len(freqs), f.N)
	}
	return nil
}

// readSpatialData reads listener, receiver, source, and emitter positions.
// Position reads are best-effort: some datasets may not be readable due to
// go-hdf5 limitations with certain storage formats.
func (f *File) readSpatialData(datasets map[string]*hdf5.Dataset) {
	// Position datasets — [N×3] float64 arrays, each carrying Type and Units
	// attributes that name its coordinate system.
	type posTarget struct {
		name  string
		dst   *[]Vector3
		typ   *string
		units *string
	}
	for _, pt := range []posTarget{
		{"ListenerPosition", &f.ListenerPositions, &f.ListenerPositionType, &f.ListenerPositionUnits},
		{"ReceiverPosition", &f.ReceiverPositions, &f.ReceiverPositionType, &f.ReceiverPositionUnits},
		{datasetSourcePosition, &f.SourcePositions, &f.SourcePositionType, &f.SourcePositionUnits},
		{"EmitterPosition", &f.EmitterPositions, &f.EmitterPositionType, &f.EmitterPositionUnits},
	} {
		ds, ok := datasets[pt.name]
		if !ok {
			continue
		}
		if vecs, err := readVector3s(ds); err == nil {
			*pt.dst = vecs
		}
		*pt.typ = readStringAttribute(ds, "Type")
		*pt.units = readStringAttribute(ds, "Units")
	}

	// Orientation datasets — single Vector3 each.
	type orientTarget struct {
		name string
		dst  *Vector3
	}
	for _, ot := range []orientTarget{
		{"ListenerUp", &f.ListenerUp},
		{"ListenerView", &f.ListenerView},
	} {
		if ds, ok := datasets[ot.name]; ok {
			if vecs, err := readVector3s(ds); err == nil && len(vecs) > 0 {
				*ot.dst = vecs[0]
			}
		}
	}
}

// readStringAttribute returns a dataset attribute as a lowercased, trimmed
// string, or "" when the attribute is absent or unreadable.
func readStringAttribute(ds *hdf5.Dataset, name string) string {
	val, err := ds.ReadAttribute(name)
	if err != nil || val == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", val)))
}

// readVector3s reads a dataset of float64 triples as Vector3 values.
func readVector3s(ds *hdf5.Dataset) ([]Vector3, error) {
	data, err := ds.Read()
	if err != nil {
		return nil, err
	}
	if len(data)%3 != 0 {
		return nil, fmt.Errorf("data length %d not divisible by 3", len(data))
	}
	n := len(data) / 3
	vecs := make([]Vector3, n)
	for i := range n {
		vecs[i] = Vector3{data[i*3], data[i*3+1], data[i*3+2]}
	}
	return vecs, nil
}

// reshapeIR reshapes a flat float64 slice into [M][R][N].
func reshapeIR(flat []float64, m, r, n int) [][][]float64 {
	result := make([][][]float64, m)
	for i := range m {
		result[i] = make([][]float64, r)
		for j := range r {
			start := (i*r + j) * n
			result[i][j] = flat[start : start+n : start+n]
		}
	}
	return result
}

// reshape4D converts a flat row-major buffer of length m*r*e*n into a
// nested [m][r][e][n]float64 view. Used for TF-E audio data.
func reshape4D(flat []float64, m, r, e, n int) [][][][]float64 {
	result := make([][][][]float64, m)
	for i := range m {
		result[i] = make([][][]float64, r)
		for j := range r {
			result[i][j] = make([][]float64, e)
			for k := range e {
				start := ((i*r+j)*e + k) * n
				result[i][j][k] = flat[start : start+n : start+n]
			}
		}
	}
	return result
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

// Duration returns the duration of the impulse responses in seconds.
func (f *File) Duration() float64 {
	sr := f.SamplingRateScalar()
	if sr == 0 || f.N == 0 {
		return 0
	}
	return float64(f.N) / sr
}

// IRAt returns the impulse response for measurement m, receiver r.
// Returns nil if indices are out of range or the file holds no impulse
// responses (DataType other than "FIR").
func (f *File) IRAt(m, r int) []float64 {
	if f.DataType != dataTypeFIR {
		return nil
	}
	if m < 0 || m >= f.M || r < 0 || r >= f.R {
		return nil
	}
	if m >= len(f.ImpulseResponses) || r >= len(f.ImpulseResponses[m]) {
		return nil
	}
	return f.ImpulseResponses[m][r]
}

// IRPeakdB returns the peak level in dB (relative to 1.0) for measurement m, receiver r.
// Returns -Inf when IRAt(m, r) is nil (out of range or non-FIR file) or silent.
func (f *File) IRPeakdB(m, r int) float64 {
	ir := f.IRAt(m, r)
	if ir == nil {
		return math.Inf(-1)
	}
	peak := 0.0
	for _, v := range ir {
		if abs := math.Abs(v); abs > peak {
			peak = abs
		}
	}
	if peak == 0 {
		return math.Inf(-1)
	}
	return 20 * math.Log10(peak)
}

// Save writes the SOFA file to the specified path.
// It validates the File struct before writing and creates a fully compliant
// SOFA file with netCDF-4/HDF5 dimension scales.
//
// Save is atomic: the file is written to a temporary file in the same
// directory, flushed and fsynced, and then renamed over path. A failed
// Save leaves any existing file at path untouched and removes the
// temporary file. If path already exists its permission bits are kept;
// otherwise the new file gets mode 0644. Output is deterministic: saving
// the same File twice produces byte-identical files.
//
// All required SOFA attributes and datasets are written, along with optional
// fields if present in the File struct.
//
// Returns an error if:
//   - Validation fails (missing required fields, invalid dimensions, etc.)
//   - HDF5 file creation fails
//   - Any write, flush, close, sync or rename operation fails
func (f *File) Save(path string) (err error) {
	// Validate the File struct before writing
	if err := f.validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	mode := os.FileMode(0o644)
	if fi, statErr := os.Stat(path); statErr == nil {
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("save %s: not a regular file", path)
		}
		mode = fi.Mode().Perm()
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tmpName := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("create temporary file: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()

	if err := f.writeHDF5(tmpName); err != nil {
		return err
	}
	if err := syncFile(tmpName); err != nil {
		return fmt.Errorf("sync %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("chmod %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmpName, path, err)
	}
	// Persist the directory entry. Platforms and filesystems that cannot
	// fsync a directory are tolerated; real I/O errors are returned.
	if err := syncFile(dir); err != nil && !syncUnsupported(err) {
		return fmt.Errorf("sync directory %s: %w", dir, err)
	}
	return nil
}

// saveTestHook, when non-nil, is called by writeHDF5 after all datasets
// have been written and before the writer is closed. Tests use it to
// inject a failure late in Save.
var saveTestHook func() error

// syncFile opens name and fsyncs it.
func syncFile(name string) error {
	fd, err := os.Open(name) //nolint:gosec // path chosen by Save
	if err != nil {
		return err
	}
	return errors.Join(fd.Sync(), fd.Close())
}

// writeHDF5 writes the SOFA structure to path (truncating it). The
// writer's Close error is returned, joined with any earlier error.
func (f *File) writeHDF5(path string) (err error) {
	rootAttrs := f.collectRootAttributes()

	// Global attributes go into the root object header at creation;
	// go-hdf5 emits them in option order, so output stays deterministic.
	opts := make([]interface{}, 0, len(rootAttrs)+1)
	for _, a := range rootAttrs {
		opts = append(opts, hdf5.WithRootAttribute(a.name, a.value))
	}
	opts = append(opts, hdf5.WithRootAttribute("_NCProperties", ncProperties()))

	fw, err := hdf5.CreateForWrite(path, hdf5.CreateTruncate, opts...)
	if err != nil {
		return fmt.Errorf("create HDF5 file: %w", err)
	}
	defer func() {
		if cerr := fw.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close HDF5 file: %w", cerr))
		}
	}()

	// Dimension scales first; every variable below is attached to them.
	nc, err := f.writeDimensionScales(fw)
	if err != nil {
		return err
	}

	// Write spatial position datasets
	for _, p := range []struct {
		name       string
		positions  []Vector3
		dim        string
		size       int
		typ, units string
	}{
		{"ListenerPosition", f.ListenerPositions, dimM, f.M, f.ListenerPositionType, f.ListenerPositionUnits},
		{"ReceiverPosition", f.ReceiverPositions, dimR, f.R, f.ReceiverPositionType, f.ReceiverPositionUnits},
		{"SourcePosition", f.SourcePositions, dimM, f.M, f.SourcePositionType, f.SourcePositionUnits},
		{"EmitterPosition", f.EmitterPositions, dimE, f.E, f.EmitterPositionType, f.EmitterPositionUnits},
	} {
		if err := nc.writePositionDataset("/"+p.name, p.positions,
			rowDim(len(p.positions), p.dim, p.size), p.typ, p.units); err != nil {
			return fmt.Errorf("write %s: %w", p.name, err)
		}
	}

	// Write listener orientation vectors
	if err := nc.writeVariable("/ListenerUp", flattenVector3s([]Vector3{f.ListenerUp}), dimI, dimC); err != nil {
		return fmt.Errorf("write ListenerUp: %w", err)
	}
	if err := nc.writeVariable("/ListenerView", flattenVector3s([]Vector3{f.ListenerView}), dimI, dimC); err != nil {
		return fmt.Errorf("write ListenerView: %w", err)
	}
	if err := f.writeRoomScalars(nc); err != nil {
		return err
	}

	// Write audio data
	if err := f.writeAudioDatasets(nc); err != nil {
		return fmt.Errorf("write audio data: %w", err)
	}

	if saveTestHook != nil {
		if err := saveTestHook(); err != nil {
			return err
		}
	}
	return nil
}

// rootAttribute is one global (root-group) string attribute.
type rootAttribute struct {
	name, value string
}

// collectRootAttributes returns the global attributes to write, in a
// fixed order. Required AES69 attributes are always emitted; optional
// ones are skipped when empty.
func (f *File) collectRootAttributes() []rootAttribute {
	attrs := []rootAttribute{
		{"Conventions", f.Conventions},
		{"Version", f.Version},
		{"SOFAConventions", f.SOFAConventions},
		{"SOFAConventionsVersion", f.SOFAConventionsVersion},
		{"DataType", f.DataType},
	}
	for _, opt := range []rootAttribute{
		{"Title", f.Title},
		{"DateCreated", f.DateCreated},
		{"DateModified", f.DateModified},
		{"APIName", f.APIName},
		{"APIVersion", f.APIVersion},
		{"AuthorContact", f.AuthorContact},
		{"Organization", f.Organization},
		{"License", f.License},
		{"ApplicationName", f.ApplicationName},
		{"ApplicationVersion", f.ApplicationVersion},
		{"Comment", f.Comment},
		{"History", f.History},
		{"References", f.References},
		{"Origin", f.Origin},
		{"RoomType", f.RoomType},
	} {
		if opt.value != "" {
			attrs = append(attrs, opt)
		}
	}
	return attrs
}

// validate checks that the File struct contains all required fields
// and that dimensions are consistent.
func (f *File) validate() error {
	// Check required string attributes
	if f.Conventions != conventionSOFA {
		return fmt.Errorf("conventions must be %q, got %q", conventionSOFA, f.Conventions)
	}
	if f.Version == "" {
		return fmt.Errorf("version is required")
	}
	if f.SOFAConventions == "" {
		return fmt.Errorf("sofaConventions is required")
	}
	if f.DataType == "" {
		return fmt.Errorf("dataType is required")
	}

	// Check dimensions are non-zero
	if f.M <= 0 {
		return fmt.Errorf("m must be > 0, got %d", f.M)
	}
	if f.R <= 0 {
		return fmt.Errorf("r must be > 0, got %d", f.R)
	}
	if f.E <= 0 {
		return fmt.Errorf("e must be > 0, got %d", f.E)
	}
	if f.N <= 0 {
		return fmt.Errorf("n must be > 0, got %d", f.N)
	}

	switch f.DataType {
	case dataTypeFIR:
		if err := f.validateFIR(); err != nil {
			return err
		}
	case dataTypeTF:
		if err := f.validateTF(); err != nil {
			return err
		}
	case dataTypeTFE:
		if err := f.validateTFE(); err != nil {
			return err
		}
	case dataTypeSOS:
		if err := f.validateSOS(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported DataType %q (want %q, %q, %q, or %q)",
			f.DataType, dataTypeFIR, dataTypeTF, dataTypeTFE, dataTypeSOS)
	}

	// Check position array dimensions
	// SOFA spec allows positions to be [M×C] or [1×C] (scalar), same for other dimensions
	if len(f.ListenerPositions) != f.M && len(f.ListenerPositions) != 1 && len(f.ListenerPositions) != 0 {
		return fmt.Errorf("ListenerPositions length %d must be M=%d, 1 (scalar), or 0",
			len(f.ListenerPositions), f.M)
	}
	if len(f.ReceiverPositions) != f.R && len(f.ReceiverPositions) != 1 && len(f.ReceiverPositions) != 0 {
		return fmt.Errorf("ReceiverPositions length %d must be R=%d, 1 (scalar), or 0",
			len(f.ReceiverPositions), f.R)
	}
	if len(f.SourcePositions) != f.M && len(f.SourcePositions) != 1 && len(f.SourcePositions) != 0 {
		return fmt.Errorf("SourcePositions length %d must be M=%d, 1 (scalar), or 0",
			len(f.SourcePositions), f.M)
	}
	if len(f.EmitterPositions) != f.E && len(f.EmitterPositions) != 1 && len(f.EmitterPositions) != 0 {
		return fmt.Errorf("EmitterPositions length %d must be E=%d, 1 (scalar), or 0",
			len(f.EmitterPositions), f.E)
	}

	return f.validateConvention()
}

// validateFIR checks FIR-specific fields: ImpulseResponses [M][R][N],
// SamplingRate (M or 1), Delay (0/1/M/R/M*R).
func (f *File) validateFIR() error {
	if len(f.ImpulseResponses) != f.M {
		return fmt.Errorf("ImpulseResponses length %d does not match M=%d",
			len(f.ImpulseResponses), f.M)
	}
	for i, mr := range f.ImpulseResponses {
		if len(mr) != f.R {
			return fmt.Errorf("ImpulseResponses[%d] length %d does not match R=%d",
				i, len(mr), f.R)
		}
		for j, n := range mr {
			if len(n) != f.N {
				return fmt.Errorf("ImpulseResponses[%d][%d] length %d does not match N=%d",
					i, j, len(n), f.N)
			}
		}
	}

	if len(f.SamplingRate) != f.M && len(f.SamplingRate) != 1 {
		return fmt.Errorf("samplingRate length %d must be M=%d or 1",
			len(f.SamplingRate), f.M)
	}

	delayLen := len(f.Delay)
	if delayLen != 0 && delayLen != 1 && delayLen != f.M && delayLen != f.R && delayLen != f.M*f.R {
		return fmt.Errorf("delay length %d must be 0 (optional), 1 (scalar), M=%d, R=%d, or M×R=%d",
			delayLen, f.M, f.R, f.M*f.R)
	}
	return nil
}

// validateTF checks TF-specific fields: Frequencies length N, TFReal/TFImag
// shape [M][R][N].
func (f *File) validateTF() error {
	if len(f.Frequencies) != f.N {
		return fmt.Errorf("frequencies length %d does not match N=%d",
			len(f.Frequencies), f.N)
	}
	if err := check3D("TFReal", f.TFReal, f.M, f.R, f.N); err != nil {
		return err
	}
	if err := check3D("TFImag", f.TFImag, f.M, f.R, f.N); err != nil {
		return err
	}
	return nil
}

// validateTFE checks TF-E-specific fields: Frequencies length N,
// TFRealE/TFImagE shape [M][R][E][N].
func (f *File) validateTFE() error {
	if len(f.Frequencies) != f.N {
		return fmt.Errorf("frequencies length %d does not match N=%d",
			len(f.Frequencies), f.N)
	}
	if err := check4D("TFRealE", f.TFRealE, f.M, f.R, f.E, f.N); err != nil {
		return err
	}
	if err := check4D("TFImagE", f.TFImagE, f.M, f.R, f.E, f.N); err != nil {
		return err
	}
	return nil
}

// validateSOS checks SOS-specific fields: SOSCoefficients shape
// [M][R][N] with N divisible by 6, SamplingRate (M or 1), Delay
// (0/1/M/R/M*R) — same conventions as FIR.
func (f *File) validateSOS() error {
	if f.N%6 != 0 {
		return fmt.Errorf("DataType=SOS requires N divisible by 6, got %d", f.N)
	}
	if err := check3D("SOSCoefficients", f.SOSCoefficients, f.M, f.R, f.N); err != nil {
		return err
	}
	if len(f.SamplingRate) != f.M && len(f.SamplingRate) != 1 {
		return fmt.Errorf("samplingRate length %d must be M=%d or 1",
			len(f.SamplingRate), f.M)
	}
	delayLen := len(f.Delay)
	if delayLen != 0 && delayLen != 1 && delayLen != f.M && delayLen != f.R && delayLen != f.M*f.R {
		return fmt.Errorf("delay length %d must be 0 (optional), 1 (scalar), M=%d, R=%d, or M×R=%d",
			delayLen, f.M, f.R, f.M*f.R)
	}
	return nil
}

func check4D(name string, data [][][][]float64, m, r, e, n int) error {
	if len(data) != m {
		return fmt.Errorf("%s length %d does not match M=%d", name, len(data), m)
	}
	for i, mr := range data {
		if len(mr) != r {
			return fmt.Errorf("%s[%d] length %d does not match R=%d", name, i, len(mr), r)
		}
		for j, re := range mr {
			if len(re) != e {
				return fmt.Errorf("%s[%d][%d] length %d does not match E=%d",
					name, i, j, len(re), e)
			}
			for k, nn := range re {
				if len(nn) != n {
					return fmt.Errorf("%s[%d][%d][%d] length %d does not match N=%d",
						name, i, j, k, len(nn), n)
				}
			}
		}
	}
	return nil
}

func check3D(name string, data [][][]float64, m, r, n int) error {
	if len(data) != m {
		return fmt.Errorf("%s length %d does not match M=%d", name, len(data), m)
	}
	for i, mr := range data {
		if len(mr) != r {
			return fmt.Errorf("%s[%d] length %d does not match R=%d", name, i, len(mr), r)
		}
		for j, nn := range mr {
			if len(nn) != n {
				return fmt.Errorf("%s[%d][%d] length %d does not match N=%d",
					name, i, j, len(nn), n)
			}
		}
	}
	return nil
}

// writeAudioDatasets dispatches to the per-DataType writer.
func (f *File) writeAudioDatasets(nc *netcdfDimensions) error {
	switch f.DataType {
	case dataTypeFIR:
		return f.writeFIRAudioDatasets(nc)
	case dataTypeTF:
		return f.writeTFAudioDatasets(nc)
	case dataTypeTFE:
		return f.writeTFEAudioDatasets(nc)
	case dataTypeSOS:
		return f.writeSOSAudioDatasets(nc)
	default:
		return fmt.Errorf("unsupported DataType %q", f.DataType)
	}
}

// writeFIRAudioDatasets writes Data.IR [M,R,N], Data.SamplingRate, and
// Data.Delay.
func (f *File) writeFIRAudioDatasets(nc *netcdfDimensions) error {
	if err := nc.writeVariable("/Data.IR", flattenIR(f.ImpulseResponses), dimM, dimR, dimN); err != nil {
		return err
	}
	return f.writeSamplingRateAndDelay(nc)
}

// writeSamplingRateAndDelay writes Data.SamplingRate ([M] or [I]) and, if
// present, Data.Delay, shared by FIR and SOS.
func (f *File) writeSamplingRateAndDelay(nc *netcdfDimensions) error {
	if err := nc.writeVariable("/Data.SamplingRate", f.SamplingRate,
		rowDim(len(f.SamplingRate), dimM, f.M)); err != nil {
		return err
	}
	if len(f.Delay) > 0 {
		if err := nc.writeVariable("/Data.Delay", f.Delay, delayDims(len(f.Delay), f.M, f.R)...); err != nil {
			return err
		}
	}
	return nil
}

// delayDims names the dimensions of a Data.Delay with n values (validated
// to be 1, M, R or M×R). An M×R delay is written two-dimensional so each
// axis has a named dimension; it is checked before M and R so that it keeps
// its [M,R] shape when M or R is 1.
func delayDims(n, m, r int) []string {
	switch n {
	case 1:
		return []string{dimI}
	case m * r:
		return []string{dimM, dimR}
	case m:
		return []string{dimM}
	default:
		return []string{dimR}
	}
}

// writeTFAudioDatasets writes Data.Real and Data.Imag [M,R,N] for TF data.
// The frequency vector is written as the /N coordinate variable (see
// writeDimensionScales).
func (f *File) writeTFAudioDatasets(nc *netcdfDimensions) error {
	if err := nc.writeVariable("/Data.Real", flattenIR(f.TFReal), dimM, dimR, dimN); err != nil {
		return err
	}
	return nc.writeVariable("/Data.Imag", flattenIR(f.TFImag), dimM, dimR, dimN)
}

// writeTFEAudioDatasets writes Data.Real / Data.Imag as 4D arrays of
// shape [M][R][E][N] for DataType == "TF-E". The frequency vector is
// the /N coordinate variable as for plain TF.
func (f *File) writeTFEAudioDatasets(nc *netcdfDimensions) error {
	if err := nc.writeVariable("/Data.Real", flatten4D(f.TFRealE), dimM, dimR, dimE, dimN); err != nil {
		return err
	}
	return nc.writeVariable("/Data.Imag", flatten4D(f.TFImagE), dimM, dimR, dimE, dimN)
}

// writeSOSAudioDatasets writes Data.SOS as [M][R][N] biquad
// coefficients along with Data.SamplingRate and (optional) Data.Delay.
func (f *File) writeSOSAudioDatasets(nc *netcdfDimensions) error {
	if err := nc.writeVariable("/Data.SOS", flattenIR(f.SOSCoefficients), dimM, dimR, dimN); err != nil {
		return err
	}
	return f.writeSamplingRateAndDelay(nc)
}

// flattenIR converts [M][R][N]float64 to []float64 in row-major order.
func flattenIR(ir [][][]float64) []float64 {
	if len(ir) == 0 {
		return nil
	}
	m := len(ir)
	r := len(ir[0])
	n := len(ir[0][0])

	flat := make([]float64, m*r*n)
	idx := 0
	for i := range m {
		for j := range r {
			copy(flat[idx:idx+n], ir[i][j])
			idx += n
		}
	}
	return flat
}

// flatten4D converts [M][R][E][N]float64 to []float64 in row-major
// order. Used for TF-E audio data.
func flatten4D(data [][][][]float64) []float64 {
	if len(data) == 0 {
		return nil
	}
	m := len(data)
	r := len(data[0])
	e := len(data[0][0])
	n := len(data[0][0][0])

	flat := make([]float64, m*r*e*n)
	idx := 0
	for i := range m {
		for j := range r {
			for k := range e {
				copy(flat[idx:idx+n], data[i][j][k])
				idx += n
			}
		}
	}
	return flat
}

// flattenVector3s converts []Vector3 to []float64 (X,Y,Z,X,Y,Z,...).
func flattenVector3s(vecs []Vector3) []float64 {
	flat := make([]float64, len(vecs)*3)
	for i, v := range vecs {
		flat[i*3] = v.X
		flat[i*3+1] = v.Y
		flat[i*3+2] = v.Z
	}
	return flat
}

// netCDF-4 dimension names used by SOFA. C (coordinate triplets) and I
// (singleton) have fixed sizes.
const (
	dimM = "M"
	dimR = "R"
	dimE = "E"
	dimN = "N"
	dimC = "C"
	dimI = "I"
)

// netcdfDimensionNAME formats the netCDF-4 NAME attribute used on
// dimension-scale datasets that are *not* coordinate variables: a fixed
// text followed by the dimension size in a 10-character field, exactly as
// netCDF-C writes it (and as our reader parses it).
func netcdfDimensionNAME(size int) string {
	return fmt.Sprintf("This is a netCDF dimension but not a netCDF variable.%10d", size)
}

// ncProperties returns the _NCProperties root attribute that marks a file
// as netCDF-4 and records the library that wrote it.
func ncProperties() string {
	sofaVersion, hdf5Version := "unknown", "unknown"
	if bi, ok := debug.ReadBuildInfo(); ok {
		if bi.Main.Path == modulePath {
			sofaVersion = bi.Main.Version
		}
		for _, dep := range bi.Deps {
			switch dep.Path {
			case modulePath:
				sofaVersion = dep.Version
			case hdf5ModulePath:
				hdf5Version = dep.Version
			}
		}
	}
	return fmt.Sprintf("version=2,go-sofa=%s,go-hdf5=%s", sofaVersion, hdf5Version)
}

const (
	modulePath     = "github.com/cwbudde/go-sofa"
	hdf5ModulePath = "github.com/cwbudde/go-hdf5"
)

// netcdfDimensions holds the dimension scales of a file being written, so
// each variable can be created with its shape taken from, and attached to,
// named netCDF-4 dimensions.
type netcdfDimensions struct {
	fw     *hdf5.FileWriter
	sizes  map[string]int
	scales map[string]*hdf5.DatasetWriter
}

// writeDimensionScales writes one dimension-scale dataset per SOFA
// dimension, in a fixed order (which is also their _Netcdf4Dimid order) so
// output is deterministic. For TF and TF-E, /N is the frequency coordinate
// variable; otherwise every scale is a netCDF "dimension without variable"
// whose length is the dimension size.
func (f *File) writeDimensionScales(fw *hdf5.FileWriter) (*netcdfDimensions, error) {
	nc := &netcdfDimensions{
		fw:     fw,
		sizes:  map[string]int{dimM: f.M, dimR: f.R, dimE: f.E, dimN: f.N, dimC: 3, dimI: 1},
		scales: map[string]*hdf5.DatasetWriter{},
	}
	for id, name := range []string{dimM, dimR, dimE, dimN, dimC, dimI} {
		var ds *hdf5.DatasetWriter
		var err error
		if name == dimN && (f.DataType == dataTypeTF || f.DataType == dataTypeTFE) {
			ds, err = writeFrequencyDimension(fw, f.Frequencies, id)
		} else {
			ds, err = writeDimensionScale(fw, "/"+name, nc.sizes[name], id)
		}
		if err != nil {
			return nil, fmt.Errorf("write dimension /%s: %w", name, err)
		}
		nc.scales[name] = ds
	}
	return nc, nil
}

// writeDimensionScale writes a netCDF-4 dimension that has no variable of
// its own: a dataset of the dimension's length with CLASS=DIMENSION_SCALE,
// the netCDF NAME carrying the size, and _Netcdf4Dimid. Like netCDF-C, it
// holds no values.
func writeDimensionScale(fw *hdf5.FileWriter, name string, size, id int) (*hdf5.DatasetWriter, error) {
	ds, err := fw.CreateDataset(name, hdf5.Float32,
		[]uint64{uint64(size)}, //nolint:gosec // size > 0 by validate()
		hdf5.WithAttribute("CLASS", "DIMENSION_SCALE"),
		hdf5.WithAttribute("NAME", netcdfDimensionNAME(size)),
		hdf5.WithAttribute("_Netcdf4Dimid", int32(id))) //nolint:gosec // id < 6
	if err != nil {
		return nil, fmt.Errorf("create dimension dataset: %w", err)
	}
	return ds, nil
}

// writeFrequencyDimension writes /N as a vector of frequency values (Hz).
// The dataset is the netCDF coordinate variable of dimension N:
// CLASS=DIMENSION_SCALE and NAME equal to the dimension label, matching
// what upstream tools emit for /N in TF files.
func writeFrequencyDimension(fw *hdf5.FileWriter, freqs []float64, id int) (*hdf5.DatasetWriter, error) {
	if len(freqs) == 0 {
		return nil, fmt.Errorf("frequencies must be non-empty for TF data")
	}
	ds, err := fw.CreateDataset("/N", hdf5.Float64,
		[]uint64{uint64(len(freqs))},
		hdf5.WithAttribute("CLASS", "DIMENSION_SCALE"),
		hdf5.WithAttribute("NAME", dimN),
		hdf5.WithAttribute("_Netcdf4Dimid", int32(id))) //nolint:gosec // id < 6
	if err != nil {
		return nil, fmt.Errorf("create /N dataset: %w", err)
	}
	if err := ds.Write(freqs); err != nil {
		return nil, fmt.Errorf("write /N values: %w", err)
	}
	return ds, nil
}

// writeVariable creates a float64 variable whose shape is given by the
// named dimensions, writes data and attaches the dimension scales, so
// netCDF-4 readers see named (not phony) dimensions.
func (nc *netcdfDimensions) writeVariable(name string, data []float64, dims ...string) error {
	return nc.writeVariableWithAttrs(name, data, dims, nil)
}

func (nc *netcdfDimensions) writeVariableWithAttrs(name string, data []float64, dims []string,
	attrs []hdf5.DatasetOption,
) error {
	shape := make([]uint64, len(dims))
	for i, d := range dims {
		size, ok := nc.sizes[d]
		if !ok {
			return fmt.Errorf("%s: unknown dimension %q", name, d)
		}
		shape[i] = uint64(size) //nolint:gosec // sizes > 0 by validate()
	}

	ds, err := nc.fw.CreateDataset(name, hdf5.Float64, shape, attrs...)
	if err != nil {
		return fmt.Errorf("create %s dataset: %w", name, err)
	}
	if err := ds.Write(data); err != nil {
		return fmt.Errorf("write %s data: %w", name, err)
	}
	for i, d := range dims {
		if err := nc.fw.AttachDimensionScale(ds, nc.scales[d], i); err != nil {
			return fmt.Errorf("attach dimension %s to %s: %w", d, name, err)
		}
	}
	return nil
}

// rowDim names the first dimension of a variable with n rows that is
// either per-element of dimension dim (n == size) or constant (n == 1, I).
func rowDim(n int, dim string, size int) string {
	if n == size {
		return dim
	}
	return dimI
}

// writePositionDataset writes a position variable [rows, C] tagged with the
// Type and Units attributes that name its coordinate system. Empty type or
// units are omitted rather than written as empty strings.
func (nc *netcdfDimensions) writePositionDataset(name string, positions []Vector3, rows, typ, units string) error {
	if len(positions) == 0 {
		// Skip if no positions provided
		return nil
	}

	var attrs []hdf5.DatasetOption
	if typ != "" {
		attrs = append(attrs, hdf5.WithAttribute("Type", typ))
	}
	if units != "" {
		attrs = append(attrs, hdf5.WithAttribute("Units", units))
	}
	return nc.writeVariableWithAttrs(name, flattenVector3s(positions), []string{rows, dimC}, attrs)
}
