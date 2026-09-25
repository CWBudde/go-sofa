package sofa

import (
	"fmt"

	hdf5 "github.com/cwbudde/go-hdf5"
)

// netCDF dimension names used by SOFA (AES69): M measurements, R receivers,
// E emitters, N samples / frequencies, C coordinate triplets, I singleton.
const (
	dimM = "M"
	dimR = "R"
	dimE = "E"
	dimN = "N"
	dimC = "C"
	dimI = "I"
)

// ncDimOrder is the netCDF definition order of the dimensions; a
// dimension's index in it is its _Netcdf4Dimid.
var ncDimOrder = []string{dimM, dimR, dimE, dimN, dimC, dimI}

// netcdfDimensionNAME formats the NAME attribute netCDF-C writes on a
// dimension-scale dataset that is a dimension but not a variable
// (nc4hdf.c: "This is a netCDF dimension but not a netCDF variable.%10d").
// The trailing number is the dimension size; the reader parses it back.
func netcdfDimensionNAME(size int) string {
	return fmt.Sprintf("This is a netCDF dimension but not a netCDF variable.%10d", size)
}

// ncWriter writes SOFA variables as netCDF-4 variables: every dimension is
// an HDF5 dimension scale, and every variable has its dimensions attached
// (DIMENSION_LIST / REFERENCE_LIST, written by go-hdf5 on Close) plus the
// _Netcdf4Coordinates attribute listing its dimension ids.
type ncWriter struct {
	fw     *hdf5.FileWriter
	sizes  map[string]int
	scales map[string]*hdf5.DatasetWriter
}

// newNCWriter writes the dimension-scale datasets /M, /R, /E, /N, /C and
// /I. /N is the frequency coordinate variable for TF and TF-E data and a
// plain dimension otherwise.
func (f *File) newNCWriter(fw *hdf5.FileWriter) (*ncWriter, error) {
	w := &ncWriter{
		fw: fw,
		sizes: map[string]int{
			dimM: f.M, dimR: f.R, dimE: f.E, dimN: f.N, dimC: 3, dimI: 1,
		},
		scales: make(map[string]*hdf5.DatasetWriter, len(ncDimOrder)),
	}
	for id, name := range ncDimOrder {
		var err error
		if name == dimN && (f.DataType == dataTypeTF || f.DataType == dataTypeTFE) {
			err = w.writeCoordinateVariable(name, int32(id), f.Frequencies) //nolint:gosec // G115: < len(ncDimOrder)
		} else {
			err = w.writeDimension(name, int32(id)) //nolint:gosec // G115: < len(ncDimOrder)
		}
		if err != nil {
			return nil, fmt.Errorf("write dimension /%s: %w", name, err)
		}
	}
	return w, nil
}

// writeDimension writes a netCDF "dimension without variable": a float
// dataset whose length is the dimension size, marked as a dimension scale.
func (w *ncWriter) writeDimension(name string, id int32) error {
	size := w.sizes[name]
	ds, err := w.fw.CreateDataset("/"+name, hdf5.Float32, []uint64{uint64(size)}, //nolint:gosec // G115: size > 0 by validate()
		hdf5.WithAttribute("CLASS", "DIMENSION_SCALE"),
		hdf5.WithAttribute("NAME", netcdfDimensionNAME(size)),
		hdf5.WithAttribute("_Netcdf4Dimid", id))
	if err != nil {
		return fmt.Errorf("create dataset: %w", err)
	}
	if err := ds.Write(make([]float32, size)); err != nil {
		return fmt.Errorf("write dataset: %w", err)
	}
	w.scales[name] = ds
	return nil
}

// writeCoordinateVariable writes a netCDF coordinate variable: a dimension
// scale named after its dimension that also carries values.
func (w *ncWriter) writeCoordinateVariable(name string, id int32, values []float64) error {
	if len(values) != w.sizes[name] {
		return fmt.Errorf("coordinate variable %s has %d values, want %d", name, len(values), w.sizes[name])
	}
	ds, err := w.fw.CreateDataset("/"+name, hdf5.Float64, []uint64{uint64(len(values))},
		hdf5.WithAttribute("CLASS", "DIMENSION_SCALE"),
		hdf5.WithAttribute("NAME", name),
		hdf5.WithAttribute("_Netcdf4Dimid", id))
	if err != nil {
		return fmt.Errorf("create dataset: %w", err)
	}
	if err := ds.Write(values); err != nil {
		return fmt.Errorf("write dataset: %w", err)
	}
	w.scales[name] = ds
	return nil
}

// writeVariable writes a float64 variable with the given netCDF dimensions.
// The dataset shape follows from the dimension sizes; data is row-major.
func (w *ncWriter) writeVariable(name string, dims []string, data []float64, opts ...hdf5.DatasetOption) error {
	shape := make([]uint64, len(dims))
	ids := make([]int32, len(dims))
	count := 1
	for i, d := range dims {
		size, ok := w.sizes[d]
		if !ok {
			return fmt.Errorf("%s: unknown dimension %q", name, d)
		}
		shape[i] = uint64(size) //nolint:gosec // G115: size > 0 by validate()
		count *= size
		for id, n := range ncDimOrder {
			if n == d {
				ids[i] = int32(id) //nolint:gosec // G115: < len(ncDimOrder)
			}
		}
	}
	if len(data) != count {
		return fmt.Errorf("%s: %d values do not match dimensions %v (%d)", name, len(data), dims, count)
	}

	opts = append(opts, hdf5.WithAttribute("_Netcdf4Coordinates", ids))
	ds, err := w.fw.CreateDataset("/"+name, hdf5.Float64, shape, opts...)
	if err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	if err := ds.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	for i, d := range dims {
		if err := ds.AttachDimensionScale(i, w.scales[d]); err != nil {
			return fmt.Errorf("attach dimension %s to %s: %w", d, name, err)
		}
	}
	return nil
}

// dimFor returns the dimension of a per-measurement (or per-receiver,
// per-emitter) quantity with n entries: primary when n matches its size,
// I when n == 1.
func (w *ncWriter) dimFor(primary string, n int) (string, error) {
	switch n {
	case w.sizes[primary]:
		return primary, nil
	case 1:
		return dimI, nil
	default:
		return "", fmt.Errorf("length %d matches neither %s=%d nor 1", n, primary, w.sizes[primary])
	}
}

// writeVectors writes Vector3 values as an [n][C] variable, where n is the
// primary dimension or I (see dimFor). Empty input writes nothing.
func (w *ncWriter) writeVectors(name, primary string, vecs []Vector3, opts ...hdf5.DatasetOption) error {
	if len(vecs) == 0 {
		return nil
	}
	d, err := w.dimFor(primary, len(vecs))
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return w.writeVariable(name, []string{d, dimC}, flattenVector3s(vecs), opts...)
}

// writePositions writes a position variable tagged with the Type and Units
// attributes naming its coordinate system; empty values are omitted.
func (w *ncWriter) writePositions(name, primary string, positions []Vector3, typ, units string) error {
	var opts []hdf5.DatasetOption
	if typ != "" {
		opts = append(opts, hdf5.WithAttribute("Type", typ))
	}
	if units != "" {
		opts = append(opts, hdf5.WithAttribute("Units", units))
	}
	return w.writeVectors(name, primary, positions, opts...)
}

// writeSamplingRate writes Data.SamplingRate as [M] or [I].
func (w *ncWriter) writeSamplingRate(sr []float64) error {
	d, err := w.dimFor(dimM, len(sr))
	if err != nil {
		return fmt.Errorf("Data.SamplingRate: %w", err)
	}
	return w.writeVariable("Data.SamplingRate", []string{d}, sr)
}

// writeDelay writes Data.Delay (when present) with the shape its length
// implies: 1 → [I], M → [M], R → [I,R], M×R → [M,R]. The row-major values
// are unchanged, so readers that take the flat data see the same slice.
func (w *ncWriter) writeDelay(delay []float64) error {
	m, r := w.sizes[dimM], w.sizes[dimR]
	var dims []string
	switch n := len(delay); {
	case n == 0:
		return nil
	case n == 1:
		dims = []string{dimI}
	case n == m*r && m > 1 && r > 1:
		dims = []string{dimM, dimR}
	case n == m:
		dims = []string{dimM}
	case n == r:
		dims = []string{dimI, dimR}
	default:
		return fmt.Errorf("Data.Delay: length %d matches none of 1, M=%d, R=%d, M×R=%d", n, m, r, m*r)
	}
	return w.writeVariable("Data.Delay", dims, delay)
}
