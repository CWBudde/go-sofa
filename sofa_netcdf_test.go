package sofa

import (
	"path/filepath"
	"slices"
	"testing"

	hdf5 "github.com/cwbudde/go-hdf5"
)

func TestNetcdfDimensionNAME(t *testing.T) {
	// netCDF-C: "This is a netCDF dimension but not a netCDF variable.%10d"
	for size, want := range map[int]string{
		1:    "This is a netCDF dimension but not a netCDF variable.         1",
		512:  "This is a netCDF dimension but not a netCDF variable.       512",
		1550: "This is a netCDF dimension but not a netCDF variable.      1550",
	} {
		if got := netcdfDimensionNAME(size); got != want {
			t.Errorf("netcdfDimensionNAME(%d) = %q, want %q", size, got, want)
		}
		if n, err := parseDimensionSize(netcdfDimensionNAME(size)); err != nil || n != size {
			t.Errorf("parseDimensionSize(NAME(%d)) = %d, %v", size, n, err)
		}
	}
}

// openDatasets returns the root datasets of an HDF5 file by name.
func openDatasets(t *testing.T, path string) (*hdf5.File, map[string]*hdf5.Dataset) {
	t.Helper()
	h, err := hdf5.Open(path)
	if err != nil {
		t.Fatalf("hdf5.Open: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	datasets := make(map[string]*hdf5.Dataset)
	for _, child := range h.Root().Children() {
		if ds, ok := child.(*hdf5.Dataset); ok {
			datasets[ds.Name()] = ds
		}
	}
	return h, datasets
}

// TestSaveWritesNetCDFDimensions checks the netCDF-4 structure Save emits:
// one dimension scale per SOFA dimension with length = size, NAME and
// _Netcdf4Dimid as netCDF-C writes them, and every variable attached to
// its dimensions.
func TestSaveWritesNetCDFDimensions(t *testing.T) {
	variables := map[string][]string{
		dataTypeFIR: {"Data.IR", "Data.SamplingRate", "Data.Delay"},
		dataTypeTF:  {"Data.Real", "Data.Imag"},
		dataTypeTFE: {"Data.Real", "Data.Imag"},
		dataTypeSOS: {"Data.SOS", "Data.SamplingRate"},
	}
	for _, f := range saveFixtures() {
		t.Run(f.DataType, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "out.sofa")
			if err := f.Save(path); err != nil {
				t.Fatalf("Save: %v", err)
			}
			_, datasets := openDatasets(t, path)

			sizes := map[string]int{dimM: f.M, dimR: f.R, dimE: f.E, dimN: f.N, dimC: 3, dimI: 1}
			for id, name := range ncDimOrder {
				ds, ok := datasets[name]
				if !ok {
					t.Fatalf("dimension /%s missing", name)
				}
				data, err := ds.Read()
				if err != nil {
					t.Fatalf("read /%s: %v", name, err)
				}
				if len(data) != sizes[name] {
					t.Errorf("/%s length %d, want %d", name, len(data), sizes[name])
				}
				if v, _ := ds.ReadAttribute("CLASS"); v != "DIMENSION_SCALE" {
					t.Errorf("/%s CLASS = %v", name, v)
				}
				wantNAME := netcdfDimensionNAME(sizes[name])
				if name == dimN && (f.DataType == dataTypeTF || f.DataType == dataTypeTFE) {
					wantNAME = dimN // coordinate variable
				}
				if v, _ := ds.ReadAttribute("NAME"); v != wantNAME {
					t.Errorf("/%s NAME = %q, want %q", name, v, wantNAME)
				}
				if v, _ := ds.ReadAttribute("_Netcdf4Dimid"); v != int32(id) { //nolint:gosec // G115: small index
					t.Errorf("/%s _Netcdf4Dimid = %v, want %d", name, v, id)
				}
			}

			vars := append([]string{
				"ListenerPosition", "ReceiverPosition", "SourcePosition",
				"EmitterPosition", "ListenerUp", "ListenerView",
			}, variables[f.DataType]...)
			for _, name := range vars {
				ds, ok := datasets[name]
				if !ok {
					t.Errorf("variable %s missing", name)
					continue
				}
				attrs, err := ds.ListAttributes()
				if err != nil {
					t.Fatalf("%s: list attributes: %v", name, err)
				}
				for _, want := range []string{"DIMENSION_LIST", "_Netcdf4Coordinates"} {
					if !slices.Contains(attrs, want) {
						t.Errorf("%s: missing %s (have %v)", name, want, attrs)
					}
				}
			}
		})
	}
}

// TestSaveDelayShapes checks that every accepted Data.Delay length is
// written with named dimensions and reads back unchanged.
func TestSaveDelayShapes(t *testing.T) {
	for _, n := range []int{1, 2, 3, 6} { // 1, R, M, M×R with M=3, R=2
		f := robustBase("FIR", 3, 2, 1, 4)
		f.ImpulseResponses = ramp3D(3, 2, 4, 0.1)
		f.SamplingRate = []float64{48000}
		f.Delay = make([]float64, n)
		for i := range f.Delay {
			f.Delay[i] = float64(i + 1)
		}
		path := filepath.Join(t.TempDir(), "delay.sofa")
		if err := f.Save(path); err != nil {
			t.Fatalf("Save with %d delays: %v", n, err)
		}
		g, err := Open(path)
		if err != nil {
			t.Fatalf("Open with %d delays: %v", n, err)
		}
		if !slices.Equal(g.Delay, f.Delay) {
			t.Errorf("Delay round trip: got %v, want %v", g.Delay, f.Delay)
		}
		_ = g.Close()
	}
}

// TestOpenLegacyGoSofaLayout reads the layout older go-sofa versions
// wrote: /M, /R, /E, /N as one-element datasets holding the size, with the
// size also in NAME.
func TestOpenLegacyGoSofaLayout(t *testing.T) {
	legacy := func(n int) craftedDim {
		return craftedDim{name: netcdfDimensionNAME(n), value: float64(n)}
	}
	path := writeCraftedFIR(t, map[string]craftedDim{
		"M": legacy(3), "R": legacy(2), "E": legacy(1), "N": legacy(4),
	}, 3*2*4)
	f, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = f.Close() }()
	if f.M != 3 || f.R != 2 || f.E != 1 || f.N != 4 {
		t.Errorf("dims M=%d R=%d E=%d N=%d, want 3 2 1 4", f.M, f.R, f.E, f.N)
	}
	if len(f.ImpulseResponses) != 3 || len(f.ImpulseResponses[0]) != 2 || len(f.ImpulseResponses[0][0]) != 4 {
		t.Errorf("ImpulseResponses shape wrong")
	}
}
