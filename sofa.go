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
		val, err := ds.ReadAttribute("NAME")
		if err != nil {
			return fmt.Errorf("dimension %q: read NAME: %w", name, err)
		}
		s, ok := val.(string)
		if !ok {
			return fmt.Errorf("dimension %q: NAME is %T, want string", name, val)
		}
		n, err := parseDimensionSize(s)
		if err != nil {
			return fmt.Errorf("dimension %q: %w", name, err)
		}
		*dst = n
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
