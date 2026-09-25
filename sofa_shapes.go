package sofa

import (
	"encoding/binary"
	"fmt"
	"slices"
	"strings"

	hdf5 "github.com/cwbudde/go-hdf5"
)

// maxRank is HDF5's limit on the number of dataspace dimensions; it bounds
// the dimension index a crafted REFERENCE_LIST entry may claim.
const maxRank = 32

// dimensionLabels maps each variable to the names of the dimensions attached
// to its axes, reconstructed from the REFERENCE_LIST attribute that netCDF-4
// writes on every dimension scale. Variables with any unlabelled axis are
// left out, so their layout is inferred from sizes alone.
func dimensionLabels(datasets map[string]*hdf5.Dataset) map[string][]string {
	byAddr := make(map[uint64]string, len(datasets))
	for name, ds := range datasets {
		byAddr[ds.Address()] = name
	}
	labels := map[string][]string{}
	for _, scale := range []string{dimM, dimR, dimE, dimN, dimC, dimI} {
		ds, ok := datasets[scale]
		if !ok {
			continue
		}
		attrs, err := ds.Attributes()
		if err != nil {
			continue
		}
		for _, a := range attrs {
			if a.Name != "REFERENCE_LIST" || a.Datatype == nil {
				continue
			}
			for _, ref := range decodeReferenceList(a.Data, int(a.Datatype.Size)) {
				name, ok := byAddr[ref.addr]
				if !ok {
					continue
				}
				l := labels[name]
				for len(l) <= ref.dim {
					l = append(l, "")
				}
				l[ref.dim] = scale
				labels[name] = l
			}
		}
	}
	for name, l := range labels {
		if slices.Contains(l, "") {
			delete(labels, name)
		}
	}
	return labels
}

type dimensionRef struct {
	addr uint64 // object header address of the attached variable
	dim  int    // axis of that variable the scale is attached to
}

// decodeReferenceList decodes the entries of an H5DS REFERENCE_LIST
// attribute: a compound of an object reference (offset 0) and the
// dimension index as a 32-bit integer (offset 8), entrySize bytes each.
// Malformed data yields no entries rather than an error: the labels only
// disambiguate layouts that sizes cannot.
func decodeReferenceList(data []byte, entrySize int) []dimensionRef {
	if entrySize < 12 || len(data)%entrySize != 0 {
		return nil
	}
	refs := make([]dimensionRef, 0, len(data)/entrySize)
	for off := 0; off < len(data); off += entrySize {
		dim := binary.LittleEndian.Uint32(data[off+8:])
		if dim >= maxRank {
			continue
		}
		refs = append(refs, dimensionRef{addr: binary.LittleEndian.Uint64(data[off:]), dim: int(dim)})
	}
	return refs
}

// axisSize returns the size of a SOFA dimension.
func (f *File) axisSize(d string) uint64 {
	switch d {
	case dimM:
		return uint64(f.M) //nolint:gosec // validated positive by readDimensions
	case dimR:
		return uint64(f.R) //nolint:gosec // validated positive by readDimensions
	case dimE:
		return uint64(f.E) //nolint:gosec // validated positive by readDimensions
	case dimN:
		return uint64(f.N) //nolint:gosec // validated positive by readDimensions
	case dimC:
		return 3
	case dimI:
		return 1
	}
	return 0
}

// resolveLayout returns which of the allowed layouts (dimension names per
// axis) variable name is stored in. When the file labels the variable's
// axes, the labels must equal one of the layouts, which tells apart layouts
// whose sizes coincide (E == N, M == R). Otherwise the first layout whose
// sizes match the dataspace wins, so layouts are listed in order of
// preference. Any other shape is an error rather than a silent misread.
func (f *File) resolveLayout(name string, ds *hdf5.Dataset, labels []string, layouts ...[]string) ([]string, error) {
	shape, ok := datasetShape(ds)
	if !ok {
		return nil, fmt.Errorf("%s: cannot determine dataset shape", name)
	}
	if len(shape) == 0 {
		shape = []uint64{1}
	}
	for _, l := range layouts {
		if f.sizesMatch(l, shape) && (labels == nil || slices.Equal(labels, l)) {
			return l, nil
		}
	}
	got := fmt.Sprint(shape)
	if labels != nil {
		got += " (" + strings.Join(labels, ",") + ")"
	}
	want := make([]string, len(layouts))
	for i, l := range layouts {
		want[i] = "[" + strings.Join(l, ",") + "]"
	}
	return nil, fmt.Errorf("%s: shape %s does not match %s with M=%d R=%d E=%d N=%d",
		name, got, strings.Join(want, " or "), f.M, f.R, f.E, f.N)
}

func (f *File) sizesMatch(layout []string, shape []uint64) bool {
	if len(layout) != len(shape) {
		return false
	}
	for i, d := range layout {
		if f.axisSize(d) != shape[i] {
			return false
		}
	}
	return true
}

// swapLastAxes turns a row-major [outer][a][b] buffer into [outer][b][a].
func swapLastAxes(flat []float64, outer, a, b int) []float64 {
	out := make([]float64, len(flat))
	for o := range outer {
		base := o * a * b
		for i := range a {
			for j := range b {
				out[base+j*a+i] = flat[base+i*b+j]
			}
		}
	}
	return out
}
