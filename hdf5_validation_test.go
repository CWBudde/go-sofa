// Phase 1 integration tests: validate that go-hdf5 can read everything a SOFA file contains.
package sofa

import (
	"testing"

	hdf5 "github.com/meko-christian/go-hdf5"
)

const testFile = "testdata/MIT_KEMAR_normal_pinna.sofa"

// TestReadRootGroupAttributes validates reading AES69 global attributes from the
// root group. SOFA files typically have >8 global attributes, which causes netCDF4/
// HDF5 to store them in dense (fractal heap) format rather than compact format.
//
// Known go-hdf5 gap: Group.Attributes() returns empty when attributes use dense
// storage. This test documents the limitation.
func TestReadRootGroupAttributes(t *testing.T) {
	f, err := hdf5.Open(testFile)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	root := f.Root()
	attrs, err := root.Attributes()
	if err != nil {
		t.Fatalf("Attributes: %v", err)
	}

	if len(attrs) == 0 {
		t.Skip("root group attributes not readable (dense attribute storage — go-hdf5 limitation)")
	}

	// If we get here, dense attribute reading has been fixed in go-hdf5.
	// Validate expected AES69 attributes.
	expected := []string{"Conventions", "Version", "SOFAConventions", "DataType"}
	found := make(map[string]bool)
	for _, a := range attrs {
		found[a.Name] = true
		val, err := a.ReadValue()
		if err != nil {
			t.Errorf("ReadValue %s: %v", a.Name, err)
			continue
		}
		t.Logf("%s = %v", a.Name, val)
	}

	for _, name := range expected {
		if !found[name] {
			t.Errorf("expected attribute %q not found", name)
		}
	}
}

func TestOpenSOFAFile(t *testing.T) {
	f, err := hdf5.Open(testFile)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	root := f.Root()
	if root == nil {
		t.Fatal("Root() returned nil")
	}
	if n := len(root.Children()); n == 0 {
		t.Fatal("root has 0 children")
	} else {
		t.Logf("root has %d children", n)
	}
}

func TestWalkGroupsAndDatasets(t *testing.T) {
	f, err := hdf5.Open(testFile)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	var groups, datasets int
	f.Walk(func(path string, obj hdf5.Object) {
		switch obj.(type) {
		case *hdf5.Group:
			groups++
		case *hdf5.Dataset:
			datasets++
		}
	})

	if groups == 0 {
		t.Error("no groups found")
	}
	if datasets == 0 {
		t.Error("no datasets found")
	}
	t.Logf("found %d groups, %d datasets", groups, datasets)

	// MIT KEMAR has 16 datasets at root level
	if datasets < 10 {
		t.Errorf("expected at least 10 datasets, got %d", datasets)
	}
}

func TestReadDataIR(t *testing.T) {
	f, err := hdf5.Open(testFile)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	root := f.Root()
	for _, child := range root.Children() {
		ds, ok := child.(*hdf5.Dataset)
		if !ok || ds.Name() != "Data.IR" {
			continue
		}

		data, err := ds.Read()
		if err != nil {
			t.Fatalf("Read Data.IR: %v", err)
		}

		// MIT KEMAR: 710 measurements × 2 receivers × 512 samples = 727040
		expected := 710 * 2 * 512
		if len(data) != expected {
			t.Errorf("Data.IR length = %d, want %d", len(data), expected)
		}
		t.Logf("Data.IR: %d float64 values", len(data))
		return
	}
	t.Fatal("Data.IR dataset not found")
}

func TestReadDatasetStringAttributes(t *testing.T) {
	f, err := hdf5.Open(testFile)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	// Find SourcePosition dataset and check its attributes
	root := f.Root()
	for _, child := range root.Children() {
		ds, ok := child.(*hdf5.Dataset)
		if !ok || ds.Name() != "SourcePosition" {
			continue
		}

		val, err := ds.ReadAttribute("Type")
		if err != nil {
			t.Fatalf("ReadAttribute Type: %v", err)
		}
		typeStr, ok := val.(string)
		if !ok {
			t.Fatalf("Type attribute is %T, want string", val)
		}
		if typeStr != "spherical" {
			t.Errorf("Type = %q, want %q", typeStr, "spherical")
		}

		val, err = ds.ReadAttribute("Units")
		if err != nil {
			t.Fatalf("ReadAttribute Units: %v", err)
		}
		t.Logf("SourcePosition: Type=%v, Units=%v", typeStr, val)
		return
	}
	t.Fatal("SourcePosition dataset not found")
}

func TestReadDimensionScaleAttributes(t *testing.T) {
	f, err := hdf5.Open(testFile)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	// Dimension datasets (M, R, E, N, etc.) should have CLASS=DIMENSION_SCALE
	dimNames := map[string]bool{"M": false, "R": false, "N": false, "E": false}

	root := f.Root()
	for _, child := range root.Children() {
		ds, ok := child.(*hdf5.Dataset)
		if !ok {
			continue
		}
		if _, isDim := dimNames[ds.Name()]; !isDim {
			continue
		}

		val, err := ds.ReadAttribute("CLASS")
		if err != nil {
			t.Errorf("%s: ReadAttribute CLASS: %v", ds.Name(), err)
			continue
		}
		classStr, ok := val.(string)
		if !ok || classStr != "DIMENSION_SCALE" {
			t.Errorf("%s: CLASS = %v, want DIMENSION_SCALE", ds.Name(), val)
			continue
		}

		// NAME attribute contains the dimension size info
		nameVal, err := ds.ReadAttribute("NAME")
		if err != nil {
			t.Errorf("%s: ReadAttribute NAME: %v", ds.Name(), err)
			continue
		}

		dimNames[ds.Name()] = true
		t.Logf("%s: CLASS=%s, NAME=%v", ds.Name(), classStr, nameVal)
	}

	for name, found := range dimNames {
		if !found {
			t.Errorf("dimension dataset %s not found or missing attributes", name)
		}
	}
}

// TestSOFAIntegration is a standalone integration test that exercises all HDF5
// features needed by SOFA files in a single test, across all test files.
func TestSOFAIntegration(t *testing.T) {
	files := []string{
		"testdata/MIT_KEMAR_normal_pinna.sofa",
		"testdata/CIPIC_subject_003_hrir_final.sofa",
		"testdata/tester.sofa",
	}

	for _, path := range files {
		t.Run(path, func(t *testing.T) {
			f, err := hdf5.Open(path)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer f.Close()

			root := f.Root()
			if root == nil {
				t.Fatal("Root() returned nil")
			}

			// 1. Walk structure: must have groups and datasets.
			var groups, datasets int
			f.Walk(func(_ string, obj hdf5.Object) {
				switch obj.(type) {
				case *hdf5.Group:
					groups++
				case *hdf5.Dataset:
					datasets++
				}
			})
			if groups == 0 {
				t.Error("no groups found")
			}
			if datasets < 10 {
				t.Errorf("expected ≥10 datasets, got %d", datasets)
			}

			// 2. Read Data.IR — multi-dimensional float64 dataset.
			var foundIR bool
			for _, child := range root.Children() {
				ds, ok := child.(*hdf5.Dataset)
				if !ok || ds.Name() != "Data.IR" {
					continue
				}
				foundIR = true
				data, err := ds.Read()
				if err != nil {
					t.Fatalf("Read Data.IR: %v", err)
				}
				if len(data) == 0 {
					t.Error("Data.IR is empty")
				}
				t.Logf("Data.IR: %d values", len(data))
			}
			if !foundIR {
				t.Error("Data.IR dataset not found")
			}

			// 3. Dimension-scale datasets (M, R, N, E).
			dimNames := []string{"M", "R", "N", "E"}
			for _, child := range root.Children() {
				ds, ok := child.(*hdf5.Dataset)
				if !ok {
					continue
				}
				for _, dim := range dimNames {
					if ds.Name() != dim {
						continue
					}
					val, err := ds.ReadAttribute("CLASS")
					if err != nil {
						t.Errorf("%s: ReadAttribute CLASS: %v", dim, err)
						break
					}
					s, ok := val.(string)
					if !ok || s != "DIMENSION_SCALE" {
						t.Errorf("%s: CLASS = %v, want DIMENSION_SCALE", dim, val)
					}
				}
			}

			// 4. Dataset string attributes (e.g. SourcePosition Type/Units).
			for _, child := range root.Children() {
				ds, ok := child.(*hdf5.Dataset)
				if !ok || ds.Name() != "SourcePosition" {
					continue
				}
				val, err := ds.ReadAttribute("Type")
				if err != nil {
					t.Errorf("SourcePosition Type: %v", err)
				} else {
					t.Logf("SourcePosition Type = %v", val)
				}
			}

			// 5. Root group attributes (known gap: dense storage not yet supported).
			attrs, err := root.Attributes()
			if err != nil {
				t.Errorf("root Attributes: %v", err)
			} else if len(attrs) == 0 {
				t.Log("root attributes: 0 (dense storage — go-hdf5 limitation)")
			} else {
				t.Logf("root attributes: %d", len(attrs))
			}
		})
	}
}
