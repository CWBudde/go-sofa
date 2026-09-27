package sofa

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func saveFixtures() []*File {
	return []*File{robustFIRFile(), robustTFFile(), robustTFEFile(), robustSOSFile()}
}

// TestSaveDeterministic checks that saving the same File repeatedly
// produces byte-identical output when SOURCE_DATE_EPOCH fixes the save
// time.
func TestSaveDeterministic(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1790000000")
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

// pinSaveTime makes Save stamp ts for the rest of the test, clearing any
// SOURCE_DATE_EPOCH the environment sets, which would take precedence.
func pinSaveTime(t *testing.T, ts time.Time) {
	t.Helper()
	t.Setenv("SOURCE_DATE_EPOCH", "")
	saveTime = func() time.Time { return ts }
	t.Cleanup(func() { saveTime = time.Now })
}

// TestSaveStampsEmptyDates checks that empty DateCreated/DateModified are
// stamped with the save time, so output differs across clock seconds (the
// reason reproducible output needs SOURCE_DATE_EPOCH), and that the File
// keeps its empty dates.
func TestSaveStampsEmptyDates(t *testing.T) {
	f := minimalFIRFile()
	f.DateCreated, f.DateModified = "", ""
	dir := t.TempDir()
	saveAt := func(ts time.Time, name string) []byte {
		t.Helper()
		pinSaveTime(t, ts)
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

// TestSaveProvenance checks that Save writes go-sofa as the API and the
// save time as DateModified, keeps DateCreated, notes a different
// previous API in History, and leaves the File unchanged.
func TestSaveProvenance(t *testing.T) {
	pinSaveTime(t, time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC))
	const (
		stamp   = "2026-09-27 10:00:00"
		created = "2020-01-02 03:04:05"
		ari     = "ARI SOFA API for Matlab/Octave"
	)
	v := moduleVersion()
	resaved := "resaved by go-sofa " + v + " from "
	cases := []struct {
		name, apiName, apiVersion, history string
		wantHistory                        string
	}{
		{"new File", "", "", "", ""},
		{"same go-sofa", defaultAPIName, v, "measured", "measured"},
		{"other API", ari, "0.4.4", "", resaved + ari + " 0.4.4"},
		{"other API with history", "MyTool", "3.1", "measured", "measured\n" + resaved + "MyTool 3.1"},
		{"history ending in a newline", "MyTool", "3.1", "measured\n", "measured\n" + resaved + "MyTool 3.1"},
		{"older go-sofa", defaultAPIName, "v0.2.0", "", resaved + "go-sofa v0.2.0"},
		{"no version", "MyTool", "", "", resaved + "MyTool"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := minimalFIRFile()
			f.APIName, f.APIVersion, f.History = tc.apiName, tc.apiVersion, tc.history
			f.DateCreated, f.DateModified = created, "2021-01-02 03:04:05"
			before := *f
			path := filepath.Join(t.TempDir(), "p.sofa")
			if err := f.Save(path); err != nil {
				t.Fatalf("Save: %v", err)
			}
			g, err := Open(path)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer g.Close()
			for _, c := range []struct{ name, got, want string }{
				{"APIName", g.APIName, defaultAPIName},
				{"APIVersion", g.APIVersion, v},
				{"DateCreated", g.DateCreated, created},
				{"DateModified", g.DateModified, stamp},
				{"History", g.History, tc.wantHistory},
			} {
				if c.got != c.want {
					t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
				}
			}
			if f.APIName != before.APIName || f.APIVersion != before.APIVersion ||
				f.DateModified != before.DateModified || f.History != before.History {
				t.Errorf("Save changed the File: APIName=%q APIVersion=%q DateModified=%q History=%q",
					f.APIName, f.APIVersion, f.DateModified, f.History)
			}
		})
	}
}

// TestSaveSourceDateEpoch checks that SOURCE_DATE_EPOCH fixes the save
// time, so output is byte-identical whatever the clock says, and that a
// malformed value fails Save and WriteTo without writing anything.
func TestSaveSourceDateEpoch(t *testing.T) {
	f := minimalFIRFile()
	f.DateCreated, f.DateModified = "", ""
	dir := t.TempDir()
	t.Setenv("SOURCE_DATE_EPOCH", "1790000000")
	var saved [][]byte
	for i, ts := range []time.Time{time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)} {
		saveTime = func() time.Time { return ts }
		t.Cleanup(func() { saveTime = time.Now })
		path := filepath.Join(dir, strconv.Itoa(i)+".sofa")
		if err := f.Save(path); err != nil {
			t.Fatalf("Save: %v", err)
		}
		g, err := Open(path)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		if g.DateCreated != "2026-09-21 14:13:20" || g.DateModified != "2026-09-21 14:13:20" {
			t.Errorf("DateCreated/DateModified = %q/%q, want the epoch's 2026-09-21 14:13:20", g.DateCreated, g.DateModified)
		}
		g.Close()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		saved = append(saved, b)
	}
	if !bytes.Equal(saved[0], saved[1]) {
		t.Error("saves at different clock times differ despite SOURCE_DATE_EPOCH")
	}

	for _, bad := range []string{"soon", "-1", "1.5"} {
		t.Run(bad, func(t *testing.T) {
			t.Setenv("SOURCE_DATE_EPOCH", bad)
			dir := t.TempDir()
			var numErr *strconv.NumError
			if err := f.Save(filepath.Join(dir, "bad.sofa")); !errors.As(err, &numErr) {
				t.Errorf("Save error = %v, want a *strconv.NumError", err)
			}
			assertOnlyFile(t, dir)
			var buf bytes.Buffer
			if n, err := f.WriteTo(&buf); !errors.As(err, &numErr) || n != 0 || buf.Len() != 0 {
				t.Errorf("WriteTo = %d, %v (%d bytes written), want 0 and a *strconv.NumError", n, err, buf.Len())
			}
		})
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

// TestSaveMissingDirectory checks that Save into a directory that does not
// exist fails creating the temporary file and creates nothing.
func TestSaveMissingDirectory(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "does", "not", "exist.sofa")
	err := robustFIRFile().Save(path)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Save error = %v, want fs.ErrNotExist", err)
	}
	assertOnlyFile(t, root)
}

// TestSaveTargetIsDirectory checks that Save refuses a path naming a
// directory before writing anything: no dataset is written, no temporary
// file is left behind, and the directory is kept.
func TestSaveTargetIsDirectory(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "dir.sofa")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	wrote := false
	writeVariableTestHook = func(string) error { wrote = true; return nil }
	t.Cleanup(func() { writeVariableTestHook = nil })

	if err := robustFIRFile().Save(target); err == nil {
		t.Fatal("Save to a directory succeeded, want error")
	}
	if wrote {
		t.Error("Save wrote variables before rejecting the directory target")
	}
	fi, err := os.Stat(target)
	if err != nil || !fi.IsDir() {
		t.Fatalf("target after Save: %v, %v; want the directory kept", fi, err)
	}
	assertOnlyFile(t, root, "dir.sofa")
	assertOnlyFile(t, target)
}

// TestSaveAudioWriteErrors injects a write failure into each audio
// variable of each DataType and checks the error propagates out of Save
// and the temporary file is removed.
func TestSaveAudioWriteErrors(t *testing.T) {
	cases := []struct {
		fixture func() *File
		vars    []string
	}{
		{robustFIRFile, []string{"/Data.IR", "/Data.SamplingRate", "/Data.Delay"}},
		{robustTFFile, []string{"/Data.Real", "/Data.Imag"}},
		{robustTFEFile, []string{"/Data.Real", "/Data.Imag"}},
		{robustSOSFile, []string{"/Data.SOS", "/Data.SamplingRate", "/Data.Delay"}},
	}
	for _, c := range cases {
		f := c.fixture()
		for _, name := range c.vars {
			t.Run(f.DataType+name, func(t *testing.T) {
				injected := errors.New("injected write failure")
				called := false
				writeVariableTestHook = func(n string) error {
					if n == name {
						called = true
						return injected
					}
					return nil
				}
				t.Cleanup(func() { writeVariableTestHook = nil })

				dir := t.TempDir()
				err := f.Save(filepath.Join(dir, "out.sofa"))
				if !called {
					t.Fatalf("Save never wrote %s", name)
				}
				if !errors.Is(err, injected) {
					t.Fatalf("Save error = %v, want injected failure", err)
				}
				assertOnlyFile(t, dir)
			})
		}
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
