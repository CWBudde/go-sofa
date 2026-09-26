package sofa

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestOpenReaderMatchesOpen checks that reading a file through an
// io.ReaderAt gives the same File as reading it from disk.
func TestOpenReaderMatchesOpen(t *testing.T) {
	paths := []string{
		testdataPath(t, "MIT_KEMAR_normal_pinna.sofa"),
		testdataPath(t, "tester.sofa"),
		filepath.Join("testdata", "sofar", "GeneralTF-E_1.0.sofa"),
		filepath.Join("testdata", "sofar", "SimpleFreeFieldHRSOS_1.0.sofa"),
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			want, err := Open(path)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			got, err := OpenReader(bytes.NewReader(b), int64(len(b)))
			if err != nil {
				t.Fatalf("OpenReader: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Error("OpenReader and Open read different Files")
			}
		})
	}
}

func TestOpenReaderErrors(t *testing.T) {
	var buf bytes.Buffer
	if _, err := minimalFIRFile().WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	b := buf.Bytes()
	half := b[:len(b)/2]
	for name, tc := range map[string]struct {
		data []byte
		size int64
	}{
		"empty":               {nil, 0},
		"truncated":           {half, int64(len(half))},
		"shorter than size":   {half, int64(len(b))},
		"size cuts the file":  {b, int64(len(b) / 2)},
		"negative size":       {b, -1},
		"not HDF5":            {[]byte("not an HDF5 file at all"), 23},
		"superblock only":     {b[:48], 48},
		"garbage after magic": {append(append([]byte{}, b[:8]...), make([]byte, 64)...), 72},
	} {
		t.Run(name, func(t *testing.T) {
			f, err := OpenReader(bytes.NewReader(tc.data), tc.size)
			if err == nil {
				t.Fatal("OpenReader succeeded, want error")
			}
			if f != nil {
				t.Error("OpenReader returned a File with its error")
			}
		})
	}
}

// TestOpenLazyReader reads measurements from a file held in memory.
func TestOpenLazyReader(t *testing.T) {
	eager := streamFIRFile(5, 2, 8)
	var buf bytes.Buffer
	if _, err := eager.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	f, err := OpenLazyReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("OpenLazyReader: %v", err)
	}
	if len(f.ImpulseResponses) != 0 {
		t.Error("OpenLazyReader loaded the audio data")
	}
	for m := range eager.M {
		ir, err := f.ReadMeasurement(m)
		if err != nil {
			t.Fatalf("ReadMeasurement(%d): %v", m, err)
		}
		if err := sameBits(ir, eager.ImpulseResponses[m]); err != nil {
			t.Errorf("measurement %d: %v", m, err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := f.ReadMeasurement(0); !errors.Is(err, ErrClosed) {
		t.Errorf("ReadMeasurement after Close: %v, want ErrClosed", err)
	}
	if _, err := OpenLazyReader(bytes.NewReader(buf.Bytes()[:100]), 100); err == nil {
		t.Error("OpenLazyReader of a truncated file succeeded")
	}
}

// TestWriteToMatchesSave checks that WriteTo produces the bytes Save
// writes and reports their count.
func TestWriteToMatchesSave(t *testing.T) {
	for _, f := range saveFixtures() {
		t.Run(f.DataType, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "out.sofa")
			if err := f.Save(path); err != nil {
				t.Fatalf("Save: %v", err)
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			n, err := f.WriteTo(&buf)
			if err != nil {
				t.Fatalf("WriteTo: %v", err)
			}
			if n != int64(buf.Len()) {
				t.Errorf("WriteTo returned %d, wrote %d bytes", n, buf.Len())
			}
			if !bytes.Equal(buf.Bytes(), want) {
				t.Errorf("WriteTo output differs from Save (%d vs %d bytes)", buf.Len(), len(want))
			}
		})
	}
}

// failingWriter accepts limit bytes, then fails.
type failingWriter struct {
	limit int
	buf   bytes.Buffer
}

var errWriterFull = errors.New("writer full")

func (w *failingWriter) Write(p []byte) (int, error) {
	if room := w.limit - w.buf.Len(); len(p) > room {
		w.buf.Write(p[:room])
		return room, errWriterFull
	}
	return w.buf.Write(p)
}

func TestWriteToErrors(t *testing.T) {
	t.Run("failing writer", func(t *testing.T) {
		for _, limit := range []int{0, 100} {
			w := &failingWriter{limit: limit}
			n, err := robustFIRFile().WriteTo(w)
			if !errors.Is(err, errWriterFull) {
				t.Fatalf("limit %d: WriteTo error = %v, want errWriterFull", limit, err)
			}
			if n != int64(limit) || w.buf.Len() != limit {
				t.Errorf("limit %d: WriteTo returned %d with %d bytes written", limit, n, w.buf.Len())
			}
		}
	})

	t.Run("validation", func(t *testing.T) {
		f := robustFIRFile()
		f.M = 0
		var buf bytes.Buffer
		n, err := f.WriteTo(&buf)
		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("WriteTo error = %v, want *ValidationError", err)
		}
		if n != 0 || buf.Len() != 0 {
			t.Errorf("WriteTo wrote %d (reported %d) bytes on a validation error", buf.Len(), n)
		}
	})

	t.Run("encoding failure", func(t *testing.T) {
		injected := errors.New("injected failure")
		saveTestHook = func() error { return injected }
		t.Cleanup(func() { saveTestHook = nil })
		var buf bytes.Buffer
		n, err := robustTFFile().WriteTo(&buf)
		if !errors.Is(err, injected) {
			t.Fatalf("WriteTo error = %v, want injected failure", err)
		}
		if n != 0 || buf.Len() != 0 {
			t.Errorf("WriteTo wrote %d (reported %d) bytes on a failed write", buf.Len(), n)
		}
	})

	t.Run("lazy without audio", func(t *testing.T) {
		f := openLazy(t, saveTemp(t, streamFIRFile(3, 1, 4), "lazy.sofa"))
		var buf bytes.Buffer
		if n, err := f.WriteTo(&buf); !errors.Is(err, ErrNotLoaded) || n != 0 || buf.Len() != 0 {
			t.Errorf("WriteTo of lazy file = %d, %v (%d bytes); want 0, ErrNotLoaded", n, err, buf.Len())
		}
	})
}
