// Command sofainfo prints a metadata summary of SOFA files: AES69 global
// attributes, dimensions and basic audio parameters.
//
// Usage:
//
//	sofainfo [file.sofa ...]
//
// Without file arguments it summarises every .sofa file in the current
// directory. With several files each summary is preceded by a
// "==> file <==" header. Errors go to stderr; the exit status is 1 if any
// file could not be read and 2 on a usage error.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/CWBudde/go-sofa"
)

// maxDelayValues is the most Delay values printed in full.
const maxDelayValues = 8

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes sofainfo with the given arguments and returns its exit
// status.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("sofainfo", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: sofainfo [file.sofa ...]\n\n"+
			"Print a metadata summary of each SOFA file. Without arguments,\n"+
			"summarise every .sofa file in the current directory.\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	files := fs.Args()
	if len(files) == 0 {
		matches, err := filepath.Glob("*.sofa")
		if err != nil {
			fmt.Fprintf(stderr, "sofainfo: %v\n", err)
			return 1
		}
		if len(matches) == 0 {
			fmt.Fprintln(stderr, "sofainfo: no .sofa files found in current directory")
			return 1
		}
		files = matches
	}

	status := 0
	for i, filename := range files {
		f, err := sofa.Open(filename)
		if err != nil {
			fmt.Fprintf(stderr, "sofainfo: %s: %v\n", filename, err)
			status = 1
			continue
		}
		if len(files) > 1 {
			if i > 0 {
				fmt.Fprintln(stdout)
			}
			fmt.Fprintf(stdout, "==> %s <==\n", filename)
		}
		printFileInformation(stdout, f)
	}
	return status
}

func printFileInformation(w io.Writer, f *sofa.File) {
	// Print all AES69 global attributes (if non-empty)
	for _, a := range []struct{ name, value string }{
		{"Conventions", f.Conventions},
		{"Version", f.Version},
		{"SOFAConventions", f.SOFAConventions},
		{"SOFAConventionsVersion", f.SOFAConventionsVersion},
		{"Title", f.Title},
		{"DataType", f.DataType},
		{"RoomType", f.RoomType},
		{"DateCreated", f.DateCreated},
		{"DateModified", f.DateModified},
		{"APIName", f.APIName},
		{"APIVersion", f.APIVersion},
		{"AuthorContact", f.AuthorContact},
		{"Organization", f.Organization},
		{"License", f.License},
		{"ApplicationName", f.ApplicationName},
		{"ApplicationVersion", f.ApplicationVersion},
		{"Comment", f.Comment},
		{"History", f.History},
		{"References", f.References},
		{"Origin", f.Origin},
	} {
		if a.value != "" {
			fmt.Fprintf(w, "%s: %s\n", a.name, a.value)
		}
	}

	fmt.Fprintln(w)

	// Print dimensions and audio parameters
	fmt.Fprintf(w, "Number of Measurements: %d\n", f.M)
	fmt.Fprintf(w, "Number of Receivers: %d\n", f.R)
	fmt.Fprintf(w, "Number of Emitters: %d\n", f.E)
	fmt.Fprintf(w, "Number of DataSamples: %d\n", f.N)
	if len(f.SamplingRate) > 0 {
		fmt.Fprintf(w, "SampleRate: %g\n", f.SamplingRate[0])
	}
	fmt.Fprintln(w, delaySummary(f))
	if (f.DataType == sofa.DataTypeTF || f.DataType == sofa.DataTypeTFE) && len(f.Frequencies) > 0 {
		fmt.Fprintf(w, "Frequencies: %d points, %g Hz – %g Hz\n",
			len(f.Frequencies),
			f.Frequencies[0],
			f.Frequencies[len(f.Frequencies)-1])
	}
	if f.DataType == sofa.DataTypeSOS && f.N > 0 {
		fmt.Fprintf(w, "Biquad sections per filter: %d (N=%d)\n", f.N/6, f.N)
	}
	if lmax, ok := f.SHOrder(); ok {
		fmt.Fprintf(w, "SH-encoded HRTF: Lmax=%d, %d coefficients\n", lmax, f.SHCoefficientCount())
	}
	if order, ok := f.AmbisonicsOrder(); ok {
		fmt.Fprintf(w, "SRIR Ambisonics order: %d (R=%d)\n", order, f.R)
	}
	for _, warning := range f.SHWarnings() {
		fmt.Fprintf(w, "Warning: %s\n", warning)
	}
	for _, warning := range f.ConventionWarnings() {
		fmt.Fprintf(w, "Warning: %s\n", warning)
	}
}

// delaySummary describes Data.Delay: its number of values, their
// dimensions (as Open read them, see sofa.File.DelayDimensions), the
// range, and the values themselves when there are at most maxDelayValues.
func delaySummary(f *sofa.File) string {
	n := len(f.Delay)
	if n == 0 {
		return "Delay: none"
	}
	dims := f.DelayDimensions()
	size := 1
	for _, d := range dims {
		switch d {
		case "M":
			size *= f.M
		case "R":
			size *= f.R
		}
	}
	layout := strings.Join(dims, ",")
	if size != n {
		layout = "?"
	}
	unit := "values"
	if n == 1 {
		unit = "value"
	}
	s := fmt.Sprintf("Delay: %d %s [%s], min %g, max %g", n, unit, layout, slices.Min(f.Delay), slices.Max(f.Delay))
	if n <= maxDelayValues {
		vals := make([]string, n)
		for i, v := range f.Delay {
			vals[i] = fmt.Sprintf("%g", v)
		}
		s += ": [" + strings.Join(vals, " ") + "]"
	}
	return s
}
