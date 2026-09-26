// Command toolbox is the Go side of the SOFA Toolbox cross-validation
// (scripts/matlab/roundtrip.m, README "Cross-validation"). It is internal
// and not a supported tool.
//
//	toolbox write FILE          write a SimpleFreeFieldHRIR file with go-sofa
//	toolbox compare A B         Data.IR, SourcePosition and ListenerPosition
//	                            of A and B must be bit-identical
//	toolbox check-created FILE  FILE (written by the toolbox) must hold the
//	                            values of roundtrip.m's toolboxValues
//
// See README.md, "Cross-validation", for the full command sequence.
package main

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"os"

	sofa "github.com/CWBudde/go-sofa"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "toolbox:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	switch {
	case len(args) == 2 && args[0] == "write":
		return write(args[1])
	case len(args) == 3 && args[0] == "compare":
		return compare(args[1], args[2])
	case len(args) == 2 && args[0] == "check-created":
		return checkCreated(args[1])
	}
	return errors.New("usage: toolbox write FILE | compare A B | check-created FILE")
}

// write saves a SimpleFreeFieldHRIR file whose samples and positions are
// seeded pseudo-random float64 values, using the full mantissa.
func write(path string) error {
	const m, r, n = 6, 2, 32
	rng := rand.New(rand.NewPCG(2026, 926)) //nolint:gosec // deterministic test data
	f := &sofa.File{
		M: m, R: r, E: 1, N: n,
		Conventions:            "SOFA",
		Version:                "2.1",
		SOFAConventions:        "SimpleFreeFieldHRIR",
		SOFAConventionsVersion: "1.0",
		DataType:               sofa.DataTypeFIR,
		RoomType:               "free field",
		Title:                  "go-sofa SOFA Toolbox cross-validation",
		AuthorContact:          "go-sofa maintainers",
		Organization:           "go-sofa",
		License:                "MIT",
		DateCreated:            "2026-09-26 12:00:00",
		DateModified:           "2026-09-26 12:00:00",
		SamplingRate:           []float64{48000},
		Delay:                  []float64{0, 0},
		ListenerPositionType:   sofa.CoordinateCartesian,
		ListenerPositionUnits:  sofa.UnitsCartesianMetres,
		ReceiverPositionType:   sofa.CoordinateCartesian,
		ReceiverPositionUnits:  sofa.UnitsCartesianMetres,
		SourcePositionType:     sofa.CoordinateSpherical,
		SourcePositionUnits:    sofa.UnitsSphericalDegrees,
		EmitterPositionType:    sofa.CoordinateCartesian,
		EmitterPositionUnits:   sofa.UnitsCartesianMetres,
		ReceiverPositions:      []sofa.Vector3{{Y: 0.09}, {Y: -0.09}},
		EmitterPositions:       []sofa.Vector3{{}},
		ListenerView:           sofa.Vector3{X: 1},
		ListenerUp:             sofa.Vector3{Z: 1},
		// Mandatory in the toolbox's SimpleFreeFieldHRIR; SOFAload warns
		// and fills them in when they are missing.
		Attributes: []sofa.Attribute{
			{Name: "DatabaseName", Value: "go-sofa"},
			{Name: "ListenerShortName", Value: "synthetic"},
		},
	}
	f.ImpulseResponses = make([][][]float64, m)
	for i := range m {
		f.ImpulseResponses[i] = make([][]float64, r)
		for j := range r {
			f.ImpulseResponses[i][j] = make([]float64, n)
			for k := range n {
				f.ImpulseResponses[i][j][k] = rng.NormFloat64() * math.Exp(-float64(k)/8)
			}
		}
		f.SourcePositions = append(f.SourcePositions,
			sofa.Vector3{X: rng.Float64() * 360, Y: rng.Float64()*180 - 90, Z: 1 + rng.Float64()})
		f.ListenerPositions = append(f.ListenerPositions,
			sofa.Vector3{X: rng.Float64(), Y: rng.Float64(), Z: 1 + rng.Float64()})
	}
	if err := f.Save(path); err != nil {
		return err
	}
	fmt.Printf("wrote %s: SimpleFreeFieldHRIR M=%d R=%d N=%d\n", path, m, r, n)
	return nil
}

// fields are the values the cross-validation requires to survive bit-exactly.
type fields struct {
	ir       [][][]float64
	src, lst []sofa.Vector3
}

func load(path string) (*sofa.File, fields, error) {
	f, err := sofa.Open(path)
	if err != nil {
		return nil, fields{}, err
	}
	if len(f.ListenerPositions) == 0 {
		return nil, fields{}, fmt.Errorf("%s: no ListenerPosition", path)
	}
	// Broadcast [I,C] positions to M rows so both layouts compare equal.
	src := make([]sofa.Vector3, f.M)
	lst := make([]sofa.Vector3, f.M)
	for m := range f.M {
		if src[m], err = f.SourcePositionAt(m); err != nil {
			return nil, fields{}, err
		}
		lst[m] = f.ListenerPositions[0]
		if len(f.ListenerPositions) == f.M {
			lst[m] = f.ListenerPositions[m]
		}
	}
	return f, fields{f.ImpulseResponses, src, lst}, nil
}

func compare(a, b string) error {
	fa, want, err := load(a)
	if err != nil {
		return err
	}
	fb, got, err := load(b)
	if err != nil {
		return err
	}
	if fa.M != fb.M || fa.R != fb.R || fa.N != fb.N {
		return fmt.Errorf("dimensions differ: M,R,N = %d,%d,%d vs %d,%d,%d", fa.M, fa.R, fa.N, fb.M, fb.R, fb.N)
	}
	if err := sameBits(want, got); err != nil {
		return fmt.Errorf("%s vs %s: %w", a, b, err)
	}
	fmt.Printf("bit-exact: Data.IR (%d values), SourcePosition, ListenerPosition of %s and %s (%s %s)\n",
		fa.M*fa.R*fa.N, a, b, fb.APIName, fb.APIVersion)
	return nil
}

// toolboxValues mirrors toolboxValues in scripts/matlab/roundtrip.m,
// operation for operation, so that IEEE 754 rounding is identical.
func toolboxValues(m, r, n int) fields {
	var v fields
	v.ir = make([][][]float64, m)
	for i := range m {
		v.ir[i] = make([][]float64, r)
		for j := range r {
			v.ir[i][j] = make([]float64, n)
			for k := range n {
				v.ir[i][j][k] = float64(i*100+j*10+k)/3 - math.Sqrt(float64(k+1))/7
			}
		}
		v.src = append(v.src, sofa.Vector3{X: float64(i*72) / 7, Y: 10.0/3 - float64(i), Z: 1.2 + float64(i)/10})
		v.lst = append(v.lst, sofa.Vector3{X: 0.1, Y: 0.2 / 3, Z: math.Sqrt(2)})
	}
	return v
}

func checkCreated(path string) error {
	f, got, err := load(path)
	if err != nil {
		return err
	}
	if f.SOFAConventions != "SimpleFreeFieldHRIR" || f.M != 5 || f.R != 2 || f.N != 16 {
		return fmt.Errorf("%s: %s M=%d R=%d N=%d, want SimpleFreeFieldHRIR M=5 R=2 N=16",
			path, f.SOFAConventions, f.M, f.R, f.N)
	}
	if err := sameBits(toolboxValues(f.M, f.R, f.N), got); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	fmt.Printf("bit-exact: %s (%s %s) holds the values roundtrip.m computed\n", path, f.APIName, f.APIVersion)
	return nil
}

func sameBits(want, got fields) error {
	for i := range want.ir {
		for j := range want.ir[i] {
			for k, w := range want.ir[i][j] {
				if g := got.ir[i][j][k]; math.Float64bits(g) != math.Float64bits(w) {
					return fmt.Errorf("Data.IR[%d][%d][%d] = %v, want %v", i, j, k, g, w)
				}
			}
		}
	}
	for _, p := range []struct {
		name      string
		want, got []sofa.Vector3
	}{{"SourcePosition", want.src, got.src}, {"ListenerPosition", want.lst, got.lst}} {
		for m := range p.want {
			w, g := p.want[m], p.got[m]
			for c, pair := range [][2]float64{{w.X, g.X}, {w.Y, g.Y}, {w.Z, g.Z}} {
				if math.Float64bits(pair[0]) != math.Float64bits(pair[1]) {
					return fmt.Errorf("%s[%d][%d] = %v, want %v", p.name, m, c, pair[1], pair[0])
				}
			}
		}
	}
	return nil
}
