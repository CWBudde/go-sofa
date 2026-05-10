package sofa

import (
	"math"
	"strconv"
	"strings"
)

// IsSHEncoded reports whether this file represents spherical-harmonic
// (SH) encoded HRTF/transfer-function data. SH SOFA files reuse the
// existing TF-E DataType and store one SH coefficient per emitter
// position (E = (Lmax+1)²); either the convention name or the
// History attribute carries the SH semantic. See PLAN.md "SH layout
// reference" for background.
func (f *File) IsSHEncoded() bool {
	_, ok := f.SHOrder()
	return ok
}

// claimsSH reports whether the file's metadata declares spherical-
// harmonic encoding, either via the convention name (e.g.
// "SimpleFreeFieldHRSH") or via the History attribute (e.g.
// "Converted to Spherical Harmonics"). Convention name match is on
// the substring "SH"; History match is on "spherical harmonic".
func (f *File) claimsSH() bool {
	if strings.Contains(strings.ToUpper(f.SOFAConventions), "SH") {
		return true
	}
	if strings.Contains(strings.ToLower(f.History), "spherical harmonic") {
		return true
	}
	return false
}

// SHOrder returns the spherical-harmonic order Lmax encoded in this
// file. ok is false if the file is not SH-encoded. Detection rule:
// the file claims SH (via convention name or History), E ≥ 4, and E
// equals (L+1)² for some integer L ≥ 1.
func (f *File) SHOrder() (lmax int, ok bool) {
	if !f.claimsSH() {
		return 0, false
	}
	if f.E < 4 {
		return 0, false
	}
	root := math.Sqrt(float64(f.E))
	rounded := int(math.Round(root))
	if rounded*rounded != f.E {
		return 0, false
	}
	return rounded - 1, true
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
	claims := f.claimsSH()
	square, sqrtL := isPerfectSquareGE4(f.E)

	if claims && f.DataType != dataTypeTFE {
		out = append(out, "metadata claims spherical-harmonic encoding but DataType is "+
			f.DataType+" (expected TF-E)")
	}
	if claims && !square {
		out = append(out,
			"metadata claims spherical-harmonic encoding but E is not (L+1)² for any L≥1")
	}
	if !claims && square && f.DataType == dataTypeTFE {
		out = append(out,
			"E is a perfect square consistent with SH order "+
				strconv.Itoa(sqrtL-1)+", but neither SOFAConventions nor History declares SH encoding")
	}
	return out
}

// isPerfectSquareGE4 reports whether n equals (L+1)² for some
// integer L≥1, returning the square root when so.
func isPerfectSquareGE4(n int) (ok bool, root int) {
	if n < 4 {
		return false, 0
	}
	r := int(math.Round(math.Sqrt(float64(n))))
	if r*r != n {
		return false, 0
	}
	return true, r
}
