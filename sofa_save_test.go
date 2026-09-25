package sofa

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func saveFixtures() []*File {
	return []*File{robustFIRFile(), robustTFFile(), robustTFEFile(), robustSOSFile()}
}

// TestSaveDeterministic checks that saving the same File repeatedly
// produces byte-identical output.
func TestSaveDeterministic(t *testing.T) {
	for _, f := range saveFixtures() {
		t.Run(f.DataType, func(t *testing.T) {
			dir := t.TempDir()
			var first []byte
			for i := range 5 {
				path := filepath.Join(dir, "out.sofa")
				if err := f.Save(path); err != nil {
					t.Fatalf("Save #%d: %v", i, err)
				}
				b, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if i == 0 {
					first = b
					continue
				}
				if !bytes.Equal(first, b) {
					t.Fatalf("Save #%d output differs from Save #0 (%d vs %d bytes)", i, len(b), len(first))
				}
			}
		})
	}
}

// TestSaveStampsEmptyDates checks that empty DateCreated/DateModified are
// stamped with the save time, so output differs across clock seconds (the
// reason the determinism guarantee requires set dates), and that the File
// keeps its empty dates.
func TestSaveStampsEmptyDates(t *testing.T) {
	f := minimalFIRFile()
	f.DateCreated, f.DateModified = "", ""
	dir := t.TempDir()
	saveAt := func(ts time.Time, name string) []byte {
		t.Helper()
		saveTime = func() time.Time { return ts }
		t.Cleanup(func() { saveTime = time.Now })
		path := filepath.Join(dir, name)
		if err := f.Save(path); err != nil {
			t.Fatalf("Save: %v", err)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	t0 := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	a := saveAt(t0, "a.sofa")
	if b := saveAt(t0, "b.sofa"); !bytes.Equal(a, b) {
		t.Error("two saves at the same time differ")
	}
	if c := saveAt(t0.Add(time.Second), "c.sofa"); bytes.Equal(a, c) {
		t.Error("saves one second apart are identical; empty dates were not stamped")
	}
	if f.DateCreated != "" || f.DateModified != "" {
		t.Errorf("Save changed the File's dates to %q/%q", f.DateCreated, f.DateModified)
	}
}

// TestSaveFailureLeavesTargetUntouched forces a failure after validation
// and all datasets are written, and checks the pre-existing target and
// the directory are unchanged.
func TestSaveFailureLeavesTargetUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.sofa")
	original := []byte("original contents, not HDF5")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	injected := errors.New("injected failure")
	saveTestHook = func() error { return injected }
	t.Cleanup(func() { saveTestHook = nil })

	err := robustFIRFile().Save(path)
	if !errors.Is(err, injected) {
		t.Fatalf("Save error = %v, want injected failure", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("target modified by failed Save: %q", got)
	}
	assertOnlyFile(t, dir, "existing.sofa")
}

// TestSaveFailureNoTarget checks that a failed Save to a new path creates
// nothing.
func TestSaveFailureNoTarget(t *testing.T) {
	dir := t.TempDir()
	saveTestHook = func() error { return errors.New("boom") }
	t.Cleanup(func() { saveTestHook = nil })

	if err := robustTFFile().Save(filepath.Join(dir, "new.sofa")); err == nil {
		t.Fatal("Save succeeded, want error")
	}
	assertOnlyFile(t, dir)
}

// TestSaveValidationFailureNoTemp checks validation errors happen before
// any file is created.
func TestSaveValidationFailureNoTemp(t *testing.T) {
	dir := t.TempDir()
	f := robustFIRFile()
	f.M = 0
	if err := f.Save(filepath.Join(dir, "x.sofa")); err == nil {
		t.Fatal("Save succeeded, want validation error")
	}
	assertOnlyFile(t, dir)
}

func TestSaveReplacesExistingAndKeepsMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "x.sofa")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := robustSOSFile().Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want 0640", fi.Mode().Perm())
	}
	g, err := Open(path)
	if err != nil {
		t.Fatalf("Open after Save: %v", err)
	}
	defer g.Close()
	if g.DataType != "SOS" {
		t.Errorf("DataType = %q, want SOS", g.DataType)
	}
	assertOnlyFile(t, dir, "x.sofa")

	// New file gets 0644.
	newPath := filepath.Join(dir, "new.sofa")
	if err := robustFIRFile().Save(newPath); err != nil {
		t.Fatalf("Save: %v", err)
	}
	fi, err = os.Stat(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o644 {
		t.Errorf("new file mode = %v, want 0644", fi.Mode().Perm())
	}
}

func TestSaveMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does", "not", "exist.sofa")
	err := robustFIRFile().Save(path)
	if err == nil || !strings.Contains(err.Error(), "temporary file") {
		t.Fatalf("Save error = %v, want temporary-file error", err)
	}
}

func TestSaveRoundTripAllTypes(t *testing.T) {
	for _, f := range saveFixtures() {
		t.Run(f.DataType, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "rt.sofa")
			if err := f.Save(path); err != nil {
				t.Fatalf("Save: %v", err)
			}
			g, err := Open(path)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer g.Close()
			if g.M != f.M || g.R != f.R || g.E != f.E || g.N != f.N {
				t.Errorf("dims = %d/%d/%d/%d, want %d/%d/%d/%d",
					g.M, g.R, g.E, g.N, f.M, f.R, f.E, f.N)
			}
		})
	}
}

// assertOnlyFile fails unless dir contains exactly the named entries
// (in particular, no leftover temporary files).
func assertOnlyFile(t *testing.T, dir string, names ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if strings.Join(got, ",") != strings.Join(names, ",") {
		t.Fatalf("directory contents = %v, want %v", got, names)
	}
}
