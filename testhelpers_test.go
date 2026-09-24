package sofa

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// testdataPath returns the path of a third-party reference file in testdata/.
// The files are not committed (see testdata/PROVENANCE.md for sources and
// licences); a missing file is a hard failure, never a skip, so that CI cannot
// silently go green without exercising real-world input.
func testdataPath(t testing.TB, name string) string {
	t.Helper()
	p := filepath.Join("testdata", name)
	if _, err := os.Stat(p); err != nil {
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
