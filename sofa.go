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
	"fmt"
	"math"
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
	dataTypeFIR = "FIR" // time-domain impulse responses
	dataTypeTF  = "TF"  // complex frequency-domain transfer functions

	// datasetSourcePosition is the SOFA dataset name for source-position data.
	datasetSourcePosition = "SourcePosition"
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
	E int // number of emitters (typically 1)
	N int // number of samples per impulse response

	// Spatial data
	ListenerPositions []Vector3 // [M] listener positions for each measurement
	ListenerUp        Vector3   // listener's up vector
	ListenerView      Vector3   // listener's view direction
	ReceiverPositions []Vector3 // [R] receiver positions (e.g., left/right ear)
	SourcePositions   []Vector3 // [M] source positions for each measurement
	EmitterPositions  []Vector3 // [E] emitter positions

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

	// AES69 Metadata (global attributes)
	Conventions            string // "SOFA" for SOFA files
	Version                string // SOFA version (e.g., "1.0")
	SOFAConventions        string // specific convention (e.g., "SimpleFreeFieldHRIR")
	SOFAConventionsVersion string // convention version
	DataType               string // data type (e.g., "FIR")
	RoomType               string // room type if applicable
	Title                  string // descriptive title
	DateCreated            string // ISO 8601 date
	DateModified           string // ISO 8601 date
	APIName                string // API used to create the file
	APIVersion             string // API version
	AuthorContact          string // author contact information
	Organization           string // organization
	License                string // license information
	ApplicationName        string // application name
	ApplicationVersion     string // application version
	Comment                string // additional comments
	History                string // processing history
	References             string // references
	Origin                 string // origin of the data

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

// readDimensions extracts M, R, E, N from dimension-scale datasets.
// These datasets have a NAME attribute containing the dimension size.
func (f *File) readDimensions(datasets map[string]*hdf5.Dataset) error {
	dims := map[string]*int{
		"M": &f.M,
		"R": &f.R,
		"E": &f.E,
		"N": &f.N,
	}

	for name, dst := range dims {
		ds, ok := datasets[name]
		if !ok {
			return fmt.Errorf("dimension dataset %q not found", name)
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
				if n, perr := parseDimensionSize(s); perr == nil {
					*dst = n
					continue
				}
				hasCoordNAME = true
			}
		}

		data, err := ds.Read()
		if err != nil {
			return fmt.Errorf("dimension %q: read dataset: %w", name, err)
		}
		switch {
		case len(data) == 0:
			return fmt.Errorf("dimension %q: empty dataset", name)
		case len(data) > 1:
			// Either /N as a TF frequency vector or any coord variable
			// with multiple elements — dataspace length is the size.
			*dst = len(data)
		case hasCoordNAME:
			// netCDF coordinate variable with one element: dataspace
			// shape is the size; the (possibly zero) value is unrelated.
			*dst = len(data)
		case name == "N":
			// Scalar /N from go-sofa-written FIR files: the value is
			// the sample count.
			*dst = int(data[0])
		default:
			// /M, /R, /E from go-sofa-written files: scalar carrying the
			// count. Fall back to len if the value is unset.
			if v := int(data[0]); v > 0 {
				*dst = v
			} else {
				*dst = len(data)
			}
		}
	}
	return nil
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
	if f.DataType == dataTypeTF {
		return f.readTFAudioData(datasets)
	}
	return f.readFIRAudioData(datasets)
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
	// /N holds the frequency vector for TF data.
	if ds, ok := datasets["N"]; ok {
		freqs, err := ds.Read()
		if err != nil {
			return fmt.Errorf("read /N (frequencies): %w", err)
		}
		// readDimensions accepts both scalar /N (FIR-shaped) and vector /N
		// (TF-shaped). For TF we expect len(freqs) == N. If we got a scalar
		// (len==1) that means N is unset on disk in TF form; reject.
		switch {
		case len(freqs) == f.N:
			f.Frequencies = freqs
		case len(freqs) == 1:
			return fmt.Errorf("/N is scalar but DataType=TF expects frequency vector of length %d", f.N)
		default:
			return fmt.Errorf("/N length %d does not match N=%d", len(freqs), f.N)
		}
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

// readSpatialData reads listener, receiver, source, and emitter positions.
// Position reads are best-effort: some datasets may not be readable due to
// go-hdf5 limitations with certain storage formats.
func (f *File) readSpatialData(datasets map[string]*hdf5.Dataset) {
	// Position datasets — [N×3] float64 arrays.
	type posTarget struct {
		name string
		dst  *[]Vector3
	}
	for _, pt := range []posTarget{
		{"ListenerPosition", &f.ListenerPositions},
		{"ReceiverPosition", &f.ReceiverPositions},
		{datasetSourcePosition, &f.SourcePositions},
		{"EmitterPosition", &f.EmitterPositions},
	} {
		if ds, ok := datasets[pt.name]; ok {
			if vecs, err := readVector3s(ds); err == nil {
				*pt.dst = vecs
			}
		}
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
			result[i][j] = flat[start : start+n]
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
// Returns nil if indices are out of range.
func (f *File) IRAt(m, r int) []float64 {
	if m < 0 || m >= f.M || r < 0 || r >= f.R {
		return nil
	}
	return f.ImpulseResponses[m][r]
}

// IRPeakdB returns the peak level in dB (relative to 1.0) for measurement m, receiver r.
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
// The file is created from scratch each time, ensuring no corruption of the original.
// All required SOFA attributes and datasets are written, along with optional fields
// if present in the File struct.
//
// Returns an error if:
//   - Validation fails (missing required fields, invalid dimensions, etc.)
//   - HDF5 file creation fails
//   - Any write operation fails
func (f *File) Save(path string) error {
	// Validate the File struct before writing
	if err := f.validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Prepare root attributes - collect all global metadata
	var rootAttrs []interface{}

	// Add required attributes
	rootAttrs = append(rootAttrs,
		hdf5.WithRootAttribute("Conventions", f.Conventions),
		hdf5.WithRootAttribute("Version", f.Version),
		hdf5.WithRootAttribute("SOFAConventions", f.SOFAConventions),
		hdf5.WithRootAttribute("SOFAConventionsVersion", f.SOFAConventionsVersion),
		hdf5.WithRootAttribute("DataType", f.DataType),
	)

	// Add optional attributes only if non-empty
	if f.Title != "" {
		rootAttrs = append(rootAttrs, hdf5.WithRootAttribute("Title", f.Title))
	}
	if f.DateCreated != "" {
		rootAttrs = append(rootAttrs, hdf5.WithRootAttribute("DateCreated", f.DateCreated))
	}
	if f.DateModified != "" {
		rootAttrs = append(rootAttrs, hdf5.WithRootAttribute("DateModified", f.DateModified))
	}
	if f.APIName != "" {
		rootAttrs = append(rootAttrs, hdf5.WithRootAttribute("APIName", f.APIName))
	}
	if f.APIVersion != "" {
		rootAttrs = append(rootAttrs, hdf5.WithRootAttribute("APIVersion", f.APIVersion))
	}
	if f.AuthorContact != "" {
		rootAttrs = append(rootAttrs, hdf5.WithRootAttribute("AuthorContact", f.AuthorContact))
	}
	if f.Organization != "" {
		rootAttrs = append(rootAttrs, hdf5.WithRootAttribute("Organization", f.Organization))
	}
	if f.License != "" {
		rootAttrs = append(rootAttrs, hdf5.WithRootAttribute("License", f.License))
	}
	if f.ApplicationName != "" {
		rootAttrs = append(rootAttrs, hdf5.WithRootAttribute("ApplicationName", f.ApplicationName))
	}
	if f.ApplicationVersion != "" {
		rootAttrs = append(rootAttrs, hdf5.WithRootAttribute("ApplicationVersion", f.ApplicationVersion))
	}
	if f.Comment != "" {
		rootAttrs = append(rootAttrs, hdf5.WithRootAttribute("Comment", f.Comment))
	}
	if f.History != "" {
		rootAttrs = append(rootAttrs, hdf5.WithRootAttribute("History", f.History))
	}
	if f.References != "" {
		rootAttrs = append(rootAttrs, hdf5.WithRootAttribute("References", f.References))
	}
	if f.Origin != "" {
		rootAttrs = append(rootAttrs, hdf5.WithRootAttribute("Origin", f.Origin))
	}
	if f.RoomType != "" {
		rootAttrs = append(rootAttrs, hdf5.WithRootAttribute("RoomType", f.RoomType))
	}

	// Create HDF5 file with root attributes
	fw, err := hdf5.CreateForWrite(path, hdf5.CreateTruncate, rootAttrs...)
	if err != nil {
		return fmt.Errorf("create HDF5 file: %w", err)
	}
	defer fw.Close()

	// Write dimension-scale datasets (M, R, E) with netCDF attributes.
	// /N is written separately: scalar size for FIR, frequency vector for TF.
	dimScales := map[string]int{
		"/M": f.M,
		"/R": f.R,
		"/E": f.E,
	}
	for name, size := range dimScales {
		if err := writeDimensionScale(fw, name, size); err != nil {
			return fmt.Errorf("write dimension %s: %w", name, err)
		}
	}
	if f.DataType == dataTypeTF {
		if err := writeFrequencyDimension(fw, f.Frequencies); err != nil {
			return fmt.Errorf("write /N (frequencies): %w", err)
		}
	} else {
		if err := writeDimensionScale(fw, "/N", f.N); err != nil {
			return fmt.Errorf("write dimension /N: %w", err)
		}
	}

	// Write spatial position datasets
	if err := writePositionDataset(fw, "/ListenerPosition", f.ListenerPositions); err != nil {
		return fmt.Errorf("write ListenerPosition: %w", err)
	}
	if err := writePositionDataset(fw, "/ReceiverPosition", f.ReceiverPositions); err != nil {
		return fmt.Errorf("write ReceiverPosition: %w", err)
	}
	if err := writePositionDataset(fw, "/SourcePosition", f.SourcePositions); err != nil {
		return fmt.Errorf("write SourcePosition: %w", err)
	}
	if err := writePositionDataset(fw, "/EmitterPosition", f.EmitterPositions); err != nil {
		return fmt.Errorf("write EmitterPosition: %w", err)
	}

	// Write listener orientation vectors
	if err := writeVector3Dataset(fw, "/ListenerUp", []Vector3{f.ListenerUp}); err != nil {
		return fmt.Errorf("write ListenerUp: %w", err)
	}
	if err := writeVector3Dataset(fw, "/ListenerView", []Vector3{f.ListenerView}); err != nil {
		return fmt.Errorf("write ListenerView: %w", err)
	}

	// Write audio data
	if err := f.writeAudioDatasets(fw); err != nil {
		return fmt.Errorf("write audio data: %w", err)
	}

	return nil
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
	default:
		return fmt.Errorf("unsupported DataType %q (want %q or %q)",
			f.DataType, dataTypeFIR, dataTypeTF)
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

	return nil
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

// writeAudioDatasets dispatches to the FIR or TF writer based on DataType.
func (f *File) writeAudioDatasets(fw *hdf5.FileWriter) error {
	switch f.DataType {
	case dataTypeFIR:
		return f.writeFIRAudioDatasets(fw)
	case dataTypeTF:
		return f.writeTFAudioDatasets(fw)
	default:
		return fmt.Errorf("unsupported DataType %q", f.DataType)
	}
}

// writeFIRAudioDatasets writes Data.IR, Data.SamplingRate, and Data.Delay.
func (f *File) writeFIRAudioDatasets(fw *hdf5.FileWriter) error {
	// Write Data.IR as [M][R][N] float64
	irFlat := flattenIR(f.ImpulseResponses)
	// Dimensions are validated > 0 by validate() before Save() reaches here.
	irDS, err := fw.CreateDataset("/Data.IR", hdf5.Float64,
		[]uint64{uint64(f.M), uint64(f.R), uint64(f.N)}) //nolint:gosec // dims > 0 by validate()
	if err != nil {
		return fmt.Errorf("create Data.IR dataset: %w", err)
	}
	if err := irDS.Write(irFlat); err != nil {
		return fmt.Errorf("write Data.IR data: %w", err)
	}

	// Write Data.SamplingRate
	srDS, err := fw.CreateDataset("/Data.SamplingRate", hdf5.Float64,
		[]uint64{uint64(len(f.SamplingRate))})
	if err != nil {
		return fmt.Errorf("create Data.SamplingRate dataset: %w", err)
	}
	if err := srDS.Write(f.SamplingRate); err != nil {
		return fmt.Errorf("write Data.SamplingRate data: %w", err)
	}

	// Write Data.Delay (if present)
	if len(f.Delay) > 0 {
		delayDS, err := fw.CreateDataset("/Data.Delay", hdf5.Float64,
			[]uint64{uint64(len(f.Delay))})
		if err != nil {
			return fmt.Errorf("create Data.Delay dataset: %w", err)
		}
		if err := delayDS.Write(f.Delay); err != nil {
			return fmt.Errorf("write Data.Delay data: %w", err)
		}
	}

	return nil
}

// writeTFAudioDatasets writes Data.Real and Data.Imag for TF data.
// The frequency vector is written separately as /N (see writeFrequencyDimension).
func (f *File) writeTFAudioDatasets(fw *hdf5.FileWriter) error {
	// Dimensions are validated > 0 by validate() before Save() reaches here.
	dims := []uint64{uint64(f.M), uint64(f.R), uint64(f.N)} //nolint:gosec // dims > 0 by validate()

	realFlat := flattenIR(f.TFReal)
	realDS, err := fw.CreateDataset("/Data.Real", hdf5.Float64, dims)
	if err != nil {
		return fmt.Errorf("create Data.Real dataset: %w", err)
	}
	if err := realDS.Write(realFlat); err != nil {
		return fmt.Errorf("write Data.Real data: %w", err)
	}

	imagFlat := flattenIR(f.TFImag)
	imagDS, err := fw.CreateDataset("/Data.Imag", hdf5.Float64, dims)
	if err != nil {
		return fmt.Errorf("create Data.Imag dataset: %w", err)
	}
	if err := imagDS.Write(imagFlat); err != nil {
		return fmt.Errorf("write Data.Imag data: %w", err)
	}

	return nil
}

// writeFrequencyDimension writes /N as a vector of frequency values (Hz).
// The dimension size N is implied by the dataset length, which differs from
// FIR /N (a scalar holding the count). The dataset is marked as a netCDF
// coordinate variable: CLASS=DIMENSION_SCALE and NAME equal to the
// dimension label, matching what upstream tools emit for /N in TF files.
func writeFrequencyDimension(fw *hdf5.FileWriter, freqs []float64) error {
	if len(freqs) == 0 {
		return fmt.Errorf("frequencies must be non-empty for TF data")
	}
	ds, err := fw.CreateDataset("/N", hdf5.Float64,
		[]uint64{uint64(len(freqs))}, //nolint:gosec // length non-negative
		hdf5.WithAttribute("CLASS", "DIMENSION_SCALE"),
		hdf5.WithAttribute("NAME", "N"))
	if err != nil {
		return fmt.Errorf("create /N dataset: %w", err)
	}
	if err := ds.Write(freqs); err != nil {
		return fmt.Errorf("write /N values: %w", err)
	}
	return nil
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

// netcdfDimensionNAME formats the netCDF-4 NAME attribute used on
// dimension-scale datasets that are *not* coordinate variables. The
// trailing decimal is the dimension size; this is the form emitted by
// the reference netCDF-4 / MATLAB SOFA Toolbox writers and is what
// our reader expects when parsing pre-existing files.
func netcdfDimensionNAME(size int) string {
	return fmt.Sprintf("This is a netCDF dimension but not a netCDF variable.         %d", size)
}

// writeDimensionScale writes a netCDF dimension-scale dataset with the
// standard CLASS=DIMENSION_SCALE and a NAME carrying the dimension
// size, so files are netCDF-4 compliant and consumable by tools such
// as the MATLAB SOFA Toolbox.
func writeDimensionScale(fw *hdf5.FileWriter, name string, size int) error {
	ds, err := fw.CreateDataset(name, hdf5.Float64, []uint64{1},
		hdf5.WithAttribute("CLASS", "DIMENSION_SCALE"),
		hdf5.WithAttribute("NAME", netcdfDimensionNAME(size)))
	if err != nil {
		return fmt.Errorf("create dimension dataset: %w", err)
	}

	if err := ds.Write([]float64{float64(size)}); err != nil {
		return fmt.Errorf("write dimension value: %w", err)
	}
	return nil
}

// writePositionDataset writes a position dataset as [N×3] float64 array.
func writePositionDataset(fw *hdf5.FileWriter, name string, positions []Vector3) error {
	if len(positions) == 0 {
		// Skip if no positions provided
		return nil
	}

	ds, err := fw.CreateDataset(name, hdf5.Float64,
		[]uint64{uint64(len(positions)), 3})
	if err != nil {
		return fmt.Errorf("create dataset: %w", err)
	}

	data := flattenVector3s(positions)
	if err := ds.Write(data); err != nil {
		return fmt.Errorf("write data: %w", err)
	}

	return nil
}

// writeVector3Dataset writes a Vector3 dataset (for orientations like ListenerUp).
func writeVector3Dataset(fw *hdf5.FileWriter, name string, vecs []Vector3) error {
	if len(vecs) == 0 {
		return nil
	}

	ds, err := fw.CreateDataset(name, hdf5.Float64,
		[]uint64{uint64(len(vecs)), 3})
	if err != nil {
		return fmt.Errorf("create dataset: %w", err)
	}

	data := flattenVector3s(vecs)
	if err := ds.Write(data); err != nil {
		return fmt.Errorf("write data: %w", err)
	}

	return nil
}
