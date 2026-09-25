package sofa

import (
	"fmt"
	"math"
	"strings"
)

// validateValues checks the values Save writes, after validate has checked
// their shapes: every number is finite, sampling rates are positive,
// frequencies ascend from zero or above, and listener orientations are
// non-zero. An unset single ListenerView or ListenerUp is allowed; Save
// writes the conventions' default instead (see listenerOrientation).
func (f *File) validateValues() error {
	for _, s := range []struct {
		name string
		vals []float64
	}{
		{"SamplingRate", f.SamplingRate},
		{"Delay", f.Delay},
		{"Frequencies", f.Frequencies},
		{"RoomVolume", []float64{f.RoomVolume}},
		{"RoomTemperature", []float64{f.RoomTemperature}},
	} {
		if err := checkFinite(s.name, s.vals); err != nil {
			return err
		}
	}
	for _, d := range []struct {
		name string
		data [][][]float64
	}{
		{"ImpulseResponses", f.ImpulseResponses},
		{"TFReal", f.TFReal},
		{"TFImag", f.TFImag},
		{"SOSCoefficients", f.SOSCoefficients},
	} {
		if err := checkFinite3D(d.name, d.data); err != nil {
			return err
		}
	}
	for _, d := range []struct {
		name string
		data [][][][]float64
	}{
		{"TFRealE", f.TFRealE},
		{"TFImagE", f.TFImagE},
	} {
		if err := checkFinite4D(d.name, d.data); err != nil {
			return err
		}
	}
	if err := f.validatePositionValues(); err != nil {
		return err
	}

	for i, sr := range f.SamplingRate {
		if sr <= 0 {
			return fmt.Errorf("SamplingRate[%d] = %g must be > 0", i, sr)
		}
	}
	for i, fr := range f.Frequencies {
		if fr < 0 {
			return fmt.Errorf("negative frequency: Frequencies[%d] = %g", i, fr)
		}
		if i > 0 && fr <= f.Frequencies[i-1] {
			return fmt.Errorf("frequencies must be strictly increasing: Frequencies[%d] = %g follows %g",
				i, fr, f.Frequencies[i-1])
		}
	}
	return nil
}

// validatePositionValues checks that positions and orientations are finite
// and that set orientations have a direction.
func (f *File) validatePositionValues() error {
	for _, p := range []struct {
		name string
		vecs []Vector3
	}{
		{"ListenerPositions", f.ListenerPositions},
		{"ReceiverPositions", f.ReceiverPositions},
		{"SourcePositions", f.SourcePositions},
		{"EmitterPositions", f.EmitterPositions},
		{"ListenerView", []Vector3{f.ListenerView}},
		{"ListenerUp", []Vector3{f.ListenerUp}},
		{"ListenerViews", f.ListenerViews},
		{"ListenerUps", f.ListenerUps},
	} {
		if err := checkFiniteVectors(p.name, p.vecs); err != nil {
			return err
		}
	}
	for _, p := range []struct {
		name string
		rows [][]Vector3
	}{
		{"ReceiverPositionsM", f.ReceiverPositionsM},
		{"EmitterPositionsM", f.EmitterPositionsM},
	} {
		for i, row := range p.rows {
			for j, v := range row {
				if !finiteVector(v) {
					return fmt.Errorf("%s[%d][%d] = %v is not finite", p.name, i, j, v)
				}
			}
		}
	}

	spherical := f.listenerViewSpherical()
	for _, o := range []struct {
		name string
		vec  Vector3
	}{
		{"ListenerView", f.ListenerView},
		{"ListenerUp", f.ListenerUp},
	} {
		if o.vec != (Vector3{}) && zeroDirection(o.vec, spherical) {
			return fmt.Errorf("%s %v has no direction", o.name, o.vec)
		}
	}
	for _, o := range []struct {
		name string
		vecs []Vector3
	}{
		{"ListenerViews", f.ListenerViews},
		{"ListenerUps", f.ListenerUps},
	} {
		for i, v := range o.vecs {
			if zeroDirection(v, spherical) {
				return fmt.Errorf("%s[%d] %v has no direction", o.name, i, v)
			}
		}
	}
	return nil
}

// listenerViewSpherical reports whether ListenerView and ListenerUp are in
// spherical coordinates (azimuth, elevation, radius).
func (f *File) listenerViewSpherical() bool {
	return strings.EqualFold(strings.TrimSpace(f.ListenerViewType), CoordinateSpherical)
}

// zeroDirection reports whether v is a zero-length vector: all components
// zero in cartesian coordinates, a zero radius in spherical ones.
func zeroDirection(v Vector3, spherical bool) bool {
	if spherical {
		return v.Z == 0
	}
	return v == (Vector3{})
}

// listenerOrientation returns the ListenerView and ListenerUp Save writes:
// the File's, or the conventions' defaults (view along +x, up along +z) for
// an unset, zero vector.
func (f *File) listenerOrientation() (view, up Vector3) {
	view, up = f.ListenerView, f.ListenerUp
	defView, defUp := Vector3{1, 0, 0}, Vector3{0, 0, 1}
	if f.listenerViewSpherical() {
		defView, defUp = Vector3{0, 0, 1}, Vector3{0, 90, 1}
	}
	if view == (Vector3{}) {
		view = defView
	}
	if up == (Vector3{}) {
		up = defUp
	}
	return view, up
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func finiteVector(v Vector3) bool { return finite(v.X) && finite(v.Y) && finite(v.Z) }

// The checkFinite helpers format an index only for the first bad value, so
// valid data costs no allocations.

func checkFinite(name string, vals []float64) error {
	for i, v := range vals {
		if !finite(v) {
			return fmt.Errorf("%s[%d] = %g is not finite", name, i, v)
		}
	}
	return nil
}

func checkFinite3D(name string, data [][][]float64) error {
	for i, a := range data {
		for j, b := range a {
			for k, v := range b {
				if !finite(v) {
					return fmt.Errorf("%s[%d][%d][%d] = %g is not finite", name, i, j, k, v)
				}
			}
		}
	}
	return nil
}

func checkFinite4D(name string, data [][][][]float64) error {
	for i, a := range data {
		for j, b := range a {
			for k, c := range b {
				for l, v := range c {
					if !finite(v) {
						return fmt.Errorf("%s[%d][%d][%d][%d] = %g is not finite", name, i, j, k, l, v)
					}
				}
			}
		}
	}
	return nil
}

func checkFiniteVectors(name string, vecs []Vector3) error {
	for i, v := range vecs {
		if !finiteVector(v) {
			return fmt.Errorf("%s[%d] = %v is not finite", name, i, v)
		}
	}
	return nil
}
