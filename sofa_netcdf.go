package sofa

import (
	"fmt"
	"runtime/debug"
	"slices"
	"strings"

	hdf5 "github.com/cwbudde/go-hdf5"
)

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
	sofaVersion, hdf5Version := buildVersions()
	return fmt.Sprintf("version=2,go-sofa=%s,go-hdf5=%s", sofaVersion, hdf5Version)
}

// moduleVersion returns go-sofa's module version, the default APIVersion.
func moduleVersion() string {
	v, _ := buildVersions()
	return v
}

// buildVersions returns the versions of go-sofa and go-hdf5 in the running
// binary, "unknown" where the build info does not say.
func buildVersions() (sofaVersion, hdf5Version string) {
	sofaVersion, hdf5Version = "unknown", "unknown"
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
	return sofaVersion, hdf5Version
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
	attrs  map[string][]Attribute // File.VariableAttributes, added to every variable written by name
}

// writeDimensionScales writes one dimension-scale dataset per SOFA
// dimension, then one per further dimension the extra variables use (such
// as the string length S), in a fixed order (which is also their
// _Netcdf4Dimid order) so output is deterministic. For TF and TF-E, /N is
// the frequency coordinate variable; otherwise every scale is a netCDF
// "dimension without variable" whose length is the dimension size.
func (f *File) writeDimensionScales(fw *hdf5.FileWriter) (*netcdfDimensions, error) {
	nc := &netcdfDimensions{
		fw:     fw,
		sizes:  map[string]int{dimM: f.M, dimR: f.R, dimE: f.E, dimN: f.N, dimC: 3, dimI: 1},
		scales: map[string]*hdf5.DatasetWriter{},
		attrs:  f.VariableAttributes,
	}
	names := []string{dimM, dimR, dimE, dimN, dimC, dimI}
	for _, v := range f.Variables {
		for i, d := range v.Dims {
			if _, ok := nc.sizes[d]; !ok {
				nc.sizes[d] = v.Shape[i]
				names = append(names, d)
			}
		}
	}
	for id, name := range names {
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
		hdf5.WithAttribute("_Netcdf4Dimid", int32(id))) //nolint:gosec // id < number of dimensions
	if err != nil {
		return nil, fmt.Errorf("create dimension dataset: %w", err)
	}
	return ds, nil
}

// writeFrequencyDimension writes /N as a vector of frequency values (Hz).
// The dataset is the netCDF coordinate variable of dimension N:
// CLASS=DIMENSION_SCALE and NAME equal to the dimension label, matching
// what upstream tools emit for /N in TF files, plus the LongName and Units
// the TF conventions require.
func writeFrequencyDimension(fw *hdf5.FileWriter, freqs []float64, id int) (*hdf5.DatasetWriter, error) {
	if len(freqs) == 0 {
		return nil, fmt.Errorf("frequencies must be non-empty for TF data")
	}
	ds, err := fw.CreateDataset("/N", hdf5.Float64,
		[]uint64{uint64(len(freqs))},
		hdf5.WithAttribute("CLASS", "DIMENSION_SCALE"),
		hdf5.WithAttribute("NAME", dimN),
		hdf5.WithAttribute("_Netcdf4Dimid", int32(id)), //nolint:gosec // id < number of dimensions
		hdf5.WithAttribute("LongName", "frequency"),
		hdf5.WithAttribute("Units", "hertz"))
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

	if extra := nc.attrs[strings.TrimPrefix(name, "/")]; len(extra) > 0 {
		attrs = append(slices.Clip(attrs), attributeOptions(extra)...)
	}
	ds, err := nc.fw.CreateDataset(name, hdf5.Float64, shape, attrs...)
	if err != nil {
		return fmt.Errorf("create %s dataset: %w", name, err)
	}
	if err := ds.Write(data); err != nil {
		return fmt.Errorf("write %s data: %w", name, err)
	}
	for i, d := range dims {
		if err := ds.AttachDimensionScale(i, nc.scales[d]); err != nil {
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

	return nc.writeVariableWithAttrs(name, flattenVector3s(positions), []string{rows, dimC},
		positionAttributes(typ, units))
}

// writePositionDatasetPerM writes per-measurement positions perM ([M][X])
// as a variable [rows, C, M], the layout Open reads into ReceiverPositionsM
// and EmitterPositionsM.
func (nc *netcdfDimensions) writePositionDatasetPerM(name string, perM [][]Vector3, rows, typ, units string) error {
	m, x := len(perM), len(perM[0])
	flat := make([]float64, x*3*m)
	for i, row := range perM {
		for j, v := range row {
			for c, val := range [3]float64{v.X, v.Y, v.Z} {
				flat[(j*3+c)*m+i] = val
			}
		}
	}
	return nc.writeVariableWithAttrs(name, flat, []string{rows, dimC, dimM}, positionAttributes(typ, units))
}

// listenerViewCoordinates returns the Type and Units written on
// ListenerView and ListenerUp: the File's, defaulting to the conventions'
// "cartesian" and "metre". Empty Units of a non-cartesian Type default to
// UnitsSphericalDegrees, so the mandatory Units attribute is never omitted.
func (f *File) listenerViewCoordinates() (typ, units string) {
	typ, units = f.ListenerViewType, f.ListenerViewUnits
	if typ == "" {
		typ = CoordinateCartesian
	}
	if units == "" {
		units = UnitsSphericalDegrees
		if strings.EqualFold(typ, CoordinateCartesian) {
			units = "metre"
		}
	}
	return typ, units
}

// positionAttributes returns the Type and Units attributes of a position
// variable. Empty values are omitted rather than written as empty strings.
func positionAttributes(typ, units string) []hdf5.DatasetOption {
	var attrs []hdf5.DatasetOption
	if typ != "" {
		attrs = append(attrs, hdf5.WithAttribute("Type", typ))
	}
	if units != "" {
		attrs = append(attrs, hdf5.WithAttribute("Units", units))
	}
	return attrs
}
