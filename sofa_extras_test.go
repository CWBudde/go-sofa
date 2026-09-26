package sofa

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// roundTrip saves f to a temporary file and opens it again.
func roundTrip(t *testing.T, f *File) *File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "roundtrip.sofa")
	if err := f.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	back, err := Open(path)
	if err != nil {
		t.Fatalf("Open re-saved: %v", err)
	}
	t.Cleanup(func() { _ = back.Close() })
	return back
}

// TestRoundTripPreservesExtras checks that global attributes, variables and
// variable attributes go-sofa does not interpret survive Open → Save → Open.
func TestRoundTripPreservesExtras(t *testing.T) {
	for _, tc := range []struct {
		file       string
		attributes []string // extra global attributes the file must yield
		variables  []string // extra variables the file must yield
	}{
		{"Mesh2HRTF.sofa", []string{"DatabaseName", "ListenerShortName"}, []string{"SourceUp", "SourceView"}},
		{"CIPIC_subject_003_hrir_final.sofa", []string{"DatabaseName", "ListenerShortName"}, nil},
		{"OfficeII.sofa", []string{"DatabaseName", "RoomDescription"}, []string{"SourceView", "RoomCornerA", "RoomCornerB"}},
		{"SingleRoomSRIR_1.1.sofa", []string{"DatabaseName"}, []string{"ReceiverDescriptions", "RoomCorners"}},
	} {
		t.Run(tc.file, func(t *testing.T) {
			f, err := Open(testdataPath(t, tc.file))
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer f.Close()
			if len(f.Dropped) > 0 {
				t.Errorf("Dropped = %q, want nothing", f.Dropped)
			}
			for _, name := range tc.attributes {
				if !hasAttribute(f.Attributes, name) {
					t.Errorf("Attributes lack %s: %v", name, f.Attributes)
				}
			}
			for _, name := range tc.variables {
				if findVariable(f.Variables, name) == nil {
					t.Errorf("Variables lack %s", name)
				}
			}

			back := roundTrip(t, f)
			if !reflect.DeepEqual(back.Attributes, f.Attributes) {
				t.Errorf("Attributes changed:\n got %v\nwant %v", back.Attributes, f.Attributes)
			}
			if !reflect.DeepEqual(back.Variables, f.Variables) {
				t.Errorf("Variables changed:\n got %+v\nwant %+v", back.Variables, f.Variables)
			}
			if !reflect.DeepEqual(back.VariableAttributes, f.VariableAttributes) {
				t.Errorf("VariableAttributes changed:\n got %v\nwant %v", back.VariableAttributes, f.VariableAttributes)
			}
			if len(back.Dropped) > 0 {
				t.Errorf("Dropped after round trip = %q", back.Dropped)
			}
		})
	}
}

// TestOpenEmptyGlobalAttributes checks that attributes stored with a null
// dataspace (the SOFA Toolbox's empty strings) read as "", not as "[]".
func TestOpenEmptyGlobalAttributes(t *testing.T) {
	f, err := Open(testdataPath(t, "CIPIC_subject_003_hrir_final.sofa"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	for name, got := range map[string]string{
		"Title": f.Title, "Comment": f.Comment, "Organization": f.Organization,
		"AuthorContact": f.AuthorContact, "Origin": f.Origin, "References": f.References,
	} {
		if got != "" {
			t.Errorf("%s = %q, want empty", name, got)
		}
	}
}

// TestRoundTripSyntheticExtras checks extras set by the caller: attributes
// on known variables, a variable with a dimension go-sofa does not use, and
// a variable without dimension names.
func TestRoundTripSyntheticExtras(t *testing.T) {
	f := minimalFIRFile()
	f.Attributes = []Attribute{{"DatabaseName", "synthetic"}, {"Count", int32(7)}}
	f.VariableAttributes = map[string][]Attribute{
		"Data.IR":        {{"ChannelOrdering", "acn"}},
		"SourcePosition": {{"Description", "loudspeaker ring"}},
		"ListenerView":   {{"Comment", "forward"}},
	}
	f.Variables = []Variable{
		{
			Name: "SourceView", Dims: []string{"I", "C"}, Shape: []int{1, 3},
			Values: []float64{1, 0, 0}, Attributes: []Attribute{{"Type", "Cartesian"}, {"Units", "Metre"}},
		},
		{
			Name: "ReceiverDescriptions", Dims: []string{"R", "S"}, Shape: []int{f.R, 4},
			Chars: []byte("left" + "rt\x00\x00"),
		},
		{Name: "Unlabelled", Shape: []int{2, 2}, Values: []float64{1, 2, 3, 4}},
	}
	back := roundTrip(t, f)
	// Open returns both lists sorted by name.
	wantAttrs := []Attribute{f.Attributes[1], f.Attributes[0]}
	wantVars := []Variable{f.Variables[1], f.Variables[0], f.Variables[2]}
	if !reflect.DeepEqual(back.Attributes, wantAttrs) {
		t.Errorf("Attributes = %v, want %v", back.Attributes, wantAttrs)
	}
	if !reflect.DeepEqual(back.VariableAttributes, f.VariableAttributes) {
		t.Errorf("VariableAttributes = %v, want %v", back.VariableAttributes, f.VariableAttributes)
	}
	if !reflect.DeepEqual(back.Variables, wantVars) {
		t.Errorf("Variables:\n got %+v\nwant %+v", back.Variables, wantVars)
	}
}

// TestRoundTripManyVariableAttributes saves variables with more than eight
// attributes, which go-hdf5 keeps in dense storage, and reads
// them back with Open and OpenLazy. `just interop` checks the same with
// h5py and netCDF4 (internal/interop/gen, extras.sofa).
func TestRoundTripManyVariableAttributes(t *testing.T) {
	notes := func(prefix string, n int) []Attribute {
		attrs := make([]Attribute, n)
		for i := range attrs {
			attrs[i] = Attribute{fmt.Sprintf("Note%02d", i), fmt.Sprintf("%s %d", prefix, i)}
		}
		return attrs
	}
	f := minimalFIRFile()
	f.VariableAttributes = map[string][]Attribute{
		"Data.IR":        notes("ir", 12),
		"SourcePosition": append(notes("source", 9), Attribute{"Scale", 2.5}),
	}
	f.Variables = []Variable{{
		Name: "SourceView", Dims: []string{"I", "C"}, Shape: []int{1, 3}, Values: []float64{1, 0, 0},
		Attributes: append([]Attribute{{"Type", "cartesian"}, {"Units", "metre"}}, notes("view", 9)...),
	}}
	path := saveTemp(t, f, "many_attributes.sofa")

	back, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for name, want := range f.VariableAttributes {
		if got := back.VariableAttributes[name]; !sameAttributes(got, want) {
			t.Errorf("VariableAttributes[%s] = %v, want %v", name, got, want)
		}
	}
	if len(back.Variables) != 1 || !sameAttributes(back.Variables[0].Attributes, f.Variables[0].Attributes) {
		t.Errorf("Variables = %+v, want the attributes %v", back.Variables, f.Variables[0].Attributes)
	}
	if len(back.Dropped) > 0 {
		t.Errorf("Dropped = %q", back.Dropped)
	}

	lazy := openLazy(t, path)
	for m := range f.M {
		ir, err := lazy.ReadMeasurement(m)
		if err != nil {
			t.Fatalf("ReadMeasurement(%d): %v", m, err)
		}
		if !reflect.DeepEqual(ir, f.ImpulseResponses[m]) {
			t.Errorf("measurement %d = %v, want %v", m, ir, f.ImpulseResponses[m])
		}
	}
}

// sameAttributes reports whether a and b hold the same attributes in any
// order (Open sorts them by name).
func sameAttributes(a, b []Attribute) bool {
	if len(a) != len(b) {
		return false
	}
	byName := make(map[string]any, len(a))
	for _, x := range a {
		byName[x.Name] = x.Value
	}
	for _, x := range b {
		v, ok := byName[x.Name]
		if !ok || !reflect.DeepEqual(v, x.Value) {
			return false
		}
	}
	return true
}

// TestRoundTripDimensionAttributes checks that further attributes of the
// dimension scales Save writes, the TF frequency axis N included, survive a
// round trip next to the LongName and Units Save sets on N itself.
func TestRoundTripDimensionAttributes(t *testing.T) {
	f := minimalTFFile()
	f.VariableAttributes = map[string][]Attribute{
		"N": {{"Comment", "linear grid"}},
		"M": {{"Description", "measurements"}},
	}
	back := roundTrip(t, f)
	if !reflect.DeepEqual(back.VariableAttributes, f.VariableAttributes) {
		t.Errorf("VariableAttributes = %v, want %v", back.VariableAttributes, f.VariableAttributes)
	}
	if len(back.Dropped) > 0 {
		t.Errorf("Dropped = %q", back.Dropped)
	}
}

// TestTypeUnitsKeepCase checks that Open keeps the case of Type and Units
// and that coordinate-system detection still ignores it.
func TestTypeUnitsKeepCase(t *testing.T) {
	f := minimalFIRFile()
	f.SourcePositionType = "Spherical"
	f.SourcePositionUnits = "Degree, Degree, Metre"
	f.ListenerViewType = "Spherical"
	f.ListenerViewUnits = "Radian, Radian, Metre"
	f.ListenerView = Vector3{}
	f.ListenerUp = Vector3{}
	back := roundTrip(t, f)
	if back.SourcePositionType != "Spherical" || back.SourcePositionUnits != "Degree, Degree, Metre" {
		t.Errorf("SourcePosition Type/Units = %q/%q, want the original case",
			back.SourcePositionType, back.SourcePositionUnits)
	}
	if back.ListenerViewType != "Spherical" || back.ListenerViewUnits != "Radian, Radian, Metre" {
		t.Errorf("ListenerView Type/Units = %q/%q, want the original case",
			back.ListenerViewType, back.ListenerViewUnits)
	}
	// The spherical, radian default up vector proves both were recognised.
	if want := (Vector3{0, 1.5707963267948966, 1}); back.ListenerUp != want {
		t.Errorf("ListenerUp = %v, want %v", back.ListenerUp, want)
	}
}

// TestValidateRejectsBadExtras checks the rules Save applies to extras.
func TestValidateRejectsBadExtras(t *testing.T) {
	for _, tc := range []struct {
		name  string
		set   func(f *File)
		match string
	}{
		{"global shadows a field", func(f *File) { f.Attributes = []Attribute{{"Title", "x"}} }, "Title"},
		{"reserved global", func(f *File) { f.Attributes = []Attribute{{"_NCProperties", "x"}} }, "_NCProperties"},
		{"duplicate global", func(f *File) { f.Attributes = []Attribute{{"A", "x"}, {"A", "y"}} }, "A"},
		{"unsupported value", func(f *File) { f.Attributes = []Attribute{{"A", struct{}{}}} }, "A"},
		{"variable shadows a written one", func(f *File) {
			f.Variables = []Variable{{Name: "Data.IR", Dims: []string{"I"}, Shape: []int{1}, Values: []float64{1}}}
		}, "Data.IR"},
		{"variable named like a dimension", func(f *File) {
			f.Variables = []Variable{{Name: "M", Dims: []string{"I"}, Shape: []int{1}, Values: []float64{1}}}
		}, "M"},
		{"duplicate variable", func(f *File) {
			v := Variable{Name: "X", Dims: []string{"I"}, Shape: []int{1}, Values: []float64{1}}
			f.Variables = []Variable{v, v}
		}, "X"},
		{"wrong value count", func(f *File) {
			f.Variables = []Variable{{Name: "X", Dims: []string{"I", "C"}, Shape: []int{1, 3}, Values: []float64{1}}}
		}, "X"},
		{"values and chars", func(f *File) {
			f.Variables = []Variable{{Name: "X", Shape: []int{1}, Values: []float64{1}, Chars: []byte("a")}}
		}, "X"},
		{"dims and shape disagree", func(f *File) {
			f.Variables = []Variable{{Name: "X", Dims: []string{"I"}, Shape: []int{1, 3}, Values: make([]float64, 3)}}
		}, "X"},
		{"size differs from the file", func(f *File) {
			f.Variables = []Variable{{Name: "X", Dims: []string{"M"}, Shape: []int{f.M + 1}, Values: make([]float64, f.M+1)}}
		}, "M"},
		{"inconsistent extra dimension", func(f *File) {
			f.Variables = []Variable{
				{Name: "X", Dims: []string{"S"}, Shape: []int{2}, Chars: []byte("ab")},
				{Name: "Y", Dims: []string{"S"}, Shape: []int{3}, Chars: []byte("abc")},
			}
		}, "S"},
		{"new dimension named like a written variable", func(f *File) {
			f.Variables = []Variable{{Name: "X", Dims: []string{"Data.IR"}, Shape: []int{2}, Values: []float64{1, 2}}}
		}, "Data.IR"},
		{"attribute Save owns on the frequency axis", func(f *File) {
			*f = *minimalTFFile()
			f.VariableAttributes = map[string][]Attribute{"N": {{"Units", "Hz"}}}
		}, "Units"},
		{"attributes of an unwritten variable", func(f *File) {
			f.VariableAttributes = map[string][]Attribute{"Data.Real": {{"A", "x"}}}
		}, "Data.Real"},
		{"attribute Save owns", func(f *File) {
			f.VariableAttributes = map[string][]Attribute{"SourcePosition": {{"Type", "cartesian"}}}
		}, "Type"},
		{"netCDF plumbing attribute", func(f *File) {
			f.Variables = []Variable{{
				Name: "X", Dims: []string{"I"}, Shape: []int{1}, Values: []float64{1},
				Attributes: []Attribute{{"DIMENSION_LIST", "x"}},
			}}
		}, "DIMENSION_LIST"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := minimalFIRFile()
			tc.set(f)
			err := f.validate()
			if err == nil {
				t.Fatal("validate accepted the file")
			}
			if !strings.Contains(err.Error(), tc.match) {
				t.Errorf("error %q does not mention %q", err, tc.match)
			}
			requireValidationError(t, err)
		})
	}
}

func hasAttribute(attrs []Attribute, name string) bool {
	for _, a := range attrs {
		if a.Name == name {
			return true
		}
	}
	return false
}

func findVariable(vars []Variable, name string) *Variable {
	for i := range vars {
		if vars[i].Name == name {
			return &vars[i]
		}
	}
	return nil
}
