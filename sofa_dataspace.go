package sofa

import (
	hdf5 "github.com/cwbudde/go-hdf5"
)

// datasetElementCount returns the number of elements in ds from its
// dataspace, without reading the data. ok is false when the shape cannot be
// determined; callers then fall back to reading the dataset. Counts above
// maxDataElements are clamped to maxDataElements+1, so the result cannot
// overflow and still compares as too large.
func datasetElementCount(ds *hdf5.Dataset) (n uint64, ok bool) {
	shape, ok := datasetShape(ds)
	if !ok {
		return 0, false
	}
	return elementCount(shape), true
}

// elementCount returns the product of shape, clamped to maxDataElements+1.
func elementCount(shape []uint64) uint64 {
	const limit = uint64(maxDataElements) + 1
	n := uint64(1)
	for _, d := range shape {
		if d != 0 && n > limit/d {
			return limit
		}
		n = min(n*d, limit)
	}
	return n
}

// datasetShape returns the dimensions of ds from its dataspace, without
// reading the data; a scalar dataspace yields an empty shape. ok is false
// when the shape cannot be determined or the dataspace is null.
func datasetShape(ds *hdf5.Dataset) (shape []uint64, ok bool) {
	shape, err := ds.Shape()
	if err != nil || shape == nil {
		return nil, false
	}
	return shape, true
}
