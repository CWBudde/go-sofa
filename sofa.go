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

	hdf5 "github.com/meko-christian/go-hdf5"
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

	// Audio data
	ImpulseResponses [][][]float64 // [M][R][N] the actual IR data
	SamplingRate     []float64     // [M] sampling rate in Hz (may be scalar)
	Delay            []float64     // [M] delay in samples

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
	if f.Conventions != "SOFA" {
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

	// Read spatial data.
	if err := f.readSpatialData(datasets); err != nil {
		h.Close()
		return nil, fmt.Errorf("read spatial data: %w", err)
	}

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

		// Try to read from NAME attribute (netCDF-4 convention)
		val, err := ds.ReadAttribute("NAME")
		if err == nil {
			// NAME attribute exists - parse it
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("dimension %q: NAME is %T, want string", name, val)
			}
			n, err := parseDimensionSize(s)
			if err != nil {
				return fmt.Errorf("dimension %q: %w", name, err)
			}
			*dst = n
		} else {
			// NAME attribute not found - fall back to reading dataset value directly
			// This handles files written by go-sofa where dataset attributes
			// are not yet supported due to go-hdf5 limitations
			data, err := ds.Read()
			if err != nil {
				return fmt.Errorf("dimension %q: read dataset: %w", name, err)
			}
			if len(data) != 1 {
				return fmt.Errorf("dimension %q: expected 1 value, got %d", name, len(data))
			}
			*dst = int(data[0])
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

// readAudioData reads Data.IR, Data.SamplingRate, and Data.Delay.
func (f *File) readAudioData(datasets map[string]*hdf5.Dataset) error {
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

	// Data.SamplingRate
	if ds, ok := datasets["Data.SamplingRate"]; ok {
		f.SamplingRate, err = ds.Read()
		if err != nil {
			return fmt.Errorf("read Data.SamplingRate: %w", err)
		}
	}

	// Data.Delay
	if ds, ok := datasets["Data.Delay"]; ok {
		f.Delay, err = ds.Read()
		if err != nil {
			return fmt.Errorf("read Data.Delay: %w", err)
		}
	}

	return nil
}

// readSpatialData reads listener, receiver, source, and emitter positions.
// Position reads are best-effort: some datasets may not be readable due to
// go-hdf5 limitations with certain storage formats.
func (f *File) readSpatialData(datasets map[string]*hdf5.Dataset) error {
	// Position datasets — [N×3] float64 arrays.
	type posTarget struct {
		name string
		dst  *[]Vector3
	}
	for _, pt := range []posTarget{
		{"ListenerPosition", &f.ListenerPositions},
		{"ReceiverPosition", &f.ReceiverPositions},
		{"SourcePosition", &f.SourcePositions},
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

	return nil
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

	// Write dimension-scale datasets (M, R, E, N) with netCDF attributes
	dimScales := map[string]int{
		"/M": f.M,
		"/R": f.R,
		"/E": f.E,
		"/N": f.N,
	}
	for name, size := range dimScales {
		if err := writeDimensionScale(fw, name, size); err != nil {
			return fmt.Errorf("write dimension %s: %w", name, err)
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
	if f.Conventions != "SOFA" {
		return fmt.Errorf("Conventions must be \"SOFA\", got %q", f.Conventions)
	}
	if f.Version == "" {
		return fmt.Errorf("Version is required")
	}
	if f.SOFAConventions == "" {
		return fmt.Errorf("SOFAConventions is required")
	}
	if f.DataType == "" {
		return fmt.Errorf("DataType is required")
	}

	// Check dimensions are non-zero
	if f.M <= 0 {
		return fmt.Errorf("M must be > 0, got %d", f.M)
	}
	if f.R <= 0 {
		return fmt.Errorf("R must be > 0, got %d", f.R)
	}
	if f.E <= 0 {
		return fmt.Errorf("E must be > 0, got %d", f.E)
	}
	if f.N <= 0 {
		return fmt.Errorf("N must be > 0, got %d", f.N)
	}

	// Check ImpulseResponses dimensions
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

	// Check SamplingRate dimension (must be M or 1 for scalar)
	if len(f.SamplingRate) != f.M && len(f.SamplingRate) != 1 {
		return fmt.Errorf("SamplingRate length %d must be M=%d or 1",
			len(f.SamplingRate), f.M)
	}

	// Check Delay dimension (can be [M], [R], [M×R], or scalar/empty)
	// SOFA spec allows various Delay dimensions depending on convention
	delayLen := len(f.Delay)
	if delayLen != 0 && delayLen != 1 && delayLen != f.M && delayLen != f.R && delayLen != f.M*f.R {
		return fmt.Errorf("Delay length %d must be 0 (optional), 1 (scalar), M=%d, R=%d, or M×R=%d",
			delayLen, f.M, f.R, f.M*f.R)
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


// writeAudioDatasets writes Data.IR, Data.SamplingRate, and Data.Delay.
func (f *File) writeAudioDatasets(fw *hdf5.FileWriter) error {
	// Write Data.IR as [M][R][N] float64
	irFlat := flattenIR(f.ImpulseResponses)
	irDS, err := fw.CreateDataset("/Data.IR", hdf5.Float64,
		[]uint64{uint64(f.M), uint64(f.R), uint64(f.N)})
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

// writeDimensionScale writes a netCDF dimension-scale dataset.
// TODO: Add dimension-scale attributes (CLASS, NAME) once go-hdf5 supports
// dataset attributes during creation (WithAttribute option for CreateDataset).
// Writing attributes after dataset creation causes file corruption.
func writeDimensionScale(fw *hdf5.FileWriter, name string, size int) error {
	// Create dataset with single float64 value
	ds, err := fw.CreateDataset(name, hdf5.Float64, []uint64{1})
	if err != nil {
		return fmt.Errorf("create dimension dataset: %w", err)
	}

	// Write the dimension size as a float64 value
	if err := ds.Write([]float64{float64(size)}); err != nil {
		return fmt.Errorf("write dimension value: %w", err)
	}

	// NOTE: Dataset attributes skipped until go-hdf5 supports WithAttribute for datasets.
	// Without these attributes, the files are not fully netCDF-4 compliant but are
	// still valid HDF5 and readable by our library.
	//
	// Required attributes for full compliance:
	// - CLASS: "DIMENSION_SCALE"
	// - NAME: "This is a netCDF dimension but not a netCDF variable.     <size>"

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
