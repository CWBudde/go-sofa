package sofa

import (
	"errors"
	"math"
	"path/filepath"
	"strings"
	"testing"

	hdf5 "github.com/cwbudde/go-hdf5"
)

// craftedDim describes one dimension-scale dataset of a hand-built file.
// If name is non-empty it is stored as the NAME attribute; value is the
// single float64 stored in the dataset.
type craftedDim struct {
	name  string
	value float64
}

// dimNAME returns a netCDF "dimension but not variable" NAME attribute
// carrying an arbitrary (possibly invalid) size token.
func dimNAME(size string) string {
	return "This is a netCDF dimension but not a netCDF variable.         " + size
}

// writeCraftedFIR builds a FIR SOFA file with arbitrary dimension scales
// and a zero-filled Data.IR of shape irShape, bypassing Save's validation.
func writeCraftedFIR(t *testing.T, dims map[string]craftedDim, irShape []uint64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "crafted.sofa")
	fw, err := hdf5.CreateForWrite(path, hdf5.CreateTruncate,
		hdf5.WithRootAttribute("Conventions", "SOFA"),
		hdf5.WithRootAttribute("Version", "2.1"),
		hdf5.WithRootAttribute("SOFAConventions", "GeneralFIR"),
		hdf5.WithRootAttribute("SOFAConventionsVersion", "1.0"),
		hdf5.WithRootAttribute("DataType", "FIR"),
	)
	if err != nil {
		t.Fatalf("CreateForWrite: %v", err)
	}
	for _, n := range []string{"M", "R", "E", "N"} {
		d := dims[n]
		var opts []hdf5.DatasetOption
		if d.name != "" {
			opts = append(opts, hdf5.WithAttribute("NAME", d.name))
		}
		ds, err := fw.CreateDataset("/"+n, hdf5.Float64, []uint64{1}, opts...)
		if err != nil {
			t.Fatalf("create /%s: %v", n, err)
		}
		if err := ds.Write([]float64{d.value}); err != nil {
			t.Fatalf("write /%s: %v", n, err)
		}
	}
	irLen := uint64(1)
	for _, d := range irShape {
		irLen *= d
	}
	ir := make([]float64, irLen)
	ds, err := fw.CreateDataset("/Data.IR", hdf5.Float64, irShape)
	if err != nil {
		t.Fatalf("create Data.IR: %v", err)
	}
	if err := ds.Write(ir); err != nil {
		t.Fatalf("write Data.IR: %v", err)
	}
	sr, err := fw.CreateDataset("/Data.SamplingRate", hdf5.Float64, []uint64{1})
	if err != nil {
		t.Fatalf("create Data.SamplingRate: %v", err)
	}
	if err := sr.Write([]float64{48000}); err != nil {
		t.Fatalf("write Data.SamplingRate: %v", err)
	}
	if err := fw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return path
}

func named(size string) craftedDim { return craftedDim{name: dimNAME(size), value: 0} }

func TestOpenRejectsInvalidDimensions(t *testing.T) {
	ok := named("1")
	tests := []struct {
		name    string
		dims    map[string]craftedDim
		irLen   int
		wantErr string
	}{
		{
			// M*R*N == 16 == len(Data.IR): used to panic in reshapeIR.
			name:    "negative M and R in NAME",
			dims:    map[string]craftedDim{"M": named("-1"), "R": named("-2"), "E": ok, "N": named("8")},
			irLen:   16,
			wantErr: "negative size",
		},
		{
			name:    "negative N in NAME",
			dims:    map[string]craftedDim{"M": ok, "R": ok, "E": ok, "N": named("-4")},
			irLen:   4,
			wantErr: "negative size",
		},
		{
			name:    "dimension above cap",
			dims:    map[string]craftedDim{"M": named("1099511627776"), "R": ok, "E": ok, "N": ok},
			irLen:   1,
			wantErr: "out of range",
		},
		{
			// Each factor is within the cap but the product is not, and
			// M*R*N overflows int64 without the checked multiplication.
			name: "product overflow",
			dims: map[string]craftedDim{
				"M": named("1073741824"), "R": named("1073741824"),
				"E": ok, "N": named("1073741824"),
			},
			irLen:   1,
			wantErr: "exceeds limit",
		},
		{
			name:    "scalar N NaN",
			dims:    map[string]craftedDim{"M": ok, "R": ok, "E": ok, "N": {value: math.NaN()}},
			irLen:   1,
			wantErr: "non-finite",
		},
		{
			name:    "scalar N +Inf",
			dims:    map[string]craftedDim{"M": ok, "R": ok, "E": ok, "N": {value: math.Inf(1)}},
			irLen:   1,
			wantErr: "non-finite",
		},
		{
			name:    "scalar N non-integer",
			dims:    map[string]craftedDim{"M": ok, "R": ok, "E": ok, "N": {value: 2.5}},
			irLen:   2,
			wantErr: "non-integer",
		},
		{
			name:    "scalar N negative",
			dims:    map[string]craftedDim{"M": ok, "R": ok, "E": ok, "N": {value: -3}},
			irLen:   3,
			wantErr: "out of range",
		},
		{
			name:    "scalar N zero",
			dims:    map[string]craftedDim{"M": ok, "R": ok, "E": ok, "N": {value: 0}},
			irLen:   0,
			wantErr: "out of range",
		},
		{
			name:    "scalar M huge",
			dims:    map[string]craftedDim{"M": {value: 1e300}, "R": ok, "E": ok, "N": ok},
			irLen:   1,
			wantErr: "out of range",
		},
		{
			// A scalar count of 0 used to be read as dimension 1.
			name:    "scalar M zero",
			dims:    map[string]craftedDim{"M": {value: 0}, "R": ok, "E": ok, "N": ok},
			irLen:   1,
			wantErr: "out of range",
		},
		{
			name:    "scalar R negative",
			dims:    map[string]craftedDim{"M": ok, "R": {value: -2}, "E": ok, "N": ok},
			irLen:   1,
			wantErr: "out of range",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeCraftedFIR(t, tt.dims, []uint64{uint64(max(tt.irLen, 1))}) //nolint:gosec // small test sizes
			f, err := Open(path)
			if err == nil {
				_ = f.Close()
				t.Fatalf("Open succeeded (M=%d R=%d E=%d N=%d), want error containing %q",
					f.M, f.R, f.E, f.N, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Open error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// TestOpenCraftedValidDimensions checks the crafted-file helper itself
// produces an openable file, so the rejections above are meaningful.
func TestOpenCraftedValidDimensions(t *testing.T) {
	path := writeCraftedFIR(t, map[string]craftedDim{
		"M": named("2"), "R": named("2"), "E": named("1"), "N": {value: 4},
	}, []uint64{2, 2, 4})
	f, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	if f.M != 2 || f.R != 2 || f.E != 1 || f.N != 4 {
		t.Fatalf("dims = M=%d R=%d E=%d N=%d, want 2 2 1 4", f.M, f.R, f.E, f.N)
	}
	if ir, err := f.IRAt(1, 1); err != nil || len(ir) != 4 {
		t.Fatalf("IRAt(1,1) = %v, %v; want 4 samples", ir, err)
	}
}

func TestDimProduct(t *testing.T) {
	if p, err := dimProduct(2, 3, 4); err != nil || p != 24 {
		t.Fatalf("dimProduct(2,3,4) = %d, %v", p, err)
	}
	for _, dims := range [][]int{
		{0, 1},
		{-1, -2},
		{maxDataElements, 2},
		{math.MaxInt, math.MaxInt},
	} {
		if _, err := dimProduct(dims...); err == nil {
			t.Errorf("dimProduct(%v) succeeded, want error", dims)
		}
	}
}

func TestReshapeNoAliasing(t *testing.T) {
	flat := []float64{1, 2, 3, 4, 5, 6, 7, 8}
	ir := reshapeIR(flat, 2, 2, 2)
	ir[0][0] = append(ir[0][0], 99)
	if ir[0][1][0] != 3 {
		t.Fatalf("append on row [0][0] overwrote row [0][1]: %v", ir[0][1])
	}
	flat4 := []float64{1, 2, 3, 4, 5, 6, 7, 8}
	d4 := reshape4D(flat4, 1, 2, 2, 2)
	d4[0][0][0] = append(d4[0][0][0], 99)
	if d4[0][0][1][0] != 3 {
		t.Fatalf("append on [0][0][0] overwrote [0][0][1]: %v", d4[0][0][1])
	}
}

func TestIRAccessorsOnNonFIR(t *testing.T) {
	for _, f := range []*File{robustTFFile(), robustTFEFile(), robustSOSFile()} {
		t.Run(f.DataType, func(t *testing.T) {
			// In-memory value.
			checkNoIR(t, f, ErrUnsupportedDataType)
			// After a Save/Open round trip.
			path := filepath.Join(t.TempDir(), "x.sofa")
			if err := f.Save(path); err != nil {
				t.Fatalf("Save: %v", err)
			}
			g, err := Open(path)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer g.Close()
			checkNoIR(t, g, ErrUnsupportedDataType)
		})
	}
	// Non-FIR file that nonetheless carries ImpulseResponses.
	checkNoIR(t, &File{DataType: "TF", M: 1, R: 1, N: 1, ImpulseResponses: [][][]float64{{{1}}}}, ErrUnsupportedDataType)
	// Inconsistent in-memory FIR value (dims larger than data).
	checkNoIR(t, &File{DataType: "FIR", M: 3, R: 2, N: 4, ImpulseResponses: [][][]float64{{}}}, ErrIndexOutOfRange)
}

// checkNoIR asserts that the IR accessors fail with want instead of
// panicking or returning data.
func checkNoIR(t *testing.T, f *File, want error) {
	t.Helper()
	if ir, err := f.IRAt(0, 0); !errors.Is(err, want) {
		t.Errorf("IRAt(0,0) = %v, %v; want %v", ir, err, want)
	}
	if db, err := f.IRPeakdB(0, 0); !errors.Is(err, want) {
		t.Errorf("IRPeakdB(0,0) = %v, %v; want %v", db, err, want)
	}
	if f.DataType != dataTypeFIR {
		if d, err := f.Duration(); !errors.Is(err, want) {
			t.Errorf("Duration() = %v, %v; want %v", d, err, want)
		}
	}
}

// Test fixtures built independently of other test files.

func robustBase(dataType string, m, r, e, n int) *File {
	return &File{
		M: m, R: r, E: e, N: n,
		Conventions:            "SOFA",
		Version:                "2.1",
		SOFAConventions:        "General" + strings.ReplaceAll(dataType, "-", ""),
		SOFAConventionsVersion: "1.0",
		DataType:               dataType,
		Title:                  "robustness fixture",
		ListenerPositions:      []Vector3{{0, 0, 0}},
		ReceiverPositions:      make([]Vector3, r),
		SourcePositions:        make([]Vector3, m),
		EmitterPositions:       make([]Vector3, e),
		ListenerView:           Vector3{1, 0, 0},
		ListenerUp:             Vector3{0, 0, 1},
	}
}

func ramp3D(m, r, n int, scale float64) [][][]float64 {
	out := make([][][]float64, m)
	for i := range out {
		out[i] = make([][]float64, r)
		for j := range out[i] {
			out[i][j] = make([]float64, n)
			for k := range out[i][j] {
				out[i][j][k] = scale * float64(i*100+j*10+k)
			}
		}
	}
	return out
}

func freqs(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = float64(i) * 1000
	}
	return out
}

func robustFIRFile() *File {
	f := robustBase("FIR", 2, 2, 1, 8)
	f.ImpulseResponses = ramp3D(2, 2, 8, 0.01)
	f.SamplingRate = []float64{48000}
	f.Delay = []float64{0}
	return f
}

func robustTFFile() *File {
	f := robustBase("TF", 2, 2, 1, 4)
	f.Frequencies = freqs(4)
	f.TFReal = ramp3D(2, 2, 4, 0.5)
	f.TFImag = ramp3D(2, 2, 4, -0.25)
	return f
}

func robustTFEFile() *File {
	f := robustBase("TF-E", 1, 2, 2, 3)
	f.Frequencies = freqs(3)
	mk := func(scale float64) [][][][]float64 {
		// shape [1][2][2][3]
		out := make([][][][]float64, 1)
		out[0] = make([][][]float64, 2)
		for j := range out[0] {
			out[0][j] = ramp3D(1, 2, 3, scale*float64(j+1))[0]
		}
		return out
	}
	f.TFRealE = mk(1)
	f.TFImagE = mk(-1)
	return f
}

func robustSOSFile() *File {
	f := robustBase("SOS", 3, 1, 1, 6)
	f.SOSCoefficients = ramp3D(3, 1, 6, 0.1)
	f.SamplingRate = []float64{44100}
	return f
}
