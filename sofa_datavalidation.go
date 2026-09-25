package sofa

import (
	"math"
	"strings"
)

// validateValues checks the values Save writes, after validate has checked
// their shapes: every number is finite, sampling rates are positive,
// frequencies ascend from zero or above, and listener orientations are
// non-zero. Only the active DataType's data is checked, since Save writes
// no other. An unset single ListenerView or ListenerUp is allowed; Save
// writes the conventions' default instead (see listenerOrientation).
func (f *File) validateValues() error {
	if err := checkFinite("RoomVolume", []float64{f.RoomVolume}); err != nil {
		return err
	}
	if err := checkFinite("RoomTemperature", []float64{f.RoomTemperature}); err != nil {
		return err
	}
	if err := f.validatePositionValues(); err != nil {
		return err
	}

	switch f.DataType {
	case DataTypeFIR:
		if err := checkFinite3D("ImpulseResponses", f.ImpulseResponses); err != nil {
			return err
		}
		return f.validateRateAndDelay()
	case DataTypeSOS:
		if err := checkFinite3D("SOSCoefficients", f.SOSCoefficients); err != nil {
			return err
		}
		return f.validateRateAndDelay()
	case DataTypeTF:
		if err := checkFinite3D("TFReal", f.TFReal); err != nil {
			return err
		}
		if err := checkFinite3D("TFImag", f.TFImag); err != nil {
			return err
		}
		return f.validateFrequencies()
	case DataTypeTFE:
		if err := checkFinite4D("TFRealE", f.TFRealE); err != nil {
			return err
		}
		if err := checkFinite4D("TFImagE", f.TFImagE); err != nil {
			return err
		}
		return f.validateFrequencies()
	}
	return nil
}

// validateRateAndDelay checks the FIR/SOS SamplingRate and Delay values.
func (f *File) validateRateAndDelay() error {
	if err := checkFinite("SamplingRate", f.SamplingRate); err != nil {
		return err
	}
	if err := checkFinite("Delay", f.Delay); err != nil {
		return err
	}
	for i, sr := range f.SamplingRate {
		if sr <= 0 {
			return invalid("SamplingRate", "[%d] = %g must be > 0", i, sr)
		}
	}
	return nil
}

// validateFrequencies checks the TF/TF-E frequency axis: finite, >= 0 and
// strictly increasing.
func (f *File) validateFrequencies() error {
	if err := checkFinite("Frequencies", f.Frequencies); err != nil {
		return err
	}
	for i, fr := range f.Frequencies {
		if fr < 0 {
			return invalid("Frequencies", "[%d] = %g is negative", i, fr)
		}
		if i > 0 && fr <= f.Frequencies[i-1] {
			return invalid("Frequencies", "must be strictly increasing: [%d] = %g follows %g",
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
					return invalid(p.name, "[%d][%d] = %v is not finite", i, j, v)
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
			return invalid(o.name, "%v has no direction", o.vec)
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
				return invalid(o.name, "[%d] = %v has no direction", i, v)
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
// an unset, zero vector. In spherical coordinates the up default's
// elevation follows the angle unit Save writes, so it is the zenith in
// radians too.
func (f *File) listenerOrientation() (view, up Vector3) {
	view, up = f.ListenerView, f.ListenerUp
	defView, defUp := Vector3{1, 0, 0}, Vector3{0, 0, 1}
	if f.listenerViewSpherical() {
		zenith := 90.0
		_, units := f.listenerViewCoordinates()
		if angleUnitIsRadian(units) {
			zenith = math.Pi / 2
		}
		defView, defUp = Vector3{0, 0, 1}, Vector3{0, zenith, 1}
	}
	if view == (Vector3{}) {
		view = defView
	}
	if up == (Vector3{}) {
		up = defUp
	}
	return view, up
}

// angleUnitIsRadian reports whether a spherical Units value such as
// "radian, radian, metre" gives its angles in radians; the SOFA default is
// degrees.
func angleUnitIsRadian(units string) bool {
	angle, _, _ := strings.Cut(units, ",")
	angle = strings.ToLower(strings.TrimSpace(angle))
	return angle == "rad" || strings.HasPrefix(angle, "radian")
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func finiteVector(v Vector3) bool { return finite(v.X) && finite(v.Y) && finite(v.Z) }

// The checkFinite helpers format an index only for the first bad value, so
// valid data costs no allocations.

func checkFinite(name string, vals []float64) error {
	for i, v := range vals {
		if !finite(v) {
			return invalid(name, "[%d] = %g is not finite", i, v)
		}
	}
	return nil
}

func checkFinite3D(name string, data [][][]float64) error {
	for i, a := range data {
		for j, b := range a {
			for k, v := range b {
				if !finite(v) {
					return invalid(name, "[%d][%d][%d] = %g is not finite", i, j, k, v)
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
						return invalid(name, "[%d][%d][%d][%d] = %g is not finite", i, j, k, l, v)
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
			return invalid(name, "[%d] = %v is not finite", i, v)
		}
	}
	return nil
}
