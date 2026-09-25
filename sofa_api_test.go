package sofa

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// openFDs counts the file descriptors this process holds.
func openFDs(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/dev/fd")
	if err != nil {
		t.Skipf("cannot list /dev/fd: %v", err)
	}
	return len(entries)
}

// TestOpenReleasesHandle checks that Open reads everything eagerly and
// closes the underlying file before it returns, so a File that is never
// closed holds no descriptor.
func TestOpenReleasesHandle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no /dev/fd on Windows")
	}
	path := filepath.Join(t.TempDir(), "handle.sofa")
	if err := minimalFIRFile().Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	before := openFDs(t)
	f, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if after := openFDs(t); after != before {
		t.Errorf("open descriptors: %d before Open, %d after", before, after)
	}
	if err := f.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}
