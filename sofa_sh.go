package sofa

import (
	"math"
	"strconv"
	"strings"
)

// IsSHEncoded reports whether this file represents spherical-harmonic
// (SH) encoded HRTF/transfer-function data. SH SOFA files reuse the
// TF-E DataType and store one SH coefficient per emitter
// (E = (Lmax+1)²); AES69 marks them with
// EmitterPosition:Type = "spherical harmonics". See SHOrder for the
// detection rule.
func (f *File) IsSHEncoded() bool {
	_, ok := f.SHOrder()
	return ok
}

// declaresSH reports whether the file declares spherical-harmonic
// encoding. A set EmitterPositionType decides on its own; only when it is
// empty (in-memory files, writers that omit it) do the convention name
// and History heuristics apply.
func (f *File) declaresSH() bool {
	if f.EmitterPositionType != "" {
		return f.emitterTypeIsSH()
	}
	return f.claimsSHHeuristically()
}

func (f *File) emitterTypeIsSH() bool {
	return strings.EqualFold(strings.TrimSpace(f.EmitterPositionType), CoordinateSphericalHarmonics)
}

// claimsSHHeuristically reports whether the convention name contains
// "SH" (e.g. "SimpleFreeFieldHRSH") or History mentions "spherical
// harmonic" (e.g. "Converted to Spherical Harmonics").
func (f *File) claimsSHHeuristically() bool {
	return strings.Contains(strings.ToUpper(f.SOFAConventions), "SH") ||
		strings.Contains(strings.ToLower(f.History), "spherical harmonic")
}

// SHOrder returns the spherical-harmonic order Lmax encoded in this
// file. ok is false if the file is not SH-encoded. Detection rule:
// DataType is TF-E, the file declares SH (EmitterPosition:Type is
// "spherical harmonics", or — only when that Type is empty — the
// convention name or History says so), and E equals (L+1)² for some
// integer L ≥ 0, so E = 1 is order 0.
func (f *File) SHOrder() (lmax int, ok bool) {
	if f.DataType != dataTypeTFE || !f.declaresSH() {
		return 0, false
	}
	root, square := shCoefficientRoot(f.E)
	if !square {
		return 0, false
	}
	return root - 1, true
}

// SHCoefficientCount returns the number of SH coefficients stored
// per (measurement, receiver, frequency) tuple. Returns 0 when the
// file is not SH-encoded; otherwise returns E = (Lmax+1)².
func (f *File) SHCoefficientCount() int {
	if !f.IsSHEncoded() {
		return 0
	}
	return f.E
}

// SHWarnings returns advisory messages about possibly-malformed or
// possibly-undocumented spherical-harmonic encoding. Empty when the
// file is unambiguous (either clearly SH or clearly not). Callers
// should surface these to users without treating them as errors.
func (f *File) SHWarnings() []string {
	var out []string
	declares := f.declaresSH()
	root, square := shCoefficientRoot(f.E)

	if declares && f.DataType != dataTypeTFE {
		out = append(out, "metadata declares spherical-harmonic encoding but DataType is "+
			f.DataType+" (expected TF-E)")
	}
	if declares && !square {
		out = append(out,
			"metadata declares spherical-harmonic encoding but E is not (L+1)² for any L≥0")
	}
	if f.EmitterPositionType != "" && !f.emitterTypeIsSH() && f.claimsSHHeuristically() {
		out = append(out, "SOFAConventions or History suggests spherical harmonics but EmitterPosition Type is "+
			strconv.Quote(f.EmitterPositionType)+"; not treated as SH")
	}
	if !declares && f.EmitterPositionType == "" && square && root >= 2 && f.DataType == dataTypeTFE {
		out = append(out,
			"E is a perfect square consistent with SH order "+
				strconv.Itoa(root-1)+", but neither EmitterPosition Type nor SOFAConventions/History declares SH encoding")
	}
	return out
}

// shCoefficientRoot reports whether n equals (L+1)² for some integer
// L≥0, returning L+1 when so.
func shCoefficientRoot(n int) (root int, ok bool) {
	if n < 1 {
		return 0, false
	}
	r := int(math.Round(math.Sqrt(float64(n))))
	if r*r != n {
		return 0, false
	}
	return r, true
}
