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
	"slices"
	"strconv"
	"strings"
	"time"

	hdf5 "github.com/cwbudde/go-hdf5"
)

// DataType values defined by the AES69 specification: the values of
// File.DataType this package reads and writes.
const (
	DataTypeFIR = "FIR"  // time-domain impulse responses
	DataTypeTF  = "TF"   // complex frequency-domain transfer functions
	DataTypeTFE = "TF-E" // TF with active emitter dimension ([M][R][E][N]); also carries SH-encoded HRTFs with E as SH coefficient index
	DataTypeSOS = "SOS"  // second-order section (biquad) filter coefficients
)

// SOFA file format constants.
const (
	// conventionSOFA is the required value of the Conventions attribute
	// for any AES69 SOFA file.
	conventionSOFA = "SOFA"

	// SOFA dataset names of the position and orientation variables.
	datasetListenerPosition = "ListenerPosition"
	datasetReceiverPosition = "ReceiverPosition"
	datasetSourcePosition   = "SourcePosition"
	datasetEmitterPosition  = "EmitterPosition"
	datasetListenerView     = "ListenerView"
	datasetListenerUp       = "ListenerUp"

	// Coordinate systems a position dataset's Type attribute may name.
	CoordinateCartesian = "cartesian"
	CoordinateSpherical = "spherical"
	// CoordinateSphericalHarmonics marks EmitterPosition data of SH-encoded
	// files, where each emitter is one SH coefficient (see SHOrder).
	CoordinateSphericalHarmonics = "spherical harmonics"

	// UnitsSphericalDegrees is the conventional Units value for spherical
	// positions measured in degrees.
	UnitsSphericalDegrees = "degree, degree, metre"

	// UnitsCartesianMetres is the conventional Units value for cartesian
	// positions.
	UnitsCartesianMetres = "metre, metre, metre"
)

// Vector3 is one coordinate triplet of a position or orientation
// variable. Its units are those the variable's Type and Units attributes
// name: X, Y, Z in metres for "cartesian"; azimuth, elevation (degrees, or
// radians where Units say so) and radius in metres for "spherical" and
// "spherical harmonics" (where each EmitterPosition row is one SH
// coefficient's emitter).
type Vector3 struct {
	X, Y, Z float64
}

// File holds the contents of a SOFA file: its AES69 attributes, positions
// and audio data. Open fills it completely, OpenLazy fills everything but
// the audio data (read it with ReadMeasurement and its siblings), and Save
// writes one built or modified in memory.
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

	// Measurement-dependent layouts, filled by Open only when a file stores
	// them: ReceiverPosition [R,C,M] and EmitterPosition [E,C,M] as [M][R]
	// and [M][E], ListenerView and ListenerUp [M,C] as [M]. The fields above
	// then hold measurement 0. When set, Save writes these fields in the
	// same layouts instead of the singular ones.
	ReceiverPositionsM [][]Vector3
	EmitterPositionsM  [][]Vector3
	ListenerViews      []Vector3
	ListenerUps        []Vector3

	// Coordinate system of each position dataset, from its Type and Units
	// attributes. Type is "cartesian" or "spherical"; for spherical data the
	// components are (azimuth, elevation, radius) and Units names their units,
	// conventionally "degree, degree, metre". Both are stored trimmed but in
	// the file's case (compare them with strings.EqualFold), and are empty
	// when the file omits the attribute — absence is
	// distinguishable from a value, because a reader that must know the
	// coordinate system should say so rather than guess.
	//
	// Save requires a Type ("cartesian", "spherical" or "spherical
	// harmonics") on every position it writes.
	ListenerPositionType  string
	ListenerPositionUnits string
	ReceiverPositionType  string
	ReceiverPositionUnits string
	SourcePositionType    string
	SourcePositionUnits   string
	EmitterPositionType   string
	EmitterPositionUnits  string

	// Coordinate system of ListenerView and ListenerUp, from ListenerView's
	// Type and Units attributes. Save writes "cartesian" and "metre" when
	// they are empty.
	ListenerViewType  string
	ListenerViewUnits string

	// Audio data — FIR (used when DataType == "FIR")
	ImpulseResponses [][][]float64 // [M][R][N] the actual IR data
	SamplingRate     []float64     // [M] sampling rate in Hz (may be scalar)
	Delay            []float64     // delay in samples: 1 (shared), M, R or M×R (row-major [M][R]) values; see DelayAt

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

	// Content go-sofa does not interpret, kept so that Open followed by
	// Save loses nothing. Open fills these fields; callers may edit them,
	// and Save writes them back.
	//
	// Attributes holds the global attributes that have no field above
	// (DatabaseName, ListenerShortName, …). Variables holds the variables
	// Save would not write otherwise (SourceView, RoomCornerA, char arrays
	// such as ReceiverDescriptions, …). Open sorts both by name, since HDF5
	// does not keep the order of attributes; Save writes them in slice
	// order. VariableAttributes holds, per variable name, the attributes of
	// the variables and dimensions Save does write beyond the ones it sets
	// itself (Type/Units, Data.SamplingRate:Units, N:LongName/Units for TF).
	// Dropped lists what Open could not keep (an unsupported data or
	// attribute type), so that a lossy round trip is never silent.
	Attributes         []Attribute
	Variables          []Variable
	VariableAttributes map[string][]Attribute
	Dropped            []string

	// Internal
	delayLayout []string   // Data.Delay dimensions as resolved by Open; see delayAxes
	lazy        *lazyAudio // audio variables left in the file by OpenLazy; nil for Open
}

// Open reads a SOFA file. It checks that the file is a SOFA file, reads all
// data and metadata into the returned File and closes the file again before
// it returns, so the File holds no open handle. A failure to close the file
// is returned too, joined with any read error, and yields no File. Use
// OpenLazy to leave the audio data in the file and read it one measurement
// at a time, and OpenReader to read a file held in memory.
func Open(path string) (*File, error) {
	return open(path, false)
}

// open reads the SOFA file at path; see read for lazy.
func open(path string, lazy bool) (*File, error) {
	h, err := hdf5.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open HDF5: %w", err)
	}
	return read(h, lazy)
}

// read reads the SOFA file h and closes it. When lazy is set, the audio
// variables are checked but not read, and h stays open for them unless
// read fails.
func read(h *hdf5.File, lazy bool) (f *File, err error) {
	defer func() {
		if lazy && err == nil {
			return
		}
		if cerr := h.Close(); cerr != nil {
			f, err = nil, errors.Join(err, fmt.Errorf("close HDF5: %w", cerr))
		}
	}()

	f = &File{}
	root := h.Root()

	// Read global attributes from root group.
	if err := f.readGlobalAttributes(root); err != nil {
		return nil, fmt.Errorf("read attributes: %w", err)
	}

	// Validate SOFA convention.
	if f.Conventions != conventionSOFA {
		return nil, fmt.Errorf("%w: Conventions=%q", ErrNotSOFA, f.Conventions)
	}
	if err := checkDataType(f.DataType); err != nil {
		return nil, err
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
		return nil, fmt.Errorf("read dimensions: %w", err)
	}

	// Read audio data, or only check its layout for a lazy File.
	labels := dimensionLabels(datasets, sofaDimensions)
	if lazy {
		err = f.prepareLazyAudio(h, datasets, labels)
	} else {
		err = f.readAudioData(datasets, labels)
	}
	if err != nil {
		return nil, fmt.Errorf("read audio data: %w", err)
	}

	// Read spatial data. Missing datasets are skipped; unreadable ones and
	// shapes that match no allowed layout fail.
	if err := f.readSpatialData(datasets, labels); err != nil {
		return nil, fmt.Errorf("read spatial data: %w", err)
	}
	f.readRoomScalars(datasets)

	// Keep what go-sofa does not interpret, so Save can write it back.
	f.readExtras(datasets)

	return f, nil
}

// globalAttribute is a root attribute's name and a deferred read of its
// value, so that attributes go-sofa does not interpret are never decoded.
type globalAttribute struct {
	name string
	read func() (interface{}, error)
}

// readGlobalAttributes reads AES69 global attributes from the root group.
func (f *File) readGlobalAttributes(root *hdf5.Group) error {
	attrs, err := root.Attributes()
	if err != nil {
		return err
	}
	global := make([]globalAttribute, len(attrs))
	for i, a := range attrs {
		global[i] = globalAttribute{name: a.Name, read: a.ReadValue}
	}
	return f.setGlobalAttributes(global)
}

// setGlobalAttributes stores the attributes go-sofa maps to File fields,
// and keeps the others in f.Attributes. A mapped attribute that cannot be
// read is an error rather than a silently empty field; an unmapped one is
// listed in f.Dropped. netCDF's own attributes (_NCProperties, …) are
// skipped unread, since Save writes its own.
func (f *File) setGlobalAttributes(attrs []globalAttribute) error {
	fields := f.globalFields()
	for _, a := range attrs {
		set, ok := fields[a.name]
		if !ok {
			if !netcdfAttribute(a.name) {
				f.keepGlobalAttribute(a)
			}
			continue
		}
		val, err := a.read()
		if err != nil {
			return fmt.Errorf("attribute %s: %w", a.name, err)
		}
		set(attributeString(val))
	}
	slices.SortFunc(f.Attributes, func(a, b Attribute) int { return strings.Compare(a.Name, b.Name) })
	return nil
}

// globalFields maps each global attribute go-sofa stores in a File field
// to a setter for that field.
func (f *File) globalFields() map[string]func(string) {
	setString := func(dst *string) func(string) { return func(s string) { *dst = s } }
	setRoom := func(dst *float64) func(string) { return func(s string) { *dst = parseRoomAttribute(s) } }
	return map[string]func(string){
		"Conventions":            setString(&f.Conventions),
		"Version":                setString(&f.Version),
		"SOFAConventions":        setString(&f.SOFAConventions),
		"SOFAConventionsVersion": setString(&f.SOFAConventionsVersion),
		"DataType":               setString(&f.DataType),
		"RoomType":               setString(&f.RoomType),
		datasetRoomVolume:        setRoom(&f.RoomVolume),
		datasetRoomTemperature:   setRoom(&f.RoomTemperature),
		"Title":                  setString(&f.Title),
		"DateCreated":            setString(&f.DateCreated),
		"DateModified":           setString(&f.DateModified),
		"APIName":                setString(&f.APIName),
		"APIVersion":             setString(&f.APIVersion),
		"AuthorContact":          setString(&f.AuthorContact),
		"Organization":           setString(&f.Organization),
		"License":                setString(&f.License),
		"ApplicationName":        setString(&f.ApplicationName),
		"ApplicationVersion":     setString(&f.ApplicationVersion),
		"Comment":                setString(&f.Comment),
		"History":                setString(&f.History),
		"References":             setString(&f.References),
		"Origin":                 setString(&f.Origin),
	}
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
// /Data.Imag, and the frequency vector from /N. For FIR, reads
// /Data.IR, /Data.SamplingRate, and /Data.Delay.
func (f *File) readAudioData(datasets map[string]*hdf5.Dataset, labels map[string][]string) error {
	switch f.DataType {
	case DataTypeFIR:
		return f.readFIRAudioData(datasets, labels)
	case DataTypeTF:
		return f.readTFAudioData(datasets, labels)
	case DataTypeTFE:
		return f.readTFEAudioData(datasets, labels)
	case DataTypeSOS:
		return f.readSOSAudioData(datasets, labels)
	default:
		return invalid("DataType", "%w", checkDataType(f.DataType))
	}
}

// Layouts of the audio variables, in order of preference.
var (
	layoutMRN  = []string{dimM, dimR, dimN}
	layoutMREN = []string{dimM, dimR, dimE, dimN} // TF-E order older go-sofa versions wrote
	layoutMRNE = []string{dimM, dimR, dimN, dimE} // AES69 TF-E order ("mrne"); SOFA Toolbox and Save write it
)

func (f *File) readFIRAudioData(datasets map[string]*hdf5.Dataset, labels map[string][]string) error {
	// Data.IR — [M][R][N] float64
	irDS, ok := datasets["Data.IR"]
	if !ok {
		return fmt.Errorf("Data.IR dataset not found")
	}
	if _, err := f.resolveLayout("Data.IR", irDS, labels["Data.IR"], layoutMRN); err != nil {
		return err
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

	return f.readRateAndDelay(datasets, labels)
}

// readRateAndDelay reads Data.SamplingRate ([I] or [M]) and Data.Delay
// ([I,R], [M,R], or the 1-D [I], [M], [R] or [M·R] that go-sofa wrote
// before it named its dimensions), shared by FIR and SOS.
func (f *File) readRateAndDelay(datasets map[string]*hdf5.Dataset, labels map[string][]string) error {
	var err error
	if ds, ok := datasets["Data.SamplingRate"]; ok {
		if _, err := f.resolveLayout("Data.SamplingRate", ds, labels["Data.SamplingRate"],
			[]string{dimI}, []string{dimM}); err != nil {
			return err
		}
		f.SamplingRate, err = ds.Read()
		if err != nil {
			return fmt.Errorf("read Data.SamplingRate: %w", err)
		}
	}
	if ds, ok := datasets["Data.Delay"]; ok {
		layout, err := f.resolveLayout("Data.Delay", ds, labels["Data.Delay"],
			[]string{dimI, dimR}, []string{dimM, dimR}, []string{dimI}, []string{dimM}, []string{dimR})
		if err != nil {
			shape, _ := datasetShape(ds)
			legacy := labels["Data.Delay"] == nil && len(shape) == 1 && shape[0] == uint64(f.M*f.R) //nolint:gosec // bounded by dimProduct
			if !legacy {
				return err
			}
			layout = []string{dimM, dimR}
		}
		f.delayLayout = layout
		f.Delay, err = ds.Read()
		if err != nil {
			return fmt.Errorf("read Data.Delay: %w", err)
		}
	}
	return nil
}

func (f *File) readTFAudioData(datasets map[string]*hdf5.Dataset, labels map[string][]string) error {
	if err := f.readFrequencyVector(datasets, labels); err != nil {
		return err
	}

	expected := f.M * f.R * f.N

	realDS, ok := datasets["Data.Real"]
	if !ok {
		return fmt.Errorf("Data.Real dataset not found")
	}
	if _, err := f.resolveLayout("Data.Real", realDS, labels["Data.Real"], layoutMRN); err != nil {
		return err
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
	if _, err := f.resolveLayout("Data.Imag", imagDS, labels["Data.Imag"], layoutMRN); err != nil {
		return err
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
// DataType == "TF-E". Files store the arrays [M,R,N,E] (AES69, SOFA
// Toolbox, Save), transposed on read, or [M,R,E,N] (older go-sofa).
func (f *File) readTFEAudioData(datasets map[string]*hdf5.Dataset, labels map[string][]string) error {
	if err := f.readFrequencyVector(datasets, labels); err != nil {
		return err
	}

	expected := f.M * f.R * f.E * f.N

	realDS, ok := datasets["Data.Real"]
	if !ok {
		return fmt.Errorf("Data.Real dataset not found")
	}
	realLayout, err := f.resolveLayout("Data.Real", realDS, labels["Data.Real"], layoutMREN, layoutMRNE)
	if err != nil {
		return err
	}
	realFlat, err := realDS.Read()
	if err != nil {
		return fmt.Errorf("read Data.Real: %w", err)
	}
	if len(realFlat) != expected {
		return fmt.Errorf("Data.Real size %d, want %d (M=%d R=%d E=%d N=%d)",
			len(realFlat), expected, f.M, f.R, f.E, f.N)
	}
	if slices.Equal(realLayout, layoutMRNE) {
		realFlat = swapLastAxes(realFlat, f.M*f.R, f.N, f.E)
	}
	f.TFRealE = reshape4D(realFlat, f.M, f.R, f.E, f.N)

	imagDS, ok := datasets["Data.Imag"]
	if !ok {
		return fmt.Errorf("Data.Imag dataset not found")
	}
	imagLayout, err := f.resolveLayout("Data.Imag", imagDS, labels["Data.Imag"], layoutMREN, layoutMRNE)
	if err != nil {
		return err
	}
	imagFlat, err := imagDS.Read()
	if err != nil {
		return fmt.Errorf("read Data.Imag: %w", err)
	}
	if len(imagFlat) != expected {
		return fmt.Errorf("Data.Imag size %d, want %d (M=%d R=%d E=%d N=%d)",
			len(imagFlat), expected, f.M, f.R, f.E, f.N)
	}
	if slices.Equal(imagLayout, layoutMRNE) {
		imagFlat = swapLastAxes(imagFlat, f.M*f.R, f.N, f.E)
	}
	f.TFImagE = reshape4D(imagFlat, f.M, f.R, f.E, f.N)

	return nil
}

// readSOSAudioData reads /Data.SOS as [M][R][N] biquad coefficients,
// plus SamplingRate and Delay (FIR-style). Used for DataType == "SOS".
func (f *File) readSOSAudioData(datasets map[string]*hdf5.Dataset, labels map[string][]string) error {
	sosDS, ok := datasets["Data.SOS"]
	if !ok {
		return fmt.Errorf("Data.SOS dataset not found")
	}
	if _, err := f.resolveLayout("Data.SOS", sosDS, labels["Data.SOS"], layoutMRN); err != nil {
		return err
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

	return f.readRateAndDelay(datasets, labels)
}

// readFrequencyVector reads /N for TF / TF-E DataTypes. Shared between
// readTFAudioData and readTFEAudioData.
func (f *File) readFrequencyVector(datasets map[string]*hdf5.Dataset, labels map[string][]string) error {
	ds, ok := datasets["N"]
	if !ok {
		return nil
	}
	if _, err := f.resolveLayout("N", ds, labels["N"], []string{dimN}); err != nil {
		return err
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

// writeHDF5 writes the SOFA structure to the HDF5 writer that create
// returns for the given root-attribute options. Once everything is
// written without error, commit (if non-nil) is called just before the
// writer is closed. The writer's Close error is returned, joined with any
// earlier error.
func (f *File) writeHDF5(create func(opts []interface{}) (*hdf5.FileWriter, error), commit func()) (err error) {
	rootAttrs := f.collectRootAttributes()

	// Global attributes go into the root object header at creation;
	// go-hdf5 emits them in option order, so output stays deterministic.
	opts := make([]interface{}, 0, len(rootAttrs)+1)
	for _, a := range rootAttrs {
		opts = append(opts, hdf5.WithRootAttribute(a.name, a.value))
	}
	for _, a := range f.Attributes {
		opts = append(opts, hdf5.WithRootAttribute(a.Name, a.Value))
	}
	opts = append(opts, hdf5.WithRootAttribute("_NCProperties", ncProperties()))

	fw, err := create(opts)
	if err != nil {
		return fmt.Errorf("create HDF5 file: %w", err)
	}
	defer func() {
		if err == nil && commit != nil {
			commit()
		}
		if cerr := fw.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close HDF5 file: %w", cerr))
		}
	}()

	// Dimension scales first; every variable below is attached to them.
	nc, err := f.writeDimensionScales(fw)
	if err != nil {
		return err
	}

	// Write spatial position datasets; per-measurement receiver and
	// emitter positions, when set, replace the shared ones.
	for _, p := range []struct {
		name       string
		positions  []Vector3
		perM       [][]Vector3
		dim        string
		size       int
		typ, units string
	}{
		{datasetListenerPosition, f.ListenerPositions, nil, dimM, f.M, f.ListenerPositionType, f.ListenerPositionUnits},
		{datasetReceiverPosition, f.ReceiverPositions, f.ReceiverPositionsM, dimR, f.R, f.ReceiverPositionType, f.ReceiverPositionUnits},
		{datasetSourcePosition, f.SourcePositions, nil, dimM, f.M, f.SourcePositionType, f.SourcePositionUnits},
		{datasetEmitterPosition, f.EmitterPositions, f.EmitterPositionsM, dimE, f.E, f.EmitterPositionType, f.EmitterPositionUnits},
	} {
		var err error
		if len(p.perM) > 0 {
			err = nc.writePositionDatasetPerM("/"+p.name, p.perM,
				rowDim(len(p.perM[0]), p.dim, p.size), p.typ, p.units)
		} else {
			err = nc.writePositionDataset("/"+p.name, p.positions,
				rowDim(len(p.positions), p.dim, p.size), p.typ, p.units)
		}
		if err != nil {
			return fmt.Errorf("write %s: %w", p.name, err)
		}
	}

	// Write listener orientation vectors, [M,C] when given per measurement,
	// else [I,C] with the conventions' default for an unset vector. Both
	// carry ListenerView's coordinate system.
	viewAttrs := positionAttributes(f.listenerViewCoordinates())
	view, up := f.listenerOrientation()
	for _, o := range []struct {
		name string
		one  Vector3
		all  []Vector3
	}{
		{datasetListenerUp, up, f.ListenerUps},
		{datasetListenerView, view, f.ListenerViews},
	} {
		vecs, rows := []Vector3{o.one}, dimI
		if len(o.all) > 0 {
			vecs, rows = o.all, dimM
		}
		if err := nc.writeVariableWithAttrs("/"+o.name, flattenVector3s(vecs), []string{rows, dimC}, viewAttrs); err != nil {
			return fmt.Errorf("write %s: %w", o.name, err)
		}
	}
	if err := f.writeRoomScalars(nc); err != nil {
		return err
	}

	// Write audio data
	if err := f.writeAudioDatasets(nc); err != nil {
		return fmt.Errorf("write audio data: %w", err)
	}
	if err := nc.writeExtraVariables(f.Variables); err != nil {
		return err
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

// Defaults Save writes for mandatory global attributes left empty, taken
// from the SOFA conventions' default values.
const (
	defaultAPIName  = "go-sofa"
	defaultLicense  = "No license provided, ask the author for permission"
	defaultRoomType = "free field"
	sofaDateLayout  = "2006-01-02 15:04:05" // the SOFA Toolbox's date format
)

// saveTime returns the time Save stamps into empty DateCreated and
// DateModified attributes; tests replace it.
var saveTime = time.Now

// collectRootAttributes returns the global attributes to write, in a
// fixed order. The attributes AES69 makes mandatory are always emitted,
// with defaults for empty APIName, APIVersion, dates, License and
// RoomType (AuthorContact, Organization and Title may be empty); the File
// itself is not changed. Optional attributes are skipped when empty.
func (f *File) collectRootAttributes() []rootAttribute {
	or := func(v, def string) string {
		if v == "" {
			return def
		}
		return v
	}
	now := saveTime().UTC().Format(sofaDateLayout)
	attrs := []rootAttribute{
		{"Conventions", f.Conventions},
		{"Version", f.Version},
		{"SOFAConventions", f.SOFAConventions},
		{"SOFAConventionsVersion", f.SOFAConventionsVersion},
		{"DataType", f.DataType},
		{"Title", f.Title},
		{"DateCreated", or(f.DateCreated, now)},
		{"DateModified", or(f.DateModified, now)},
		{"APIName", or(f.APIName, defaultAPIName)},
		{"APIVersion", or(f.APIVersion, moduleVersion())},
		{"AuthorContact", f.AuthorContact},
		{"Organization", f.Organization},
		{"License", or(f.License, defaultLicense)},
		{"RoomType", or(f.RoomType, defaultRoomType)},
	}
	for _, opt := range []rootAttribute{
		{"ApplicationName", f.ApplicationName},
		{"ApplicationVersion", f.ApplicationVersion},
		{"Comment", f.Comment},
		{"History", f.History},
		{"References", f.References},
		{"Origin", f.Origin},
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
	if err := f.validateRequiredAttributes(); err != nil {
		return err
	}

	// Check dimensions are non-zero
	if f.M <= 0 {
		return invalid("M", "must be > 0, got %d", f.M)
	}
	if f.R <= 0 {
		return invalid("R", "must be > 0, got %d", f.R)
	}
	if f.E <= 0 {
		return invalid("E", "must be > 0, got %d", f.E)
	}
	if f.N <= 0 {
		return invalid("N", "must be > 0, got %d", f.N)
	}

	switch f.DataType {
	case DataTypeFIR:
		if err := f.validateFIR(); err != nil {
			return err
		}
	case DataTypeTF:
		if err := f.validateTF(); err != nil {
			return err
		}
	case DataTypeTFE:
		if err := f.validateTFE(); err != nil {
			return err
		}
	case DataTypeSOS:
		if err := f.validateSOS(); err != nil {
			return err
		}
	default:
		return invalid("DataType", "%w", checkDataType(f.DataType))
	}

	// Check position array dimensions
	// SOFA spec allows positions to be [M×C] or [1×C] (scalar), same for other dimensions
	if len(f.ListenerPositions) != f.M && len(f.ListenerPositions) != 1 && len(f.ListenerPositions) != 0 {
		return invalid("ListenerPositions", "length %d must be M=%d, 1 (scalar), or 0",
			len(f.ListenerPositions), f.M)
	}
	if len(f.ReceiverPositions) != f.R && len(f.ReceiverPositions) != 1 && len(f.ReceiverPositions) != 0 {
		return invalid("ReceiverPositions", "length %d must be R=%d, 1 (scalar), or 0",
			len(f.ReceiverPositions), f.R)
	}
	if len(f.SourcePositions) != f.M && len(f.SourcePositions) != 1 && len(f.SourcePositions) != 0 {
		return invalid("SourcePositions", "length %d must be M=%d, 1 (scalar), or 0",
			len(f.SourcePositions), f.M)
	}
	if len(f.EmitterPositions) != f.E && len(f.EmitterPositions) != 1 && len(f.EmitterPositions) != 0 {
		return invalid("EmitterPositions", "length %d must be E=%d, 1 (scalar), or 0",
			len(f.EmitterPositions), f.E)
	}
	if err := f.validatePerMeasurement(); err != nil {
		return err
	}
	if err := f.validateCoordinateTypes(); err != nil {
		return err
	}
	if err := f.validateValues(); err != nil {
		return err
	}
	if err := f.validateExtras(); err != nil {
		return err
	}

	return f.validateConvention()
}

// validateCoordinateTypes checks that every position Save writes names its
// coordinate system with an AES69 Type, and that a ListenerView Type, when
// set, is one too. Types are compared case-insensitively.
func (f *File) validateCoordinateTypes() error {
	for _, p := range []struct {
		name    string
		written bool
		typ     string
	}{
		{datasetListenerPosition, len(f.ListenerPositions) > 0, f.ListenerPositionType},
		{datasetReceiverPosition, len(f.ReceiverPositions) > 0 || len(f.ReceiverPositionsM) > 0, f.ReceiverPositionType},
		{datasetSourcePosition, len(f.SourcePositions) > 0, f.SourcePositionType},
		{datasetEmitterPosition, len(f.EmitterPositions) > 0 || len(f.EmitterPositionsM) > 0, f.EmitterPositionType},
		{datasetListenerView, f.ListenerViewType != "", f.ListenerViewType},
	} {
		if !p.written {
			continue
		}
		if p.typ == "" {
			return invalid(p.name+"Type", "is required")
		}
		switch strings.ToLower(strings.TrimSpace(p.typ)) {
		case CoordinateCartesian, CoordinateSpherical, CoordinateSphericalHarmonics:
		default:
			return invalid(p.name+"Type", "%q must be %q, %q or %q", p.typ,
				CoordinateCartesian, CoordinateSpherical, CoordinateSphericalHarmonics)
		}
	}
	return nil
}

// validateRequiredAttributes checks the global attributes Save cannot
// default: Conventions, Version, SOFAConventions, SOFAConventionsVersion
// and DataType.
func (f *File) validateRequiredAttributes() error {
	if f.Conventions != conventionSOFA {
		return invalid("Conventions", "must be %q, got %q", conventionSOFA, f.Conventions)
	}
	if f.Version == "" {
		return invalid("Version", "is required")
	}
	if f.SOFAConventions == "" {
		return invalid("SOFAConventions", "is required")
	}
	if f.SOFAConventionsVersion == "" {
		return invalid("SOFAConventionsVersion", "is required")
	}
	if f.DataType == "" {
		return invalid("DataType", "is required")
	}
	return nil
}

// validatePerMeasurement checks the per-measurement layouts: M rows of R
// (E) or 1 receivers (emitters) each, and M listener orientations.
func (f *File) validatePerMeasurement() error {
	for _, p := range []struct {
		name string
		perM [][]Vector3
		dim  string
		size int
	}{
		{"ReceiverPositionsM", f.ReceiverPositionsM, dimR, f.R},
		{"EmitterPositionsM", f.EmitterPositionsM, dimE, f.E},
	} {
		if len(p.perM) == 0 {
			continue
		}
		if len(p.perM) != f.M {
			return invalid(p.name, "has %d rows, want M=%d", len(p.perM), f.M)
		}
		width := len(p.perM[0])
		if width != p.size && width != 1 {
			return invalid(p.name, "[0] has length %d, want %s=%d or 1", width, p.dim, p.size)
		}
		for i, row := range p.perM {
			if len(row) != width {
				return invalid(p.name, "[%d] has length %d, but [0] has %d", i, len(row), width)
			}
		}
	}
	if n := len(f.ListenerViews); n != 0 && n != f.M {
		return invalid("ListenerViews", "length %d must be M=%d or 0", n, f.M)
	}
	if n := len(f.ListenerUps); n != 0 && n != f.M {
		return invalid("ListenerUps", "length %d must be M=%d or 0", n, f.M)
	}
	return nil
}

// validateFIR checks FIR-specific fields: ImpulseResponses [M][R][N],
// SamplingRate (M or 1), Delay (0/1/M/R/M*R).
func (f *File) validateFIR() error {
	if len(f.ImpulseResponses) != f.M {
		return invalid("ImpulseResponses", "length %d does not match M=%d",
			len(f.ImpulseResponses), f.M)
	}
	for i, mr := range f.ImpulseResponses {
		if len(mr) != f.R {
			return invalid("ImpulseResponses", "[%d] length %d does not match R=%d",
				i, len(mr), f.R)
		}
		for j, n := range mr {
			if len(n) != f.N {
				return invalid("ImpulseResponses", "[%d][%d] length %d does not match N=%d",
					i, j, len(n), f.N)
			}
		}
	}

	if len(f.SamplingRate) != f.M && len(f.SamplingRate) != 1 {
		return invalid("SamplingRate", "length %d must be M=%d or 1",
			len(f.SamplingRate), f.M)
	}

	delayLen := len(f.Delay)
	if delayLen != 0 && delayLen != 1 && delayLen != f.M && delayLen != f.R && delayLen != f.M*f.R {
		return invalid("Delay", "length %d must be 0 (optional), 1 (scalar), M=%d, R=%d, or M×R=%d",
			delayLen, f.M, f.R, f.M*f.R)
	}
	return nil
}

// validateTF checks TF-specific fields: Frequencies length N, TFReal/TFImag
// shape [M][R][N].
func (f *File) validateTF() error {
	if len(f.Frequencies) != f.N {
		return invalid("Frequencies", "length %d does not match N=%d",
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
		return invalid("Frequencies", "length %d does not match N=%d",
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
		return invalid("N", "must be divisible by 6 for DataType SOS, got %d", f.N)
	}
	if err := check3D("SOSCoefficients", f.SOSCoefficients, f.M, f.R, f.N); err != nil {
		return err
	}
	if len(f.SamplingRate) != f.M && len(f.SamplingRate) != 1 {
		return invalid("SamplingRate", "length %d must be M=%d or 1",
			len(f.SamplingRate), f.M)
	}
	delayLen := len(f.Delay)
	if delayLen != 0 && delayLen != 1 && delayLen != f.M && delayLen != f.R && delayLen != f.M*f.R {
		return invalid("Delay", "length %d must be 0 (optional), 1 (scalar), M=%d, R=%d, or M×R=%d",
			delayLen, f.M, f.R, f.M*f.R)
	}
	return nil
}

func check4D(name string, data [][][][]float64, m, r, e, n int) error {
	if len(data) != m {
		return invalid(name, "length %d does not match M=%d", len(data), m)
	}
	for i, mr := range data {
		if len(mr) != r {
			return invalid(name, "[%d] length %d does not match R=%d", i, len(mr), r)
		}
		for j, re := range mr {
			if len(re) != e {
				return invalid(name, "[%d][%d] length %d does not match E=%d",
					i, j, len(re), e)
			}
			for k, nn := range re {
				if len(nn) != n {
					return invalid(name, "[%d][%d][%d] length %d does not match N=%d",
						i, j, k, len(nn), n)
				}
			}
		}
	}
	return nil
}

func check3D(name string, data [][][]float64, m, r, n int) error {
	if len(data) != m {
		return invalid(name, "length %d does not match M=%d", len(data), m)
	}
	for i, mr := range data {
		if len(mr) != r {
			return invalid(name, "[%d] length %d does not match R=%d", i, len(mr), r)
		}
		for j, nn := range mr {
			if len(nn) != n {
				return invalid(name, "[%d][%d] length %d does not match N=%d",
					i, j, len(nn), n)
			}
		}
	}
	return nil
}

// writeAudioDatasets dispatches to the per-DataType writer.
func (f *File) writeAudioDatasets(nc *netcdfDimensions) error {
	switch f.DataType {
	case DataTypeFIR:
		return f.writeFIRAudioDatasets(nc)
	case DataTypeTF:
		return f.writeTFAudioDatasets(nc)
	case DataTypeTFE:
		return f.writeTFEAudioDatasets(nc)
	case DataTypeSOS:
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

// writeSamplingRateAndDelay writes Data.SamplingRate ([M] or [I]) and
// Data.Delay, shared by FIR and SOS.
func (f *File) writeSamplingRateAndDelay(nc *netcdfDimensions) error {
	if err := nc.writeVariableWithAttrs("/Data.SamplingRate", f.SamplingRate,
		[]string{rowDim(len(f.SamplingRate), dimM, f.M)},
		[]hdf5.DatasetOption{hdf5.WithAttribute("Units", "hertz")}); err != nil {
		return err
	}
	delay, dims := f.writtenDelay()
	return nc.writeVariable("/Data.Delay", delay, dims...)
}

// writtenDelay returns Data.Delay in one of the two layouts AES69 allows,
// [I,R] or [M,R]; the variable is mandatory for FIR and SOS. The 1-D
// layouts go-sofa accepts in memory are expanded by broadcasting ([I] and
// [R] to [I,R], [M] to [M,R]; see delayAxes for M == R), and an absent
// delay is written as zeros, the conventions' default.
func (f *File) writtenDelay() ([]float64, []string) {
	if len(f.Delay) == 0 {
		return make([]float64, f.R), []string{dimI, dimR}
	}
	axes := f.delayAxes()
	if len(axes) == 2 {
		return f.Delay, axes
	}
	rows, dims := 1, []string{dimI, dimR}
	if axes[0] == dimM {
		rows, dims = f.M, []string{dimM, dimR}
	}
	out := make([]float64, 0, rows*f.R)
	for m := range rows {
		for r := range f.R {
			// Indices are in range: validate checked len(f.Delay).
			v, _ := f.DelayAt(m, r)
			out = append(out, v)
		}
	}
	return out, dims
}

// delayDims names the dimensions of an in-memory Data.Delay with n values
// (validated to be 1, M, R or M×R). M×R is checked before M and R so that
// it keeps its [M,R] shape when M or R is 1, and M before R, so that a
// delay with M == R values is per measurement.
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

// writeTFEAudioDatasets writes Data.Real / Data.Imag for DataType ==
// "TF-E" as [M,R,N,E], the order of the GeneralTF-E and FreeFieldHRTF
// convention tables, transposing the in-memory [M][R][E][N]. The frequency
// vector is the /N coordinate variable as for plain TF.
func (f *File) writeTFEAudioDatasets(nc *netcdfDimensions) error {
	for _, v := range []struct {
		name string
		data [][][][]float64
	}{{"/Data.Real", f.TFRealE}, {"/Data.Imag", f.TFImagE}} {
		flat := swapLastAxes(flatten4D(v.data), f.M*f.R, f.E, f.N)
		if err := nc.writeVariable(v.name, flat, layoutMRNE...); err != nil {
			return err
		}
	}
	return nil
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
