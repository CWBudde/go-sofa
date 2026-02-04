package sofa

import (
	"fmt"
	"testing"
	"github.com/meko-christian/go-hdf5"
)

func TestInspectSOFA20(t *testing.T) {
	f, err := hdf5.Open("testdata/FreeFieldHRTF_2.0.sofa")
	if err != nil {
		t.Fatalf("Error opening file: %v", err)
	}
	defer f.Close()
	
	root := f.RootGroup()
	
	// Check SOFA version
	if val, err := root.ReadAttribute("SOFAConventionsVersion"); err == nil {
		fmt.Printf("SOFAConventionsVersion: %v\n", val)
	}
	
	if val, err := root.ReadAttribute("Conventions"); err == nil {
		fmt.Printf("Conventions: %v\n", val)
	}
	
	if val, err := root.ReadAttribute("Version"); err == nil {
		fmt.Printf("Version: %v\n", val)
	}
	
	// List dimension variables
	fmt.Println("\nDimension variables:")
	for _, name := range []string{"M", "R", "E", "N", "C", "I"} {
		if ds, err := root.Dataset(name); err == nil {
			fmt.Printf("  %s: exists\n", name)
			if class, err := ds.ReadAttribute("CLASS"); err == nil {
				fmt.Printf("    CLASS: %v\n", class)
			}
			if nameAttr, err := ds.ReadAttribute("NAME"); err == nil {
				fmt.Printf("    NAME: %v\n", nameAttr)
			} else {
				fmt.Printf("    NAME: error - %v\n", err)
			}
			ds.Close()
		}
	}
	
	// Check Data group
	if dataGrp, err := root.Group("Data"); err == nil {
		fmt.Println("\nData group contents:")
		datasets, _ := dataGrp.Datasets()
		for _, name := range datasets {
			fmt.Printf("  %s\n", name)
			if ds, err := dataGrp.Dataset(name); err == nil {
				info := ds.Info()
				fmt.Printf("    Shape: %v\n", info.Shape)
				ds.Close()
			}
		}
		dataGrp.Close()
	}
}
