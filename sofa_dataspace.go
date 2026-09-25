package sofa

import (
	"regexp"
	"strconv"
	"strings"

	hdf5 "github.com/cwbudde/go-hdf5"
)

// dataspaceArrayRE matches the dataspace part of go-hdf5's Dataset.Info
// output: "1D array [5]", "2D array [3 x 4]" or "3D array [2 3 4]".
var dataspaceArrayRE = regexp.MustCompile(`\b\d+D array \[([0-9 x]*)\]`)

// datasetElementCount returns the number of elements in ds from its
// dataspace, without reading the data. ok is false when the shape cannot be
// determined; callers then fall back to reading the dataset.
func datasetElementCount(ds *hdf5.Dataset) (n uint64, ok bool) {
	info, err := ds.Info()
	if err != nil {
		return 0, false
	}
	return parseDataspaceElements(info)
}

// datasetShape returns the dimensions of ds from its dataspace, without
// reading the data; a scalar dataspace yields an empty shape. ok is false
// when the shape cannot be determined.
func datasetShape(ds *hdf5.Dataset) (shape []uint64, ok bool) {
	info, err := ds.Info()
	if err != nil {
		return nil, false
	}
	return parseDataspaceShape(info)
}

// parseDataspaceShape extracts the dimensions from a Dataset.Info string.
func parseDataspaceShape(info string) (shape []uint64, ok bool) {
	m := dataspaceArrayRE.FindStringSubmatch(info)
	if m == nil {
		if strings.Contains(info, ", scalar,") {
			return []uint64{}, true
		}
		return nil, false
	}
	for _, field := range strings.FieldsFunc(m[1], func(r rune) bool { return r == ' ' || r == 'x' }) {
		d, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return nil, false
		}
		shape = append(shape, d)
	}
	return shape, true
}

// parseDataspaceElements extracts the element count from a Dataset.Info
// string. Counts above maxDataElements are clamped to maxDataElements+1, so
// the result cannot overflow and still compares as too large.
func parseDataspaceElements(info string) (n uint64, ok bool) {
	shape, ok := parseDataspaceShape(info)
	if !ok {
		return 0, false
	}
	const limit = uint64(maxDataElements) + 1
	n = 1
	for _, d := range shape {
		if d != 0 && n > limit/d {
			return limit, true
		}
		n = min(n*d, limit)
	}
	return n, true
}
