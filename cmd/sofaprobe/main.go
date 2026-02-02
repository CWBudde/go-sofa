// Command sofaprobe inspects a SOFA file and dumps its HDF5 structure,
// attributes, datasets, and dimension scales. Used during development to
// validate that go-hdf5 can read everything a SOFA file contains.
package main

import (
	"fmt"
	"os"
	"strings"

	hdf5 "github.com/meko-christian/go-hdf5"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: sofaprobe <file.sofa>\n")
		os.Exit(1)
	}

	f, err := hdf5.Open(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	fmt.Printf("=== SOFA Probe: %s ===\n\n", os.Args[1])
	fmt.Printf("Superblock version: %d\n", f.SuperblockVersion())

	root := f.Root()
	fmt.Printf("Children: %d\n\n", len(root.Children()))

	// 1. Root group attributes (AES69 global attributes)
	fmt.Println("--- Root Attributes ---")
	printGroupAttributes(root)

	// 2. Walk entire file tree
	fmt.Println("\n--- File Structure ---")
	f.Walk(func(path string, obj hdf5.Object) {
		switch v := obj.(type) {
		case *hdf5.Group:
			fmt.Printf("[Group]   %s\n", path)
		case *hdf5.Dataset:
			fmt.Printf("[Dataset] %s\n", path)
			printDatasetInfo(v)
		}
	})

	// 3. Try reading Data.IR (may be at root level or in Data group)
	fmt.Println("\n--- Data.IR Read Test ---")
	readDataIR(f)
}

func printGroupAttributes(g *hdf5.Group) {
	attrs, err := g.Attributes()
	if err != nil {
		fmt.Printf("  (error reading attributes: %v)\n", err)
		return
	}
	if len(attrs) == 0 {
		fmt.Println("  (none — may use dense attribute storage)")
		return
	}
	for _, a := range attrs {
		val, err := a.ReadValue()
		if err != nil {
			fmt.Printf("  %s = (error: %v)\n", a.Name, err)
			continue
		}
		fmt.Printf("  %s = %v\n", a.Name, val)
	}
}

func printDatasetInfo(ds *hdf5.Dataset) {
	info, err := ds.Info()
	if err != nil {
		fmt.Printf("  (info error: %v)\n", err)
	} else {
		fmt.Printf("  %s\n", info)
	}

	// List attributes (dimension scales etc.)
	attrNames, err := ds.ListAttributes()
	if err != nil {
		return
	}
	if len(attrNames) > 0 {
		fmt.Printf("  attributes: %s\n", strings.Join(attrNames, ", "))
		for _, name := range attrNames {
			val, err := ds.ReadAttribute(name)
			if err != nil {
				fmt.Printf("    %s = (error: %v)\n", name, err)
				continue
			}
			fmt.Printf("    %s = %v\n", name, val)
		}
	}
}

func readDataIR(f *hdf5.File) {
	root := f.Root()

	// SOFA files may have Data.IR at root level (flat) or in a Data group (nested).
	for _, child := range root.Children() {
		// Check for flat structure: dataset named "Data.IR" at root
		if ds, ok := child.(*hdf5.Dataset); ok && ds.Name() == "Data.IR" {
			readAndPrintDataset(ds)
			return
		}
		// Check for nested structure: Data group containing IR dataset
		if g, ok := child.(*hdf5.Group); ok && g.Name() == "Data" {
			for _, dChild := range g.Children() {
				if ds, ok := dChild.(*hdf5.Dataset); ok && ds.Name() == "IR" {
					readAndPrintDataset(ds)
					return
				}
			}
		}
	}
	fmt.Println("  Data.IR not found")
}

func readAndPrintDataset(ds *hdf5.Dataset) {
	data, err := ds.Read()
	if err != nil {
		fmt.Printf("  Read error: %v\n", err)
		return
	}
	fmt.Printf("  Data.IR: %d float64 values\n", len(data))
	if len(data) > 6 {
		fmt.Printf("  First 3: %v\n", data[:3])
		fmt.Printf("  Last 3:  %v\n", data[len(data)-3:])
	}
}
