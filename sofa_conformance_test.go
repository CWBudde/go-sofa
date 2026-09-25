package sofa

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	hdf5 "github.com/cwbudde/go-hdf5"
)

// readRawDataset returns the flat values and shape of dataset name in path,
// read with go-hdf5 directly rather than through Open.
func readRawDataset(t *testing.T, path, name string) ([]float64, []uint64) {
	t.Helper()
	h, err := hdf5.Open(path)
	if err != nil {
		t.Fatalf("hdf5.Open: %v", err)
	}
	defer h.Close()
	for _, child := range h.Root().Children() {
		ds, ok := child.(*hdf5.Dataset)
		if !ok || ds.Name() != name {
			continue
		}
		shape, ok := datasetShape(ds)
		if !ok {
			t.Fatalf("%s: cannot determine shape", name)
		}
		data, err := ds.Read()
		if err != nil {
			t.Fatalf("%s: read: %v", name, err)
		}
		return data, shape
	}
	t.Fatalf("dataset %s not found", name)
	return nil, nil
}

// TestSaveTFEAxisOrder checks that TF-E data is written [M,R,N,E], the order
// of the GeneralTF-E and FreeFieldHRTF convention tables ("mrne"), and reads
// back unchanged.
func TestSaveTFEAxisOrder(t *testing.T) {
	f := minimalTFEFile()
	for m := range f.M {
		for r := range f.R {
			for e := range f.E {
				for n := range f.N {
					f.TFRealE[m][r][e][n] = float64(1000*m + 100*r + 10*e + n)
					f.TFImagE[m][r][e][n] = -float64(1000*m + 100*r + 10*e + n)
				}
			}
		}
	}
	path := filepath.Join(t.TempDir(), "tfe.sofa")
	if err := f.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	layout := readNetcdfLayout(t, path)
	for _, name := range []string{"Data.Real", "Data.Imag"} {
		if got, want := layout.variables[name], []string{"M", "R", "N", "E"}; !reflect.DeepEqual(got, want) {
			t.Errorf("%s dimensions = %v, want %v", name, got, want)
		}
	}

	for name, want := range map[string][][][][]float64{"Data.Real": f.TFRealE, "Data.Imag": f.TFImagE} {
		raw, shape := readRawDataset(t, path, name)
		wantShape := []uint64{uint64(f.M), uint64(f.R), uint64(f.N), uint64(f.E)} //nolint:gosec // small test values
		if !reflect.DeepEqual(shape, wantShape) {
			t.Fatalf("%s shape = %v, want %v", name, shape, wantShape)
		}
		for m := range f.M {
			for r := range f.R {
				for n := range f.N {
					for e := range f.E {
						got := raw[((m*f.R+r)*f.N+n)*f.E+e]
						if got != want[m][r][e][n] {
							t.Fatalf("%s[m=%d,r=%d,n=%d,e=%d] = %v, want %v", name, m, r, n, e, got, want[m][r][e][n])
						}
					}
				}
			}
		}
	}

	back, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer back.Close()
	if !reflect.DeepEqual(back.TFRealE, f.TFRealE) || !reflect.DeepEqual(back.TFImagE, f.TFImagE) {
		t.Errorf("TF-E data changed in the round trip")
	}
}

// TestSaveDelayLayouts checks that Data.Delay is always written 2-D, [I,R]
// or [M,R] as AES69 requires for FIR and SOS, expanding the 1-D layouts
// go-sofa accepts in memory, that an absent delay is written as zeros, and
// that DelayAt answers the same before and after the round trip.
func TestSaveDelayLayouts(t *testing.T) {
	for _, tc := range []struct {
		name     string
		m, r     int
		delay    []float64
		wantDims []string
		want     []float64
	}{
		{"absent", 3, 2, nil, []string{"I", "R"}, []float64{0, 0}},
		{"scalar I", 3, 2, []float64{5}, []string{"I", "R"}, []float64{5, 5}},
		{"per receiver R", 3, 2, []float64{1, 2}, []string{"I", "R"}, []float64{1, 2}},
		{"per measurement M", 3, 2, []float64{1, 2, 3}, []string{"M", "R"}, []float64{1, 1, 2, 2, 3, 3}},
		{"M×R", 3, 2, []float64{1, 2, 3, 4, 5, 6}, []string{"M", "R"}, []float64{1, 2, 3, 4, 5, 6}},
		{"M==R reads as M", 2, 2, []float64{1, 2}, []string{"M", "R"}, []float64{1, 1, 2, 2}},
		{"R=1 per measurement", 3, 1, []float64{1, 2, 3}, []string{"M", "R"}, []float64{1, 2, 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := minimalFIRFile()
			f.SOFAConventions = "GeneralFIR" // SimpleFreeFieldHRIR requires R=2
			f.M, f.R = tc.m, tc.r
			f.ImpulseResponses = make([][][]float64, tc.m)
			for m := range f.ImpulseResponses {
				f.ImpulseResponses[m] = make([][]float64, tc.r)
				for r := range f.ImpulseResponses[m] {
					f.ImpulseResponses[m][r] = make([]float64, f.N)
				}
			}
			f.ReceiverPositions = f.ReceiverPositions[:tc.r]
			f.SourcePositions = f.SourcePositions[:1]
			f.Delay = tc.delay

			path := filepath.Join(t.TempDir(), "delay.sofa")
			if err := f.Save(path); err != nil {
				t.Fatalf("Save: %v", err)
			}
			if got := readNetcdfLayout(t, path).variables["Data.Delay"]; !reflect.DeepEqual(got, tc.wantDims) {
				t.Errorf("Data.Delay dimensions = %v, want %v", got, tc.wantDims)
			}
			if raw, _ := readRawDataset(t, path, "Data.Delay"); !reflect.DeepEqual(raw, tc.want) {
				t.Errorf("Data.Delay values = %v, want %v", raw, tc.want)
			}

			back, err := Open(path)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer back.Close()
			for m := range tc.m {
				for r := range tc.r {
					want, err := f.DelayAt(m, r)
					if err != nil {
						t.Fatalf("DelayAt(%d, %d) before Save: %v", m, r, err)
					}
					got, err := back.DelayAt(m, r)
					if err != nil || got != want {
						t.Errorf("DelayAt(%d, %d) after round trip = %v, %v; want %v", m, r, got, err, want)
					}
				}
			}
		})
	}
}

// TestSavePerMeasurementLayouts checks that the per-measurement fields Open
// fills are written in their AES69 layouts ([R,C,M], [E,C,M], [M,C]) and
// read back unchanged, with the singular fields holding measurement 0.
func TestSavePerMeasurementLayouts(t *testing.T) {
	f := minimalFIRFile() // M=3, R=2, E=1
	for m := range f.M {
		mf := float64(m)
		f.ReceiverPositionsM = append(f.ReceiverPositionsM,
			[]Vector3{{mf, 0.09, 0}, {mf, -0.09, 0}})
		f.EmitterPositionsM = append(f.EmitterPositionsM, []Vector3{{0, 0, mf}})
		f.ListenerViews = append(f.ListenerViews, Vector3{1, mf, 0})
		f.ListenerUps = append(f.ListenerUps, Vector3{0, mf, 1})
	}

	path := filepath.Join(t.TempDir(), "perm.sofa")
	if err := f.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	layout := readNetcdfLayout(t, path)
	for name, want := range map[string][]string{
		"ReceiverPosition": {"R", "C", "M"},
		"EmitterPosition":  {"E", "C", "M"},
		"ListenerView":     {"M", "C"},
		"ListenerUp":       {"M", "C"},
	} {
		if got := layout.variables[name]; !reflect.DeepEqual(got, want) {
			t.Errorf("%s dimensions = %v, want %v", name, got, want)
		}
	}

	back, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer back.Close()
	if !reflect.DeepEqual(back.ReceiverPositionsM, f.ReceiverPositionsM) {
		t.Errorf("ReceiverPositionsM = %v, want %v", back.ReceiverPositionsM, f.ReceiverPositionsM)
	}
	if !reflect.DeepEqual(back.EmitterPositionsM, f.EmitterPositionsM) {
		t.Errorf("EmitterPositionsM = %v, want %v", back.EmitterPositionsM, f.EmitterPositionsM)
	}
	if !reflect.DeepEqual(back.ListenerViews, f.ListenerViews) {
		t.Errorf("ListenerViews = %v, want %v", back.ListenerViews, f.ListenerViews)
	}
	if !reflect.DeepEqual(back.ListenerUps, f.ListenerUps) {
		t.Errorf("ListenerUps = %v, want %v", back.ListenerUps, f.ListenerUps)
	}
	if !reflect.DeepEqual(back.ReceiverPositions, f.ReceiverPositionsM[0]) || back.ListenerView != f.ListenerViews[0] {
		t.Errorf("singular fields = %v, %v; want measurement 0", back.ReceiverPositions, back.ListenerView)
	}
}

func TestValidatePerMeasurementLayouts(t *testing.T) {
	for name, mutate := range map[string]func(*File){
		"ReceiverPositionsM rows":  func(f *File) { f.ReceiverPositionsM = [][]Vector3{{{}, {}}} },
		"ReceiverPositionsM width": func(f *File) { f.ReceiverPositionsM = [][]Vector3{{{}}, {{}, {}}, {{}}} },
		"EmitterPositionsM width":  func(f *File) { f.EmitterPositionsM = [][]Vector3{{{}, {}}, {{}, {}}, {{}, {}}} },
		"ListenerViews length":     func(f *File) { f.ListenerViews = []Vector3{{1, 0, 0}} },
		"ListenerUps length":       func(f *File) { f.ListenerUps = []Vector3{{}, {}} },
	} {
		t.Run(name, func(t *testing.T) {
			f := minimalFIRFile()
			mutate(f)
			if err := f.validate(); err == nil {
				t.Errorf("validate accepted %s", name)
			}
		})
	}
}

// TestSaveOfficeIIRoundTrip re-saves a third-party file with [M,C]
// ListenerView and checks the per-measurement orientation survives.
func TestSaveOfficeIIRoundTrip(t *testing.T) {
	f, err := Open(testdataPath(t, "OfficeII.sofa"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	path := filepath.Join(t.TempDir(), "office.sofa")
	if err := f.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	back, err := Open(path)
	if err != nil {
		t.Fatalf("Open re-saved: %v", err)
	}
	defer back.Close()
	for name, pair := range map[string][2]interface{}{
		"ListenerViews":      {back.ListenerViews, f.ListenerViews},
		"ListenerUps":        {back.ListenerUps, f.ListenerUps},
		"ReceiverPositionsM": {back.ReceiverPositionsM, f.ReceiverPositionsM},
		"EmitterPositionsM":  {back.EmitterPositionsM, f.EmitterPositionsM},
	} {
		if !reflect.DeepEqual(pair[0], pair[1]) {
			t.Errorf("%s changed in the round trip", name)
		}
	}
}

// readDatasetAttr returns attribute attr of dataset name in path, or nil
// when either is absent.
func readDatasetAttr(t *testing.T, path, name, attr string) interface{} {
	t.Helper()
	h, err := hdf5.Open(path)
	if err != nil {
		t.Fatalf("hdf5.Open: %v", err)
	}
	defer h.Close()
	for _, child := range h.Root().Children() {
		ds, ok := child.(*hdf5.Dataset)
		if !ok || ds.Name() != name {
			continue
		}
		v, err := ds.ReadAttribute(attr)
		if err != nil {
			return nil
		}
		return v
	}
	return nil
}

// TestSaveVariableAttributes checks the variable attributes the convention
// tables make mandatory.
func TestSaveVariableAttributes(t *testing.T) {
	cases := []struct {
		name string
		file *File
		want map[[2]string]string // {variable, attribute} -> value
	}{
		{"FIR", minimalFIRFile(), map[[2]string]string{
			{"Data.SamplingRate", "Units"}: "hertz",
			{"ListenerView", "Type"}:       "cartesian",
			{"ListenerView", "Units"}:      "metre",
			{"ListenerUp", "Type"}:         "cartesian",
			{"ListenerUp", "Units"}:        "metre",
			{"SourcePosition", "Type"}:     "spherical",
		}},
		{"TF", minimalTFFile(), map[[2]string]string{
			{"N", "Units"}:    "hertz",
			{"N", "LongName"}: "frequency",
		}},
		{"TF-E", minimalTFEFile(), map[[2]string]string{
			{"N", "Units"}:    "hertz",
			{"N", "LongName"}: "frequency",
		}},
		{"SOS", minimalSOSFile(), map[[2]string]string{
			{"Data.SamplingRate", "Units"}: "hertz",
		}},
		{"spherical view", func() *File {
			f := minimalFIRFile()
			setSphericalOrientation(f)
			f.ListenerViewUnits = UnitsSphericalDegrees
			return f
		}(), map[[2]string]string{
			{"ListenerView", "Type"}:  "spherical",
			{"ListenerView", "Units"}: UnitsSphericalDegrees,
			{"ListenerUp", "Type"}:    "spherical",
		}},
		{"spherical view without units", func() *File {
			f := minimalFIRFile()
			setSphericalOrientation(f)
			return f
		}(), map[[2]string]string{
			{"ListenerView", "Units"}: UnitsSphericalDegrees,
			{"ListenerUp", "Units"}:   UnitsSphericalDegrees,
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "attrs.sofa")
			if err := tc.file.Save(path); err != nil {
				t.Fatalf("Save: %v", err)
			}
			for key, want := range tc.want {
				if got := readDatasetAttr(t, path, key[0], key[1]); got != want {
					t.Errorf("%s:%s = %v, want %q", key[0], key[1], got, want)
				}
			}
		})
	}

	// Open reads the ListenerView coordinate system back.
	f := minimalFIRFile()
	setSphericalOrientation(f)
	f.ListenerViewUnits = UnitsSphericalDegrees
	path := filepath.Join(t.TempDir(), "view.sofa")
	if err := f.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	back, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer back.Close()
	if back.ListenerViewType != CoordinateSpherical || back.ListenerViewUnits != UnitsSphericalDegrees {
		t.Errorf("ListenerView Type/Units = %q/%q after round trip", back.ListenerViewType, back.ListenerViewUnits)
	}
}

// TestValidateRejectsPositionType checks that every written position must
// name its coordinate system with an AES69 Type.
func TestValidateRejectsPositionType(t *testing.T) {
	for name, mutate := range map[string]func(*File){
		"missing ListenerPosition Type": func(f *File) { f.ListenerPositionType = "" },
		"missing ReceiverPosition Type": func(f *File) { f.ReceiverPositionType = "" },
		"missing SourcePosition Type":   func(f *File) { f.SourcePositionType = "" },
		"missing EmitterPosition Type":  func(f *File) { f.EmitterPositionType = "" },
		"unknown SourcePosition Type":   func(f *File) { f.SourcePositionType = "polar" },
		"unknown ListenerView Type":     func(f *File) { f.ListenerViewType = "polar" },
	} {
		t.Run(name, func(t *testing.T) {
			f := minimalFIRFile()
			mutate(f)
			if err := f.validate(); err == nil {
				t.Errorf("validate accepted %s", name)
			}
		})
	}

	// A position that is not written needs no Type.
	f := minimalFIRFile()
	f.EmitterPositions, f.EmitterPositionType = nil, ""
	if err := f.validate(); err != nil {
		t.Errorf("validate without EmitterPositions: %v", err)
	}
	// Type is compared case-insensitively, as Open lowercases it.
	f = minimalFIRFile()
	f.SourcePositionType = "Spherical"
	if err := f.validate(); err != nil {
		t.Errorf("validate with Type %q: %v", f.SourcePositionType, err)
	}
}

// TestSaveMandatoryGlobalAttributes checks that the global attributes the
// conventions make mandatory are always written, with defaults for empty
// fields, and that Save does not change the File.
func TestSaveMandatoryGlobalAttributes(t *testing.T) {
	readRoot := func(t *testing.T, path string) map[string]interface{} {
		t.Helper()
		h, err := hdf5.Open(path)
		if err != nil {
			t.Fatalf("hdf5.Open: %v", err)
		}
		defer h.Close()
		attrs, err := h.Root().Attributes()
		if err != nil {
			t.Fatalf("root attributes: %v", err)
		}
		out := map[string]interface{}{}
		for _, a := range attrs {
			v, err := a.ReadValue()
			if err != nil {
				t.Fatalf("attribute %s: %v", a.Name, err)
			}
			out[a.Name] = v
		}
		return out
	}
	// Pin the clock; a local time must be written as UTC.
	orig := saveTime
	saveTime = func() time.Time { return time.Date(2026, 9, 25, 14, 30, 0, 0, time.FixedZone("CEST", 2*3600)) }
	t.Cleanup(func() { saveTime = orig })

	t.Run("defaults", func(t *testing.T) {
		f := minimalFIRFile() // sets none of the attributes below
		path := filepath.Join(t.TempDir(), "defaults.sofa")
		if err := f.Save(path); err != nil {
			t.Fatalf("Save: %v", err)
		}
		got := readRoot(t, path)
		for name, want := range map[string]string{
			"APIName":       "go-sofa",
			"License":       "No license provided, ask the author for permission",
			"RoomType":      "free field",
			"Title":         "",
			"AuthorContact": "",
			"Organization":  "",
		} {
			if v, ok := got[name]; !ok || v != want {
				t.Errorf("%s = %v (present %v), want %q", name, v, ok, want)
			}
		}
		if v, _ := got["APIVersion"].(string); v == "" {
			t.Errorf("APIVersion = %q, want the go-sofa version", v)
		}
		for _, name := range []string{"DateCreated", "DateModified"} {
			if v, _ := got[name].(string); v != "2026-09-25 12:30:00" {
				t.Errorf("%s = %q, want the save time in UTC, 2026-09-25 12:30:00", name, v)
			}
		}
		if f.APIName != "" || f.DateCreated != "" || f.License != "" || f.RoomType != "" {
			t.Errorf("Save changed the File: APIName=%q DateCreated=%q License=%q RoomType=%q",
				f.APIName, f.DateCreated, f.License, f.RoomType)
		}
	})

	t.Run("set values kept", func(t *testing.T) {
		f := minimalFIRFile()
		f.APIName, f.APIVersion = "MyTool", "3.1"
		f.DateCreated, f.DateModified = "2020-01-02 03:04:05", "2021-01-02 03:04:05"
		f.License, f.RoomType, f.Title = "CC-BY-4.0", "reverberant", "T"
		f.AuthorContact, f.Organization = "a@example.org", "Org"
		path := filepath.Join(t.TempDir(), "set.sofa")
		if err := f.Save(path); err != nil {
			t.Fatalf("Save: %v", err)
		}
		got := readRoot(t, path)
		for name, want := range map[string]string{
			"APIName": f.APIName, "APIVersion": f.APIVersion,
			"DateCreated": f.DateCreated, "DateModified": f.DateModified,
			"License": f.License, "RoomType": f.RoomType, "Title": f.Title,
			"AuthorContact": f.AuthorContact, "Organization": f.Organization,
		} {
			if got[name] != want {
				t.Errorf("%s = %v, want %q", name, got[name], want)
			}
		}
	})
}

func TestValidateRequiresConventionsVersion(t *testing.T) {
	f := minimalFIRFile()
	f.SOFAConventionsVersion = ""
	if err := f.validate(); err == nil {
		t.Error("validate accepted an empty SOFAConventionsVersion")
	}
}
