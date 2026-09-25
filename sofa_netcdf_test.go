package sofa

import (
	"encoding/binary"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	hdf5 "github.com/cwbudde/go-hdf5"
)

// netcdfLayout is what a written file looks like to netCDF-4: the dimension
// scales with their lengths and, per variable, the dimension names attached
// to each axis.
type netcdfLayout struct {
	scales    map[string]uint64   // scale name -> length
	variables map[string][]string // variable name -> dimension names
}

// readNetcdfLayout opens path with go-hdf5 and reconstructs the dimensions
// of every variable from the REFERENCE_LIST attributes of the scales, which
// hold (dataset object reference, dimension index) pairs inline.
func readNetcdfLayout(t *testing.T, path string) netcdfLayout {
	t.Helper()
	f, err := hdf5.Open(path)
	if err != nil {
		t.Fatalf("hdf5.Open: %v", err)
	}
	defer f.Close()

	byAddr := map[uint64]string{}
	datasets := map[string]*hdf5.Dataset{}
	f.Walk(func(p string, obj hdf5.Object) {
		if ds, ok := obj.(*hdf5.Dataset); ok {
			name := strings.TrimPrefix(p, "/")
			byAddr[ds.Address()] = name
			datasets[name] = ds
		}
	})

	layout := netcdfLayout{scales: map[string]uint64{}, variables: map[string][]string{}}
	for name, ds := range datasets {
		attrs, err := ds.Attributes()
		if err != nil {
			t.Fatalf("%s: attributes: %v", name, err)
		}
		class, _ := ds.ReadAttribute("CLASS")
		if class != "DIMENSION_SCALE" {
			// Every variable must carry a DIMENSION_LIST with one entry per axis.
			rank := -1
			for _, a := range attrs {
				if a.Name == "DIMENSION_LIST" {
					rank = int(a.Dataspace.Dimensions[0]) //nolint:gosec // small test values
				}
			}
			if rank < 0 {
				t.Errorf("variable %s has no DIMENSION_LIST (netCDF would invent phony dims)", name)
				continue
			}
			if len(layout.variables[name]) < rank {
				dims := make([]string, rank)
				copy(dims, layout.variables[name])
				layout.variables[name] = dims
			}
			continue
		}

		n, ok := datasetElementCount(ds)
		if !ok {
			t.Fatalf("scale %s: cannot determine length", name)
		}
		layout.scales[name] = n
		if _, err := ds.ReadAttribute("_Netcdf4Dimid"); err != nil {
			t.Errorf("scale %s: missing _Netcdf4Dimid: %v", name, err)
		}

		for _, a := range attrs {
			if a.Name != "REFERENCE_LIST" {
				continue
			}
			for off := 0; off+16 <= len(a.Data); off += 16 {
				varName := byAddr[binary.LittleEndian.Uint64(a.Data[off:])]
				dim := int(binary.LittleEndian.Uint32(a.Data[off+8:]))
				dims := layout.variables[varName]
				for len(dims) <= dim {
					dims = append(dims, "")
				}
				dims[dim] = name
				layout.variables[varName] = dims
			}
		}
	}
	return layout
}

func TestSaveWritesNetcdf4Dimensions(t *testing.T) {
	cases := []struct {
		name      string
		file      *File
		scales    map[string]uint64
		variables map[string][]string
	}{
		{
			name:   "FIR",
			file:   minimalFIRFile(),
			scales: map[string]uint64{"M": 3, "R": 2, "E": 1, "N": 4, "C": 3, "I": 1},
			variables: map[string][]string{
				"Data.IR":           {"M", "R", "N"},
				"Data.SamplingRate": {"I"},
				"Data.Delay":        {"M", "R"},
				"ListenerPosition":  {"I", "C"},
				"ReceiverPosition":  {"R", "C"},
				"SourcePosition":    {"M", "C"},
				"EmitterPosition":   {"E", "C"},
				"ListenerUp":        {"I", "C"},
				"ListenerView":      {"I", "C"},
			},
		},
		{
			name:   "TF",
			file:   minimalTFFile(),
			scales: map[string]uint64{"M": 2, "R": 1, "E": 1, "N": 3, "C": 3, "I": 1},
			variables: map[string][]string{
				"Data.Real":    {"M", "R", "N"},
				"Data.Imag":    {"M", "R", "N"},
				"ListenerUp":   {"I", "C"},
				"ListenerView": {"I", "C"},
			},
		},
		{
			name:   "TF-E",
			file:   minimalTFEFile(),
			scales: map[string]uint64{"M": 2, "R": 1, "E": 2, "N": 3, "C": 3, "I": 1},
			variables: map[string][]string{
				"Data.Real":    {"M", "R", "N", "E"},
				"Data.Imag":    {"M", "R", "N", "E"},
				"ListenerUp":   {"I", "C"},
				"ListenerView": {"I", "C"},
			},
		},
		{
			name:   "SOS",
			file:   minimalSOSFile(),
			scales: map[string]uint64{"M": 1, "R": 1, "E": 1, "N": 6, "C": 3, "I": 1},
			variables: map[string][]string{
				"Data.SOS":          {"M", "R", "N"},
				"Data.SamplingRate": {"M"},
				"Data.Delay":        {"I", "R"}, // mandatory; zeros when absent
				"ListenerUp":        {"I", "C"},
				"ListenerView":      {"I", "C"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "out.sofa")
			if err := tc.file.Save(path); err != nil {
				t.Fatalf("Save: %v", err)
			}

			got := readNetcdfLayout(t, path)
			if !reflect.DeepEqual(got.scales, tc.scales) {
				t.Errorf("dimension scales = %v, want %v", got.scales, tc.scales)
			}
			if !reflect.DeepEqual(got.variables, tc.variables) {
				t.Errorf("variable dimensions = %v, want %v", got.variables, tc.variables)
			}

			// The file must still read back through the public API.
			back, err := Open(path)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer back.Close()
			if back.M != tc.file.M || back.R != tc.file.R || back.E != tc.file.E || back.N != tc.file.N {
				t.Errorf("dims after round trip M=%d R=%d E=%d N=%d, want M=%d R=%d E=%d N=%d",
					back.M, back.R, back.E, back.N, tc.file.M, tc.file.R, tc.file.E, tc.file.N)
			}
		})
	}
}

func TestSaveWritesNCProperties(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.sofa")
	if err := minimalFIRFile().Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	f, err := hdf5.Open(path)
	if err != nil {
		t.Fatalf("hdf5.Open: %v", err)
	}
	defer f.Close()
	v, err := f.Root().ReadAttribute("_NCProperties")
	if err != nil {
		t.Fatalf("_NCProperties: %v", err)
	}
	s, _ := v.(string)
	if !strings.HasPrefix(s, "version=2,") || !strings.Contains(s, "go-sofa=") {
		t.Errorf("_NCProperties = %q, want version=2,go-sofa=...", s)
	}
}

// minimalFIRFile has distinct M, R and E so every dimension is identifiable,
// one row per measurement for SourcePosition, and an M×R delay.
func minimalFIRFile() *File {
	const M, R, E, N = 3, 2, 1, 4
	ir := make([][][]float64, M)
	for m := range ir {
		ir[m] = [][]float64{make([]float64, N), make([]float64, N)}
	}
	return &File{
		Conventions:            "SOFA",
		Version:                "2.1",
		SOFAConventions:        "SimpleFreeFieldHRIR",
		SOFAConventionsVersion: "1.0",
		DataType:               "FIR",
		M:                      M, R: R, E: E, N: N,
		ImpulseResponses:  ir,
		SamplingRate:      []float64{48000},
		Delay:             []float64{0, 1, 2, 3, 4, 5},
		ListenerPositions: []Vector3{{0, 0, 0}},
		ReceiverPositions: []Vector3{{0, 0.09, 0}, {0, -0.09, 0}},
		SourcePositions:   []Vector3{{0, 0, 1}, {90, 0, 1}, {180, 0, 1}},
		EmitterPositions:  []Vector3{{0, 0, 0}},
		ListenerUp:        Vector3{0, 0, 1},
		ListenerView:      Vector3{1, 0, 0},

		ListenerPositionType: CoordinateCartesian,
		ReceiverPositionType: CoordinateCartesian,
		SourcePositionType:   CoordinateSpherical,
		EmitterPositionType:  CoordinateCartesian,
	}
}

func TestDelayDims(t *testing.T) {
	for _, tc := range []struct {
		n, m, r int
		want    []string
	}{
		{1, 3, 2, []string{"I"}},
		{3, 3, 2, []string{"M"}},
		{2, 3, 2, []string{"R"}},
		{6, 3, 2, []string{"M", "R"}},
		{3, 3, 1, []string{"M", "R"}}, // M×R with R=1 keeps its 2-D shape
		{2, 1, 2, []string{"M", "R"}}, // M×R with M=1
		{1, 1, 1, []string{"I"}},
	} {
		if got := delayDims(tc.n, tc.m, tc.r); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("delayDims(n=%d, M=%d, R=%d) = %v, want %v", tc.n, tc.m, tc.r, got, tc.want)
		}
	}
}
