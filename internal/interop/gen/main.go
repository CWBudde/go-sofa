// Command gen writes one SOFA file per supported DataType (FIR, TF, TF-E,
// SOS) with sofa.Save, filled with known values, plus an expected.json
// sidecar describing what a reference reader must see. It exists only to
// drive the interoperability check in scripts/interop_check.py (h5py and
// netCDF4); it is internal and not a supported tool.
//
// Usage: go run ./internal/interop/gen <outdir>
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	sofa "github.com/cwbudde/go-sofa"
)

// dataset is the expected content of one HDF5 dataset / netCDF variable.
type dataset struct {
	Shape  []int     `json:"shape"`
	Values []float64 `json:"values"` // row-major (C order)
}

// expectation is what a reference reader must find in one written file.
type expectation struct {
	Attributes map[string]string  `json:"attributes"`
	Datasets   map[string]dataset `json:"datasets"`
}

const (
	numM = 3
	numR = 2
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gen <outdir>")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}

func run(dir string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil { //nolint:gosec // output dir from CLI arg (dev tool)
		return err
	}

	builders := map[string]func() (*sofa.File, map[string]dataset){
		"fir.sofa": buildFIR,
		"tf.sofa":  buildTF,
		"tfe.sofa": buildTFE,
		"sos.sofa": buildSOS,
	}

	expected := make(map[string]expectation, len(builders))
	for name, build := range builders {
		f, data := build()
		if err := f.Save(filepath.Join(dir, name)); err != nil {
			return fmt.Errorf("save %s: %w", name, err)
		}
		for k, v := range positionDatasets(f) {
			data[k] = v
		}
		expected[name] = expectation{
			Attributes: map[string]string{
				"Conventions":     f.Conventions,
				"Version":         f.Version,
				"SOFAConventions": f.SOFAConventions,
				"DataType":        f.DataType,
				"Title":           f.Title,
			},
			Datasets: data,
		}
		fmt.Println("wrote", filepath.Join(dir, name))
	}

	out, err := json.MarshalIndent(expected, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "expected.json"), append(out, '\n'), 0o600) //nolint:gosec // output dir from CLI arg (dev tool)
}

// base returns a file with metadata and positions common to all DataTypes.
func base(conv, dataType string, e, n int) *sofa.File {
	f := &sofa.File{
		M: numM, R: numR, E: e, N: n,
		Conventions:            "SOFA",
		Version:                "2.1",
		SOFAConventions:        conv,
		SOFAConventionsVersion: "1.0",
		DataType:               dataType,
		RoomType:               "free field",
		Title:                  "go-sofa interop " + dataType,
		APIName:                "go-sofa",
		APIVersion:             "interop",
		License:                "No license provided, ask the author for permission",

		ListenerPositionType:  sofa.CoordinateCartesian,
		ListenerPositionUnits: sofa.UnitsCartesianMetres,
		ReceiverPositionType:  sofa.CoordinateCartesian,
		ReceiverPositionUnits: sofa.UnitsCartesianMetres,
		SourcePositionType:    sofa.CoordinateSpherical,
		SourcePositionUnits:   sofa.UnitsSphericalDegrees,
		EmitterPositionType:   sofa.CoordinateCartesian,
		EmitterPositionUnits:  sofa.UnitsCartesianMetres,

		ListenerUp:   sofa.Vector3{X: 0, Y: 0, Z: 1},
		ListenerView: sofa.Vector3{X: 1, Y: 0, Z: 0},
	}
	f.ListenerPositions = []sofa.Vector3{{X: 0, Y: 0, Z: 0}}
	f.ReceiverPositions = []sofa.Vector3{{X: 0, Y: 0.09, Z: 0}, {X: 0, Y: -0.09, Z: 0}}
	for m := range numM {
		f.SourcePositions = append(f.SourcePositions,
			sofa.Vector3{X: float64(30 * m), Y: float64(-10 + 10*m), Z: 1.5})
	}
	for i := range e {
		f.EmitterPositions = append(f.EmitterPositions, sofa.Vector3{X: 0.01 * float64(i), Y: 0, Z: 0})
	}
	return f
}

// value is a deterministic, distinct, exactly-representable-in-JSON sample.
func value(scale float64, idx ...int) float64 {
	v := scale
	w := 1.0
	for _, i := range idx {
		v += float64(i+1) * w
		w /= 16
	}
	return v
}

func grid3(scale float64, n int) ([][][]float64, []float64) {
	m, r := numM, numR
	out := make([][][]float64, m)
	flat := make([]float64, 0, m*r*n)
	for i := range m {
		out[i] = make([][]float64, r)
		for j := range r {
			out[i][j] = make([]float64, n)
			for k := range n {
				out[i][j][k] = value(scale, i, j, k)
				flat = append(flat, out[i][j][k])
			}
		}
	}
	return out, flat
}

func grid4(scale float64, e, n int) ([][][][]float64, []float64) {
	m, r := numM, numR
	out := make([][][][]float64, m)
	flat := make([]float64, 0, m*r*e*n)
	for i := range m {
		out[i] = make([][][]float64, r)
		for j := range r {
			out[i][j] = make([][]float64, e)
			for l := range e {
				out[i][j][l] = make([]float64, n)
				for k := range n {
					out[i][j][l][k] = value(scale, i, j, l, k)
					flat = append(flat, out[i][j][l][k])
				}
			}
		}
	}
	return out, flat
}

func frequencies(n int) []float64 {
	fs := make([]float64, n)
	for i := range fs {
		fs[i] = 1000 * float64(i)
	}
	return fs
}

func buildFIR() (*sofa.File, map[string]dataset) {
	const n = 8
	f := base("SimpleFreeFieldHRIR", "FIR", 1, n)
	ir, flat := grid3(0, n)
	f.ImpulseResponses = ir
	f.SamplingRate = []float64{48000}
	f.Delay = []float64{0, 0}
	return f, map[string]dataset{
		"Data.IR":           {Shape: []int{numM, numR, n}, Values: flat},
		"Data.SamplingRate": {Shape: []int{1}, Values: f.SamplingRate},
	}
}

func buildTF() (*sofa.File, map[string]dataset) {
	const n = 5
	f := base("SimpleFreeFieldHRTF", "TF", 1, n)
	re, reFlat := grid3(0, n)
	im, imFlat := grid3(-100, n)
	f.TFReal, f.TFImag = re, im
	f.Frequencies = frequencies(n)
	return f, map[string]dataset{
		"Data.Real": {Shape: []int{numM, numR, n}, Values: reFlat},
		"Data.Imag": {Shape: []int{numM, numR, n}, Values: imFlat},
		"N":         {Shape: []int{n}, Values: f.Frequencies},
	}
}

func buildTFE() (*sofa.File, map[string]dataset) {
	const e, n = 2, 4
	f := base("GeneralTF-E", "TF-E", e, n)
	re, reFlat := grid4(0, e, n)
	im, imFlat := grid4(-100, e, n)
	f.TFRealE, f.TFImagE = re, im
	f.Frequencies = frequencies(n)
	return f, map[string]dataset{
		"Data.Real": {Shape: []int{numM, numR, e, n}, Values: reFlat},
		"Data.Imag": {Shape: []int{numM, numR, e, n}, Values: imFlat},
		"N":         {Shape: []int{n}, Values: f.Frequencies},
	}
}

func buildSOS() (*sofa.File, map[string]dataset) {
	const n = 12 // two biquads
	f := base("SimpleFreeFieldHRSOS", "SOS", 1, n)
	sos, flat := grid3(0, n)
	f.SOSCoefficients = sos
	f.SamplingRate = []float64{44100}
	return f, map[string]dataset{
		"Data.SOS":          {Shape: []int{numM, numR, n}, Values: flat},
		"Data.SamplingRate": {Shape: []int{1}, Values: f.SamplingRate},
	}
}

func positionDatasets(f *sofa.File) map[string]dataset {
	flat := func(vs []sofa.Vector3) dataset {
		d := dataset{Shape: []int{len(vs), 3}}
		for _, v := range vs {
			d.Values = append(d.Values, v.X, v.Y, v.Z)
		}
		return d
	}
	return map[string]dataset{
		"ListenerPosition": flat(f.ListenerPositions),
		"ReceiverPosition": flat(f.ReceiverPositions),
		"SourcePosition":   flat(f.SourcePositions),
		"EmitterPosition":  flat(f.EmitterPositions),
		"ListenerUp":       flat([]sofa.Vector3{f.ListenerUp}),
		"ListenerView":     flat([]sofa.Vector3{f.ListenerView}),
	}
}
