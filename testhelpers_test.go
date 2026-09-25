package sofa

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// optionalTestdata lists the reference files that CI cannot obtain: the
// sofacoustics.org files marked optional in scripts/fetch-testdata.sh (the
// server answers 403 to GitHub runners) and the local-only files listed in
// testdata/PROVENANCE.md. Keep it in sync with both.
var optionalTestdata = map[string]bool{
	"GeneralTF_2.0.sofa":            true,
	"GeneralTF-E_1.0.sofa":          true,
	"FreeFieldHRTF_1.0.sofa":        true,
	"SimpleFreeFieldHRSOS_1.0.sofa": true,
	"demo_FreeFieldHRTF_4_SH.sofa":  true,
	"OfficeII.sofa":                 true,
	"SingleRoomSRIR_1.1.sofa":       true,
}

// testdataPath returns the path of a third-party reference file in testdata/.
// The files are not committed (see testdata/PROVENANCE.md for sources and
// licences). A missing required file is a hard failure, never a skip, so that
// CI cannot silently go green without exercising real-world input; only the
// files in optionalTestdata, which CI cannot fetch, skip when absent.
func testdataPath(t testing.TB, name string) string {
	t.Helper()
	p := filepath.Join("testdata", name)
	if _, err := os.Stat(p); err != nil {
		if optionalTestdata[name] {
			t.Skipf("optional reference file %s is not available (%v); see testdata/PROVENANCE.md", p, err)
		}
		t.Fatalf("reference file %s is missing (%v): run `just fetch-testdata` "+
			"(or scripts/fetch-testdata.sh); sources are listed in testdata/PROVENANCE.md", p, err)
	}
	return p
}

// assertClose fails the test when got differs from want by more than tol.
func assertClose(t testing.TB, name string, got, want, tol float64) {
	t.Helper()
	if math.IsNaN(got) || math.Abs(got-want) > tol {
		t.Errorf("%s = %.17g, want %.17g (±%g)", name, got, want, tol)
	}
}
