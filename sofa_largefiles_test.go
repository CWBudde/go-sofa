//go:build largefiles

package sofa

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// The large-file suite runs only with `go test -tags largefiles`; it writes
// a synthetic FIR file of more than 100 MB of impulse responses once per
// test binary and removes it in TestMain.

// Dimensions of the large file: 6400·2·1024·8 B = 104,857,600 B of Data.IR.
const (
	largeM = 6400
	largeR = 2
	largeN = 1024
)

// largeDataBytes is the size of the large file's impulse responses.
const largeDataBytes = largeM * largeR * largeN * 8

var (
	largeDir     string
	largeOnce    sync.Once
	largePathVal string
	largeErr     error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if largeDir != "" {
		_ = os.RemoveAll(largeDir)
	}
	os.Exit(code)
}

// largeValue is sample k of measurement m, receiver r in the large file.
func largeValue(m, r, k int) float64 {
	return float64(m) + float64(r)/4 + float64(k)/float64(4*largeN)
}

// largeFIRFile builds the large file in memory.
func largeFIRFile() *File {
	f := robustBase(DataTypeFIR, largeM, largeR, 1, largeN)
	flat := make([]float64, largeM*largeR*largeN)
	for m := range largeM {
		for r := range largeR {
			for k := range largeN {
				flat[(m*largeR+r)*largeN+k] = largeValue(m, r, k)
			}
		}
	}
	f.ImpulseResponses = reshapeIR(flat, largeM, largeR, largeN)
	f.SamplingRate = []float64{48000}
	return f
}

// largePath returns the path of the large file, writing it on first use.
func largePath(tb testing.TB) string {
	tb.Helper()
	largeOnce.Do(func() {
		largeDir, largeErr = os.MkdirTemp("", "go-sofa-largefiles-")
		if largeErr != nil {
			return
		}
		largePathVal = filepath.Join(largeDir, "large.sofa")
		largeErr = largeFIRFile().Save(largePathVal)
		runtime.GC()
	})
	if largeErr != nil {
		tb.Fatalf("write large file: %v", largeErr)
	}
	return largePathVal
}

func TestLargeFileSize(t *testing.T) {
	fi, err := os.Stat(largePath(t))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() < 100<<20 {
		t.Errorf("large file is %d B, want ≥ 100 MiB", fi.Size())
	}
}

// TestLargeFileStreamMatchesEager streams every measurement of the large
// file and compares it with the generator and with Open.
func TestLargeFileStreamMatchesEager(t *testing.T) {
	path := largePath(t)
	lazy := openLazy(t, path)
	if err := lazy.RangeMeasurements(func(m int, ir [][]float64) error {
		for r := range largeR {
			for _, k := range []int{0, 1, largeN / 2, largeN - 1} {
				if want := largeValue(m, r, k); ir[r][k] != want {
					return fmt.Errorf("measurement %d receiver %d sample %d = %v, want %v", m, r, k, ir[r][k], want)
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	eager, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, m := range []int{0, 1, largeM / 2, largeM - 1} {
		ir, err := lazy.ReadMeasurement(m)
		if err != nil {
			t.Fatalf("ReadMeasurement(%d): %v", m, err)
		}
		if err := sameBits(ir, eager.ImpulseResponses[m]); err != nil {
			t.Errorf("measurement %d: %v", m, err)
		}
	}
}

// TestLargeFileLazyHeap checks that streaming the large file never keeps
// more than a small fraction of it live on the heap (measured after a GC,
// so garbage from earlier measurements does not count).
func TestLargeFileLazyHeap(t *testing.T) {
	path := largePath(t)
	var base, peak runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&base)
	lazy := openLazy(t, path)
	var maxHeap uint64
	if err := lazy.RangeMeasurements(func(m int, _ [][]float64) error {
		if m%1000 == 0 {
			runtime.GC()
			runtime.ReadMemStats(&peak)
			maxHeap = max(maxHeap, peak.HeapAlloc)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	grown := int64(maxHeap) - int64(base.HeapAlloc) //nolint:gosec // heap sizes fit int64
	t.Logf("live heap growth while streaming %d B of data: %d B", largeDataBytes, grown)
	if grown > largeDataBytes/10 {
		t.Errorf("heap grew by %d B while streaming, want < 10%% of %d B", grown, largeDataBytes)
	}
}

// BenchmarkStreamVsEager reads every impulse response of the large file,
// eagerly with Open or one measurement at a time with OpenLazy.
func BenchmarkStreamVsEager(b *testing.B) {
	path := largePath(b)
	var sink float64
	b.Run("eager", func(b *testing.B) {
		b.SetBytes(largeDataBytes)
		b.ReportAllocs()
		for b.Loop() {
			f, err := Open(path)
			if err != nil {
				b.Fatal(err)
			}
			for _, ir := range f.ImpulseResponses {
				sink += ir[0][0]
			}
		}
	})
	b.Run("stream", func(b *testing.B) {
		b.SetBytes(largeDataBytes)
		b.ReportAllocs()
		for b.Loop() {
			f, err := OpenLazy(path)
			if err != nil {
				b.Fatal(err)
			}
			if err := f.RangeMeasurements(func(_ int, ir [][]float64) error {
				sink += ir[0][0]
				return nil
			}); err != nil {
				b.Fatal(err)
			}
			if err := f.Close(); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("stream-one", func(b *testing.B) {
		f, err := OpenLazy(path)
		if err != nil {
			b.Fatal(err)
		}
		defer f.Close()
		b.SetBytes(largeR * largeN * 8)
		b.ReportAllocs()
		m := 0
		for b.Loop() {
			ir, err := f.ReadMeasurement(m)
			if err != nil {
				b.Fatal(err)
			}
			sink += ir[0][0]
			m = (m + 7919) % largeM
		}
	})
	_ = sink
}

// BenchmarkWriteLarge saves the large file.
func BenchmarkWriteLarge(b *testing.B) {
	f := largeFIRFile()
	path := filepath.Join(b.TempDir(), "write.sofa")
	b.SetBytes(largeDataBytes)
	b.ReportAllocs()
	for b.Loop() {
		if err := f.Save(path); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReadLarge opens the large file eagerly.
func BenchmarkReadLarge(b *testing.B) {
	path := largePath(b)
	b.SetBytes(largeDataBytes)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Open(path); err != nil {
			b.Fatal(err)
		}
	}
}
