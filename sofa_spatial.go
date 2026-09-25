package sofa

import (
	"fmt"
	"strings"

	hdf5 "github.com/cwbudde/go-hdf5"
)

// readSpatialData reads listener, receiver, source, and emitter positions
// and the listener orientation. Each dataset's shape must match one of the
// layouts AES69 allows; one that does not is an error. Reads themselves
// stay best-effort: a dataset go-hdf5 cannot read is skipped.
func (f *File) readSpatialData(datasets map[string]*hdf5.Dataset, labels map[string][]string) error {
	// Position datasets, each carrying Type and Units attributes that name
	// its coordinate system. perM is set for [X,C,M] layouts.
	type posTarget struct {
		name    string
		dst     *[]Vector3
		perM    *[][]Vector3
		typ     *string
		units   *string
		layouts [][]string
	}
	perObject := func(x string) [][]string {
		return [][]string{
			{x, dimC}, {dimI, dimC}, {x, dimC, dimI}, {dimI, dimC, dimI}, {x, dimC, dimM}, {dimI, dimC, dimM},
		}
	}
	perMeasurement := [][]string{{dimI, dimC}, {dimM, dimC}}
	for _, pt := range []posTarget{
		{"ListenerPosition", &f.ListenerPositions, nil, &f.ListenerPositionType, &f.ListenerPositionUnits, perMeasurement},
		{"ReceiverPosition", &f.ReceiverPositions, &f.ReceiverPositionsM, &f.ReceiverPositionType, &f.ReceiverPositionUnits, perObject(dimR)},
		{datasetSourcePosition, &f.SourcePositions, nil, &f.SourcePositionType, &f.SourcePositionUnits, perMeasurement},
		{"EmitterPosition", &f.EmitterPositions, &f.EmitterPositionsM, &f.EmitterPositionType, &f.EmitterPositionUnits, perObject(dimE)},
	} {
		ds, ok := datasets[pt.name]
		if !ok {
			continue
		}
		layout, err := f.resolveLayout(pt.name, ds, labels[pt.name], pt.layouts...)
		if err != nil {
			return err
		}
		*pt.typ = readStringAttribute(ds, "Type")
		*pt.units = readStringAttribute(ds, "Units")
		if len(layout) == 3 && layout[2] == dimM {
			if perM, err := readVector3sPerMeasurement(ds, f.M); err == nil {
				*pt.perM = perM
				*pt.dst = perM[0]
			}
			continue
		}
		if vecs, err := readVector3s(ds); err == nil {
			*pt.dst = vecs
		}
	}

	// Orientation datasets: [I,C], or [M,C] kept in full in the plural field.
	type orientTarget struct {
		name string
		dst  *Vector3
		all  *[]Vector3
	}
	for _, ot := range []orientTarget{
		{"ListenerUp", &f.ListenerUp, &f.ListenerUps},
		{"ListenerView", &f.ListenerView, &f.ListenerViews},
	} {
		ds, ok := datasets[ot.name]
		if !ok {
			continue
		}
		layout, err := f.resolveLayout(ot.name, ds, labels[ot.name], perMeasurement...)
		if err != nil {
			return err
		}
		vecs, err := readVector3s(ds)
		if err != nil || len(vecs) == 0 {
			continue
		}
		*ot.dst = vecs[0]
		if layout[0] == dimM {
			*ot.all = vecs
		}
	}
	return nil
}

// readVector3sPerMeasurement reads an [X,C,M] dataset as [M][X] vectors.
func readVector3sPerMeasurement(ds *hdf5.Dataset, m int) ([][]Vector3, error) {
	data, err := ds.Read()
	if err != nil {
		return nil, err
	}
	if m <= 0 || len(data)%(3*m) != 0 {
		return nil, fmt.Errorf("data length %d not divisible by 3·M=%d", len(data), 3*m)
	}
	x := len(data) / (3 * m)
	out := make([][]Vector3, m)
	for i := range m {
		out[i] = make([]Vector3, x)
		for j := range x {
			at := func(c int) float64 { return data[(j*3+c)*m+i] }
			out[i][j] = Vector3{at(0), at(1), at(2)}
		}
	}
	return out, nil
}

// readStringAttribute returns a dataset attribute as a lowercased, trimmed
// string, or "" when the attribute is absent or unreadable.
func readStringAttribute(ds *hdf5.Dataset, name string) string {
	val, err := ds.ReadAttribute(name)
	if err != nil || val == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", val)))
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
