package sofa

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	hdf5 "github.com/cwbudde/go-hdf5"
)

// craftedVar is one dataset of a crafted file: its shape, its row-major
// values (zero-filled when nil) and optional string attributes.
type craftedVar struct {
	shape []uint64
	data  []float64
	attrs map[string]string
}

// craftedSpec describes a SOFA file written directly through go-hdf5,
// bypassing Save's validation, so reader behaviour on shapes and
// DataTypes Save would never produce can be tested.
type craftedSpec struct {
	dataType string         // omitted from the file when ""
	dims     map[string]int // dimension scales, written at full length
	vars     map[string]craftedVar
}

func writeCraftedSpec(t *testing.T, spec craftedSpec) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "crafted.sofa")
	opts := []any{
		hdf5.WithRootAttribute("Conventions", "SOFA"),
		hdf5.WithRootAttribute("Version", "2.1"),
		hdf5.WithRootAttribute("SOFAConventions", "General"),
		hdf5.WithRootAttribute("SOFAConventionsVersion", "1.0"),
	}
	if spec.dataType != "" {
		opts = append(opts, hdf5.WithRootAttribute("DataType", spec.dataType))
	}
	fw, err := hdf5.CreateForWrite(path, hdf5.CreateTruncate, opts...)
	if err != nil {
		t.Fatalf("CreateForWrite: %v", err)
	}
	for _, name := range []string{dimM, dimR, dimE, dimN, dimC, dimI} {
		size, ok := spec.dims[name]
		if !ok {
			continue
		}
		write(t, fw, name, craftedVar{
			shape: []uint64{uint64(size)}, //nolint:gosec // small test sizes
			attrs: map[string]string{"NAME": dimNAME(strconv.Itoa(size))},
		})
	}
	for name, v := range spec.vars {
		write(t, fw, name, v)
	}
	if err := fw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return path
}

func write(t *testing.T, fw *hdf5.FileWriter, name string, v craftedVar) {
	t.Helper()
	n := uint64(1)
	for _, d := range v.shape {
		n *= d
	}
	data := v.data
	if data == nil {
		data = make([]float64, n)
	}
	if uint64(len(data)) != n {
		t.Fatalf("%s: %d values for shape %v", name, len(data), v.shape)
	}
	var opts []hdf5.DatasetOption
	for k, val := range v.attrs {
		opts = append(opts, hdf5.WithAttribute(k, val))
	}
	ds, err := fw.CreateDataset("/"+name, hdf5.Float64, v.shape, opts...)
	if err != nil {
		t.Fatalf("create /%s: %v", name, err)
	}
	if err := ds.Write(data); err != nil {
		t.Fatalf("write /%s: %v", name, err)
	}
}

// firSpec is a valid FIR file with M=3, R=2, N=4 in the netCDF-4 layout.
func firSpec() craftedSpec {
	return craftedSpec{
		dataType: dataTypeFIR,
		dims:     map[string]int{dimM: 3, dimR: 2, dimE: 1, dimN: 4, dimC: 3, dimI: 1},
		vars: map[string]craftedVar{
			"Data.IR":           {shape: []uint64{3, 2, 4}},
			"Data.SamplingRate": {shape: []uint64{1}, data: []float64{48000}},
		},
	}
}

func TestOpenRejectsUnsupportedDataType(t *testing.T) {
	for _, dt := range []string{"", "Wavelet", "FIR-E", "FIRE"} {
		t.Run(dt, func(t *testing.T) {
			spec := firSpec()
			spec.dataType = dt
			f, err := Open(writeCraftedSpec(t, spec))
			if err == nil {
				f.Close()
				t.Fatalf("Open succeeded for DataType %q, want ErrUnsupportedDataType", dt)
			}
			if !errors.Is(err, ErrUnsupportedDataType) {
				t.Fatalf("Open error = %v, want ErrUnsupportedDataType", err)
			}
			if dt != "" && !strings.Contains(err.Error(), strconv.Quote(dt)) {
				t.Errorf("error %q does not name the DataType %q", err, dt)
			}
		})
	}
}

func TestOpenCraftedFIR(t *testing.T) {
	f, err := Open(writeCraftedSpec(t, firSpec()))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	if f.M != 3 || f.R != 2 || f.N != 4 {
		t.Fatalf("dims M=%d R=%d N=%d, want 3 2 4", f.M, f.R, f.N)
	}
}

func TestValidateRejectsUnsupportedDataType(t *testing.T) {
	f := minimalFIRFile()
	f.DataType = "FIR-E"
	if err := f.validate(); !errors.Is(err, ErrUnsupportedDataType) {
		t.Fatalf("Validate error = %v, want ErrUnsupportedDataType", err)
	}
}
