// Command sofaprobe inspects SOFA files and dumps their HDF5 structure,
// attributes, datasets and dimension scales, followed by a preview of the
// audio data (Data.IR, Data.Real, Data.Imag or Data.SOS). Used during
// development to validate that go-hdf5 can read everything a SOFA file
// contains.
//
// Usage:
//
//	sofaprobe file.sofa [file.sofa ...]
//
// Errors go to stderr; the exit status is 1 if any file could not be
// opened and 2 on a usage error.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	hdf5 "github.com/cwbudde/go-hdf5"
)

// previewValues is how many values are shown from each end of a dataset.
const previewValues = 3

// dataVariables are the SOFA audio-data variables previewed, flat
// ("Data.IR") or nested in a Data group ("IR").
var dataVariables = []string{"IR", "Real", "Imag", "SOS"}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes sofaprobe with the given arguments and returns its exit
// status.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("sofaprobe", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: sofaprobe file.sofa [file.sofa ...]\n\n"+
			"Dump the HDF5 structure, attributes and a data preview of each file.\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return 2
	}

	status := 0
	printed := false
	for _, filename := range fs.Args() {
		f, err := hdf5.Open(filename)
		if err != nil {
			fmt.Fprintf(stderr, "sofaprobe: %s: %v\n", filename, err)
			status = 1
			continue
		}
		if printed {
			fmt.Fprintln(stdout)
		}
		probe(stdout, filename, f)
		printed = true
		if err := f.Close(); err != nil {
			fmt.Fprintf(stderr, "sofaprobe: %s: %v\n", filename, err)
			status = 1
		}
	}
	return status
}

func probe(w io.Writer, filename string, f *hdf5.File) {
	fmt.Fprintf(w, "=== SOFA Probe: %s ===\n\n", filename)
	fmt.Fprintf(w, "Superblock version: %d\n", f.SuperblockVersion())

	root := f.Root()
	fmt.Fprintf(w, "Children: %d\n\n", len(root.Children()))

	// 1. Root group attributes (AES69 global attributes)
	fmt.Fprintln(w, "--- Root Attributes ---")
	printGroupAttributes(w, root)

	// 2. Walk entire file tree
	fmt.Fprintln(w, "\n--- File Structure ---")
	f.Walk(func(path string, obj hdf5.Object) {
		switch v := obj.(type) {
		case *hdf5.Group:
			fmt.Fprintf(w, "[Group]   %s\n", path)
		case *hdf5.Dataset:
			fmt.Fprintf(w, "[Dataset] %s\n", path)
			printDatasetInfo(w, v)
		}
	})

	// 3. Preview the audio data
	fmt.Fprintln(w, "\n--- Data Preview ---")
	found := false
	for _, name := range dataVariables {
		if ds := findDataset(root, name); ds != nil {
			previewDataset(w, "Data."+name, ds)
			found = true
		}
	}
	if !found {
		fmt.Fprintln(w, "  no Data.IR, Data.Real, Data.Imag or Data.SOS found")
	}
}

func printGroupAttributes(w io.Writer, g *hdf5.Group) {
	attrs, err := g.Attributes()
	if err != nil {
		fmt.Fprintf(w, "  (error reading attributes: %v)\n", err)
		return
	}
	if len(attrs) == 0 {
		fmt.Fprintln(w, "  (none — may use dense attribute storage)")
		return
	}
	for _, a := range attrs {
		val, err := a.ReadValue()
		if err != nil {
			fmt.Fprintf(w, "  %s = (error: %v)\n", a.Name, err)
			continue
		}
		fmt.Fprintf(w, "  %s = %v\n", a.Name, val)
	}
}

func printDatasetInfo(w io.Writer, ds *hdf5.Dataset) {
	info, err := ds.Info()
	if err != nil {
		fmt.Fprintf(w, "  (info error: %v)\n", err)
	} else {
		fmt.Fprintf(w, "  %s\n", info)
	}

	// List attributes (dimension scales etc.)
	attrNames, err := ds.ListAttributes()
	if err != nil {
		return
	}
	if len(attrNames) > 0 {
		fmt.Fprintf(w, "  attributes: %s\n", strings.Join(attrNames, ", "))
		for _, name := range attrNames {
			val, err := ds.ReadAttribute(name)
			if err != nil {
				fmt.Fprintf(w, "    %s = (error: %v)\n", name, err)
				continue
			}
			fmt.Fprintf(w, "    %s = %v\n", name, val)
		}
	}
}

// findDataset returns the Data.<name> dataset, stored flat at the root
// ("Data.IR") or nested in a Data group ("Data/IR"), or nil.
func findDataset(root *hdf5.Group, name string) *hdf5.Dataset {
	for _, child := range root.Children() {
		if ds, ok := child.(*hdf5.Dataset); ok && ds.Name() == "Data."+name {
			return ds
		}
		if g, ok := child.(*hdf5.Group); ok && g.Name() == "Data" {
			for _, dChild := range g.Children() {
				if ds, ok := dChild.(*hdf5.Dataset); ok && ds.Name() == name {
					return ds
				}
			}
		}
	}
	return nil
}

// previewDataset prints the shape of ds and the first values of its first
// row and the last values of its last row (rows run along the last axis).
// It reads only those two rows, never the whole dataset. Whole rows are
// read because go-hdf5 v0.16.1 returns zeros for a contiguous hyperslab of
// three or more dimensions that starts inside a row.
func previewDataset(w io.Writer, label string, ds *hdf5.Dataset) {
	info, err := ds.Info()
	if err != nil {
		fmt.Fprintf(w, "  %s: info error: %v\n", label, err)
		return
	}
	shape := parseShape(info)
	if len(shape) == 0 {
		fmt.Fprintf(w, "  %s: no array shape in %q\n", label, info)
		return
	}
	total := uint64(1)
	for _, d := range shape {
		total *= d
	}
	fmt.Fprintf(w, "  %s: shape %v, %d values\n", label, shape, total)
	if total == 0 {
		return
	}

	last := len(shape) - 1
	rowLen := shape[last]
	n := min(rowLen, previewValues)
	count := make([]uint64, len(shape))
	for i := range count {
		count[i] = 1
	}
	count[last] = rowLen

	firstRow := make([]uint64, len(shape))
	lastRow := make([]uint64, len(shape))
	for i, d := range shape[:last] {
		lastRow[i] = d - 1
	}

	for _, s := range []struct {
		name  string
		start []uint64
		tail  bool
	}{
		{fmt.Sprintf("first %d:", n), firstRow, false},
		{fmt.Sprintf("last %d: ", n), lastRow, true},
	} {
		row, err := ds.ReadSlice(s.start, count)
		if err != nil {
			fmt.Fprintf(w, "    %s read error: %v\n", s.name, err)
			continue
		}
		vals, ok := row.([]float64)
		if !ok || uint64(len(vals)) != rowLen {
			fmt.Fprintf(w, "    %s %v\n", s.name, row)
			continue
		}
		if s.tail {
			vals = vals[rowLen-n:]
		} else {
			vals = vals[:n]
		}
		fmt.Fprintf(w, "    %s %v\n", s.name, vals)
	}
}

// shapeRE matches the dataspace part of go-hdf5's Dataset.Info output:
// "1D array [5]", "2D array [3 x 4]".
var shapeRE = regexp.MustCompile(`\b\d+D array \[([0-9 x]*)\]`)

// parseShape extracts the dimensions from a Dataset.Info string; nil if it
// describes no array.
func parseShape(info string) []uint64 {
	m := shapeRE.FindStringSubmatch(info)
	if m == nil {
		return nil
	}
	var shape []uint64
	for _, field := range strings.FieldsFunc(m[1], func(r rune) bool { return r == ' ' || r == 'x' }) {
		d, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return nil
		}
		shape = append(shape, d)
	}
	return shape
}
