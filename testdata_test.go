package sofa

import (
	"errors"
	"io/fs"
	"os"
	"testing"
)

// requireTestdata skips the test when a real-world SOFA file is missing.
// testdata/ is gitignored (licences and file sizes), so these files exist
// only on machines where they were downloaded, not in CI.
func requireTestdata(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		t.Skipf("testdata file %s not present (testdata/ is not committed)", path)
	}
}
