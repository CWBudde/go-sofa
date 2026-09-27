package sofa

import (
	"fmt"
	"slices"

	hdf5 "github.com/cwbudde/go-hdf5"
)

// readBudget returns how many elements Open reads at most, across all
// variables, from a file of size bytes. It is a variable so that tests can
// lower it.
var readBudget = defaultReadBudget

// defaultReadBudget allows 64 Mi elements (512 MiB as float64), or eight
// elements per byte of file when that is more: compressed audio data
// rarely shrinks below a byte per sample, but a crafted file declares
// gigabytes in a few bytes (a chunked variable nobody wrote reads as
// zeros).
func defaultReadBudget(size int64) uint64 {
	return max(1<<26, 8*uint64(max(size, 0)))
}

// checkReadBudget fails with ErrTooLarge when the datasets, except those
// named in skip, declare more elements together than readBudget allows for
// a file of size bytes. It reads no data, so a file it rejects allocates
// nothing. A dataset whose shape is unknown is not counted; reading it is
// bounded by go-hdf5.
func checkReadBudget(datasets map[string]*hdf5.Dataset, skip []string, size int64) error {
	var total uint64 // each count is at most maxDataElements+1: no overflow
	for name, ds := range datasets {
		if slices.Contains(skip, name) {
			continue
		}
		if n, ok := datasetElementCount(ds); ok {
			total += n
		}
	}
	if budget := readBudget(size); total > budget {
		return fmt.Errorf("%w: variables declare %d elements, more than the %d read from a %d-byte file",
			ErrTooLarge, total, budget, size)
	}
	return nil
}

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
