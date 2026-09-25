// Package clitest builds small, valid SOFA files for the command tests in
// cmd/. It is internal and not a supported API.
package clitest

import (
	"path/filepath"
	"testing"

	sofa "github.com/cwbudde/go-sofa"
)

// Dimensions of every file Build returns (E is 2 for TF-E).
const (
	M = 3
	R = 2
)

// Build returns an in-memory file of the given DataType (sofa.DataTypeFIR,
// DataTypeTF, DataTypeTFE or DataTypeSOS) with M measurements, R receivers
// and deterministic data: sample k of measurement m, receiver r (and
// emitter e) is 100m + 10r + k (+ 1000e). FIR files carry a per-receiver
// Delay of {0, 2.5}.
func Build(dataType string) *sofa.File {
	switch dataType {
	case sofa.DataTypeTF:
		f := base("SimpleFreeFieldHRTF", dataType, 1, 5)
		f.TFReal, f.TFImag = grid(f.N, 1), grid(f.N, -1)
		f.Frequencies = frequencies(f.N)
		return f
	case sofa.DataTypeTFE:
		f := base("GeneralTF-E", dataType, 2, 4)
		f.TFRealE, f.TFImagE = gridE(f.E, f.N, 1), gridE(f.E, f.N, -1)
		f.Frequencies = frequencies(f.N)
		return f
	case sofa.DataTypeSOS:
		f := base("SimpleFreeFieldHRSOS", dataType, 1, 12)
		f.SOSCoefficients = grid(f.N, 1)
		f.SamplingRate = []float64{44100}
		return f
	default:
		f := base("SimpleFreeFieldHRIR", sofa.DataTypeFIR, 1, 8)
		f.ImpulseResponses = grid(f.N, 1)
		f.SamplingRate = []float64{48000}
		f.Delay = []float64{0, 2.5}
		return f
	}
}

// Write saves Build(dataType) as dir/name and returns its path.
func Write(t testing.TB, dir, name, dataType string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := Build(dataType).Save(path); err != nil {
		t.Fatalf("save %s: %v", path, err)
	}
	return path
}

func base(conv, dataType string, e, n int) *sofa.File {
	f := &sofa.File{
		M: M, R: R, E: e, N: n,
		Conventions:            "SOFA",
		Version:                "2.1",
		SOFAConventions:        conv,
		SOFAConventionsVersion: "1.0",
		DataType:               dataType,
		RoomType:               "free field",
		Title:                  "go-sofa clitest " + dataType,
		APIName:                "go-sofa",
		APIVersion:             "clitest",
		License:                "No license provided, ask the author for permission",
		DateCreated:            "2026-01-01 00:00:00",
		DateModified:           "2026-01-01 00:00:00",

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
	f.ListenerPositions = []sofa.Vector3{{}}
	f.ReceiverPositions = []sofa.Vector3{{Y: 0.09}, {Y: -0.09}}
	for m := range M {
		f.SourcePositions = append(f.SourcePositions, sofa.Vector3{X: float64(30 * m), Z: 1.5})
	}
	for i := range e {
		f.EmitterPositions = append(f.EmitterPositions, sofa.Vector3{X: 0.01 * float64(i)})
	}
	return f
}

func grid(n int, sign float64) [][][]float64 {
	out := make([][][]float64, M)
	for m := range out {
		out[m] = make([][]float64, R)
		for r := range out[m] {
			out[m][r] = make([]float64, n)
			for k := range n {
				out[m][r][k] = sign * float64(100*m+10*r+k)
			}
		}
	}
	return out
}

func gridE(e, n int, sign float64) [][][][]float64 {
	out := make([][][][]float64, M)
	for m := range out {
		out[m] = make([][][]float64, R)
		for r := range out[m] {
			out[m][r] = make([][]float64, e)
			for l := range e {
				out[m][r][l] = make([]float64, n)
				for k := range n {
					out[m][r][l][k] = sign * float64(1000*l+100*m+10*r+k)
				}
			}
		}
	}
	return out
}

func frequencies(n int) []float64 {
	out := make([]float64, n)
	for k := range out {
		out[k] = 1000 * float64(k)
	}
	return out
}
