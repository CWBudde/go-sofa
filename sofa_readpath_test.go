package sofa

import (
	"encoding/binary"
	"errors"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	hdf5 "github.com/cwbudde/go-hdf5"
)

// craftedVar is one dataset of a crafted file: its row-major values
// (zero-filled when nil), optional string attributes, and either named
// dims (the shape then follows from the dimension sizes and the dataset
// is attached to the scales, as netCDF-4 does) or a bare shape without
// dimension labels.
type craftedVar struct {
	dims  []string
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
	// extra, when set, writes further objects the craftedVar model cannot
	// express (other datatypes) before the file is closed.
	extra func(t *testing.T, fw *hdf5.FileWriter)
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
	nc := &netcdfDimensions{fw: fw, sizes: spec.dims, scales: map[string]*hdf5.DatasetWriter{}}
	for id, name := range []string{dimM, dimR, dimE, dimN, dimC, dimI} {
		size, ok := spec.dims[name]
		if !ok {
			continue
		}
		var scale *hdf5.DatasetWriter
		if name == dimN && (spec.dataType == DataTypeTF || spec.dataType == DataTypeTFE) {
			scale, err = writeFrequencyDimension(fw, make([]float64, size), id, nil)
		} else {
			scale, err = writeDimensionScale(fw, "/"+name, size, id, nil)
		}
		if err != nil {
			t.Fatalf("scale %s: %v", name, err)
		}
		nc.scales[name] = scale
	}
	for name, v := range spec.vars {
		writeCraftedVar(t, nc, name, v)
	}
	if spec.extra != nil {
		spec.extra(t, fw)
	}
	if err := fw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return path
}

func writeCraftedVar(t *testing.T, nc *netcdfDimensions, name string, v craftedVar) {
	t.Helper()
	shape := v.shape
	if v.dims != nil {
		shape = make([]uint64, len(v.dims))
		for i, d := range v.dims {
			shape[i] = uint64(nc.sizes[d]) //nolint:gosec // small test sizes
		}
	}
	n := uint64(1)
	for _, d := range shape {
		n *= d
	}
	data := v.data
	if data == nil {
		data = make([]float64, n)
	}
	if uint64(len(data)) != n {
		t.Fatalf("%s: %d values for shape %v", name, len(data), shape)
	}
	var opts []hdf5.DatasetOption
	for k, val := range v.attrs {
		opts = append(opts, hdf5.WithAttribute(k, val))
	}
	if v.dims != nil {
		if err := nc.writeVariableWithAttrs("/"+name, data, v.dims, opts); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return
	}
	ds, err := nc.fw.CreateDataset("/"+name, hdf5.Float64, shape, opts...)
	if err != nil {
		t.Fatalf("create /%s: %v", name, err)
	}
	if err := ds.Write(data); err != nil {
		t.Fatalf("write /%s: %v", name, err)
	}
}

// iota4 fills a row-major array of the given shape with values that
// encode their own index, so a transposition error cannot go unnoticed.
func iota4(shape [4]int) []float64 {
	out := make([]float64, 0, shape[0]*shape[1]*shape[2]*shape[3])
	for a := range shape[0] {
		for b := range shape[1] {
			for c := range shape[2] {
				for d := range shape[3] {
					out = append(out, float64(1000*a+100*b+10*c+d))
				}
			}
		}
	}
	return out
}

// firSpec is a valid FIR file with M=3, R=2, N=4 in the netCDF-4 layout.
func firSpec() craftedSpec {
	return craftedSpec{
		dataType: DataTypeFIR,
		dims:     map[string]int{dimM: 3, dimR: 2, dimE: 1, dimN: 4, dimC: 3, dimI: 1},
		vars: map[string]craftedVar{
			"Data.IR":           {dims: []string{dimM, dimR, dimN}},
			"Data.SamplingRate": {dims: []string{dimI}, data: []float64{48000}},
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

func TestOpenRejectsPermutedAxes(t *testing.T) {
	for _, tc := range []struct {
		name string
		v    craftedVar
	}{
		// Same element count as [M,R,N] = [3,2,4]; used to be accepted.
		{"unlabelled [N,R,M]", craftedVar{shape: []uint64{4, 2, 3}}},
		{"labelled [N,R,M]", craftedVar{dims: []string{dimN, dimR, dimM}}},
		{"flat", craftedVar{shape: []uint64{24}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec := firSpec()
			spec.vars["Data.IR"] = tc.v
			f, err := Open(writeCraftedSpec(t, spec))
			if err == nil {
				f.Close()
				t.Fatal("Open accepted a permuted Data.IR")
			}
			if !strings.Contains(err.Error(), "Data.IR") {
				t.Errorf("error %q does not name Data.IR", err)
			}
		})
	}
}

func TestOpenRejectsBadPositionShape(t *testing.T) {
	spec := firSpec()
	spec.dims[dimM] = 5
	spec.vars["Data.IR"] = craftedVar{dims: []string{dimM, dimR, dimN}}
	spec.vars["SourcePosition"] = craftedVar{shape: []uint64{3, 5}} // [C,M]
	f, err := Open(writeCraftedSpec(t, spec))
	if err == nil {
		f.Close()
		t.Fatal("Open accepted SourcePosition [C,M]")
	}
	if !strings.Contains(err.Error(), "SourcePosition") {
		t.Errorf("error %q does not name SourcePosition", err)
	}
}

// tfeSpec is a TF-E file with M=2, R=2, E=3, N=4 whose Data.Real/Imag
// are stored with the given axis order and value = index code.
func tfeSpec(e, n int, order []string, labelled bool) craftedSpec {
	sizes := map[string]int{dimM: 2, dimR: 2, dimE: e, dimN: n, dimC: 3, dimI: 1}
	var shape [4]int
	for i, d := range order {
		shape[i] = sizes[d]
	}
	v := craftedVar{data: iota4(shape)}
	if labelled {
		v.dims = order
	} else {
		v.shape = []uint64{uint64(shape[0]), uint64(shape[1]), uint64(shape[2]), uint64(shape[3])} //nolint:gosec // small
	}
	return craftedSpec{
		dataType: DataTypeTFE,
		dims:     sizes,
		vars:     map[string]craftedVar{"Data.Real": v, "Data.Imag": v},
	}
}

func TestOpenTFEAxisOrder(t *testing.T) {
	mren := []string{dimM, dimR, dimE, dimN}
	mrne := []string{dimM, dimR, dimN, dimE}
	for _, tc := range []struct {
		name     string
		e, n     int
		order    []string
		labelled bool
	}{
		{"go-sofa [M,R,E,N] unlabelled", 3, 4, mren, false},
		{"toolbox [M,R,N,E] unlabelled", 3, 4, mrne, false},
		{"go-sofa [M,R,E,N] labelled E==N", 3, 3, mren, true},
		{"toolbox [M,R,N,E] labelled E==N", 3, 3, mrne, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Open(writeCraftedSpec(t, tfeSpec(tc.e, tc.n, tc.order, tc.labelled)))
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer f.Close()
			pos := map[string]int{}
			for m := range f.M {
				for r := range f.R {
					for e := range f.E {
						for n := range f.N {
							pos[dimM], pos[dimR], pos[dimE], pos[dimN] = m, r, e, n
							want := float64(1000*pos[tc.order[0]] + 100*pos[tc.order[1]] +
								10*pos[tc.order[2]] + pos[tc.order[3]])
							if got := f.TFRealE[m][r][e][n]; got != want {
								t.Fatalf("TFRealE[%d][%d][%d][%d] = %v, want %v", m, r, e, n, got, want)
							}
							if got := f.TFImagE[m][r][e][n]; got != want {
								t.Fatalf("TFImagE[%d][%d][%d][%d] = %v, want %v", m, r, e, n, got, want)
							}
						}
					}
				}
			}
		})
	}
}

// TestOpenToolboxTFEFixture checks a SOFA Toolbox file, whose Data.Real is
// stored (M, R, N, E), against the raw dataset values.
func TestOpenToolboxTFEFixture(t *testing.T) {
	path := testdataPath(t, "FreeFieldHRTF_1.0.sofa")
	f, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	h, err := hdf5.Open(path)
	if err != nil {
		t.Fatalf("hdf5.Open: %v", err)
	}
	defer h.Close()
	var raw []float64
	for _, c := range h.Root().Children() {
		if ds, ok := c.(*hdf5.Dataset); ok && ds.Name() == "Data.Real" {
			if raw, err = ds.Read(); err != nil {
				t.Fatalf("read Data.Real: %v", err)
			}
		}
	}
	for _, idx := range [][4]int{{0, 0, 0, 0}, {0, 1, 5, 7}, {0, 1, f.E - 1, f.N - 1}, {0, 0, 1000, 64}} {
		m, r, e, n := idx[0], idx[1], idx[2], idx[3]
		want := raw[((m*f.R+r)*f.N+n)*f.E+e]
		if got := f.TFRealE[m][r][e][n]; got != want {
			t.Errorf("TFRealE[%d][%d][%d][%d] = %v, want raw (M,R,N,E) value %v", m, r, e, n, got, want)
		}
	}
}

func TestOpenPerMeasurementPositions(t *testing.T) {
	spec := firSpec()
	spec.dims[dimE] = 2
	// value(x, c, m) = 100*x + 10*c + m for receiver/emitter x.
	xcm := func(x int) []float64 {
		var out []float64
		for i := range x {
			for c := range 3 {
				for m := range 3 {
					out = append(out, float64(100*i+10*c+m))
				}
			}
		}
		return out
	}
	mc := func(base float64) []float64 {
		var out []float64
		for m := range 3 {
			for c := range 3 {
				out = append(out, base+float64(10*m+c))
			}
		}
		return out
	}
	spec.vars["ReceiverPosition"] = craftedVar{dims: []string{dimR, dimC, dimM}, data: xcm(2)}
	spec.vars["EmitterPosition"] = craftedVar{dims: []string{dimE, dimC, dimM}, data: xcm(2)}
	spec.vars["ListenerView"] = craftedVar{dims: []string{dimM, dimC}, data: mc(1000)}
	spec.vars["ListenerUp"] = craftedVar{dims: []string{dimM, dimC}, data: mc(2000)}

	f, err := Open(writeCraftedSpec(t, spec))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	for name, got := range map[string][][]Vector3{
		"ReceiverPositionsM": f.ReceiverPositionsM,
		"EmitterPositionsM":  f.EmitterPositionsM,
	} {
		if len(got) != 3 {
			t.Fatalf("len(%s) = %d, want M=3", name, len(got))
		}
		for m := range 3 {
			if len(got[m]) != 2 {
				t.Fatalf("len(%s[%d]) = %d, want 2", name, m, len(got[m]))
			}
			for x := range 2 {
				b := float64(100*x + m)
				if want := (Vector3{b, b + 10, b + 20}); got[m][x] != want {
					t.Errorf("%s[%d][%d] = %v, want %v", name, m, x, got[m][x], want)
				}
			}
		}
	}
	if !reflect.DeepEqual(f.ReceiverPositions, f.ReceiverPositionsM[0]) {
		t.Errorf("ReceiverPositions = %v, want measurement 0 %v", f.ReceiverPositions, f.ReceiverPositionsM[0])
	}
	if !reflect.DeepEqual(f.EmitterPositions, f.EmitterPositionsM[0]) {
		t.Errorf("EmitterPositions = %v, want measurement 0 %v", f.EmitterPositions, f.EmitterPositionsM[0])
	}

	for name, tc := range map[string]struct {
		all   []Vector3
		first Vector3
		base  float64
	}{
		"ListenerView": {f.ListenerViews, f.ListenerView, 1000},
		"ListenerUp":   {f.ListenerUps, f.ListenerUp, 2000},
	} {
		if len(tc.all) != 3 {
			t.Fatalf("len(%ss) = %d, want M=3", name, len(tc.all))
		}
		for m := range 3 {
			b := tc.base + float64(10*m)
			if want := (Vector3{b, b + 1, b + 2}); tc.all[m] != want {
				t.Errorf("%ss[%d] = %v, want %v", name, m, tc.all[m], want)
			}
		}
		if tc.first != tc.all[0] {
			t.Errorf("%s = %v, want first measurement %v", name, tc.first, tc.all[0])
		}
	}
}

// TestOpenSharedPositionsLeavePerMFieldsEmpty checks that the per-M fields
// stay nil for the common, measurement-independent layouts.
func TestOpenSharedPositionsLeavePerMFieldsEmpty(t *testing.T) {
	spec := firSpec()
	spec.vars["ReceiverPosition"] = craftedVar{dims: []string{dimR, dimC, dimI}}
	spec.vars["ListenerView"] = craftedVar{dims: []string{dimI, dimC}, data: []float64{1, 0, 0}}
	f, err := Open(writeCraftedSpec(t, spec))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	if f.ReceiverPositionsM != nil || f.ListenerViews != nil {
		t.Errorf("per-M fields set for shared layouts: %v, %v", f.ReceiverPositionsM, f.ListenerViews)
	}
	if len(f.ReceiverPositions) != 2 || f.ListenerView != (Vector3{1, 0, 0}) {
		t.Errorf("ReceiverPositions = %v, ListenerView = %v", f.ReceiverPositions, f.ListenerView)
	}
}

// TestOpenOfficeIIListenerViewPerMeasurement covers a third-party file
// whose ListenerView is [M,C].
func TestOpenOfficeIIListenerViewPerMeasurement(t *testing.T) {
	f, err := Open(testdataPath(t, "OfficeII.sofa"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	if len(f.ListenerViews) != f.M {
		t.Fatalf("len(ListenerViews) = %d, want M=%d", len(f.ListenerViews), f.M)
	}
	if f.ListenerView != f.ListenerViews[0] {
		t.Errorf("ListenerView = %v, want ListenerViews[0] = %v", f.ListenerView, f.ListenerViews[0])
	}
}

func TestDecodeReferenceList(t *testing.T) {
	entry := func(addr uint64, dim uint32) []byte {
		b := make([]byte, 16)
		binary.LittleEndian.PutUint64(b, addr)
		binary.LittleEndian.PutUint32(b[8:], dim)
		return b
	}
	data := append(entry(0x100, 0), entry(0x200, 2)...)
	got := decodeReferenceList(data, 16)
	want := []dimensionRef{{0x100, 0}, {0x200, 2}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("decodeReferenceList = %v, want %v", got, want)
	}
	for name, tc := range map[string]struct {
		data []byte
		size int
	}{
		"entry too small":   {data, 8},
		"ragged length":     {data[:20], 16},
		"dimension >= rank": {entry(0x100, maxRank), 16},
	} {
		if got := decodeReferenceList(tc.data, tc.size); len(got) != 0 {
			t.Errorf("%s: decodeReferenceList = %v, want none", name, got)
		}
	}
}

func TestSwapLastAxes(t *testing.T) {
	// Two blocks of [2][3] -> [3][2].
	in := []float64{0, 1, 2, 3, 4, 5, 10, 11, 12, 13, 14, 15}
	want := []float64{0, 3, 1, 4, 2, 5, 10, 13, 11, 14, 12, 15}
	if got := swapLastAxes(in, 2, 2, 3); !reflect.DeepEqual(got, want) {
		t.Errorf("swapLastAxes = %v, want %v", got, want)
	}
}
