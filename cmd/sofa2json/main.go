// Command sofa2json exports SOFA files to JSON.
//
// By default it exports metadata, dimensions, positions, SamplingRate and
// Delay only. Bulk audio data is gated by per-DataType flags:
//
//	--include-ir   FIR  files: include ImpulseResponses
//	--include-tf   TF / TF-E files: include TFReal/TFImag (TF-E: TFRealE/TFImagE)
//	--include-sos  SOS  files: include SOSCoefficients
//
// Frequencies are included automatically (small) for TF / TF-E. JSON keys
// are the field names of sofa.File; positions and vectors are [x, y, z]
// triples in their coordinate Type and Units, which are exported alongside.
// NaN and ±Inf are written as null.
//
// Usage:
//
//	sofa2json [flags] [file.sofa ...]
//
// Each file is written to <file>.json (replacing the .sofa extension);
// an existing output file is an error unless -f is given. Without file
// arguments every .sofa file in the current directory is converted.
// Progress and errors go to stderr; the exit status is 1 if any file
// failed and 2 on a usage error.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/CWBudde/go-sofa"
)

// includeFlags carries the user's --include-* selections.
type includeFlags struct {
	IR, TF, SOS bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes sofa2json with the given arguments and returns its exit
// status. It writes nothing to stdout.
func run(args []string, _, stderr io.Writer) int {
	var inc includeFlags
	var force bool
	flags := flag.NewFlagSet("sofa2json", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.BoolVar(&inc.IR, "include-ir", false, "FIR files: include ImpulseResponses")
	flags.BoolVar(&inc.TF, "include-tf", false, "TF / TF-E files: include TFReal/TFImag (TFRealE/TFImagE)")
	flags.BoolVar(&inc.SOS, "include-sos", false, "SOS files: include SOSCoefficients")
	flags.BoolVar(&force, "f", false, "overwrite existing .json files")
	flags.Usage = func() {
		fmt.Fprintf(stderr, "Usage: sofa2json [flags] [file.sofa ...]\n\n"+
			"Export each SOFA file to <file>.json. Without file arguments,\n"+
			"convert every .sofa file in the current directory.\n\nFlags:\n")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	files := flags.Args()
	if len(files) == 0 {
		matches, err := filepath.Glob("*.sofa")
		if err != nil {
			fmt.Fprintf(stderr, "sofa2json: %v\n", err)
			return 1
		}
		if len(matches) == 0 {
			fmt.Fprintln(stderr, "sofa2json: no .sofa files found in current directory")
			return 1
		}
		files = matches
	}

	status := 0
	for _, filename := range files {
		out, err := convert(filename, inc, force)
		if err != nil {
			fmt.Fprintf(stderr, "sofa2json: %s: %v\n", filename, err)
			status = 1
			continue
		}
		fmt.Fprintf(stderr, "%s -> %s\n", filename, out)
	}
	return status
}

// convert exports one SOFA file and returns the path of the JSON file. On
// error no (partial) output file is left behind, and with force an existing
// output file is replaced only once the new one is completely written.
func convert(filename string, inc includeFlags, force bool) (out string, err error) {
	f, err := sofa.Open(filename)
	if err != nil {
		return "", err
	}

	out = strings.TrimSuffix(filename, filepath.Ext(filename)) + ".json"
	if out == filename {
		return "", fmt.Errorf("output %s would overwrite the input", out)
	}

	// Without force, create out exclusively: there is no previous file to
	// protect. With force, write a temporary file next to out and rename
	// it over out, so a failed export keeps the previous JSON intact.
	target := out
	var w *os.File
	if force {
		w, err = os.CreateTemp(filepath.Dir(out), "."+filepath.Base(out)+".tmp-*")
	} else {
		w, err = os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // output path derived from the user-supplied input path
		if errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("%s already exists (use -f to overwrite)", out)
		}
	}
	if err != nil {
		return "", err
	}
	written := w.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(written)
		}
	}()

	if err = encodeJSON(w, f, inc); err == nil {
		err = w.Close()
	} else {
		_ = w.Close()
	}
	if err != nil {
		return "", fmt.Errorf("write %s: %w", target, err)
	}
	if force {
		if err = os.Rename(written, target); err != nil {
			return "", fmt.Errorf("replace %s: %w", target, err)
		}
	}
	return out, nil
}

// encodeJSON is the encoder convert uses; tests replace it to simulate a
// failed export.
var encodeJSON = encode

// encode streams f as an indented JSON object to w. Bulk data is written
// value by value, so memory use does not grow with the output size.
func encode(w io.Writer, f *sofa.File, inc includeFlags) error {
	e := &encoder{w: bufio.NewWriter(w)}
	e.metadata(f)
	e.positions(f)
	e.data(f, inc)
	e.raw("\n}\n")
	return e.w.Flush()
}

func (e *encoder) metadata(f *sofa.File) {
	for _, a := range []struct{ name, value string }{
		{"Conventions", f.Conventions},
		{"Version", f.Version},
		{"SOFAConventions", f.SOFAConventions},
		{"SOFAConventionsVersion", f.SOFAConventionsVersion},
		{"DataType", f.DataType},
		{"RoomType", f.RoomType},
		{"Title", f.Title},
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
		e.stringField(a.name, a.value)
	}
	if f.RoomVolume != 0 {
		e.key("RoomVolume")
		e.float(f.RoomVolume)
	}
	if f.RoomTemperature != 0 {
		e.key("RoomTemperature")
		e.float(f.RoomTemperature)
	}

	// M is always written, so the object is never empty and key's "{"
	// always precedes encode's closing "}".
	for _, d := range []struct {
		name  string
		value int
	}{{"M", f.M}, {"R", f.R}, {"E", f.E}, {"N", f.N}} {
		e.key(d.name)
		e.raw(strconv.Itoa(d.value))
	}

	e.key("SamplingRate")
	e.floats(f.SamplingRate)
	e.key("Delay")
	e.floats(f.Delay)
}

func (e *encoder) positions(f *sofa.File) {
	for _, p := range []struct {
		name, typ, units string
		values           []sofa.Vector3
	}{
		{"ListenerPositions", f.ListenerPositionType, f.ListenerPositionUnits, f.ListenerPositions},
		{"ReceiverPositions", f.ReceiverPositionType, f.ReceiverPositionUnits, f.ReceiverPositions},
		{"SourcePositions", f.SourcePositionType, f.SourcePositionUnits, f.SourcePositions},
		{"EmitterPositions", f.EmitterPositionType, f.EmitterPositionUnits, f.EmitterPositions},
	} {
		base := strings.TrimSuffix(p.name, "s")
		e.stringField(base+"Type", p.typ)
		e.stringField(base+"Units", p.units)
		e.key(p.name)
		e.vectors(1, p.values)
	}
	for _, p := range []struct {
		name   string
		values [][]sofa.Vector3
	}{
		{"ReceiverPositionsM", f.ReceiverPositionsM},
		{"EmitterPositionsM", f.EmitterPositionsM},
	} {
		if len(p.values) > 0 {
			e.key(p.name)
			e.array(1, len(p.values), func(i int) { e.vectors(2, p.values[i]) })
		}
	}

	e.stringField("ListenerViewType", f.ListenerViewType)
	e.stringField("ListenerViewUnits", f.ListenerViewUnits)
	e.key("ListenerView")
	e.vector(f.ListenerView)
	e.key("ListenerUp")
	e.vector(f.ListenerUp)
	if len(f.ListenerViews) > 0 {
		e.key("ListenerViews")
		e.vectors(1, f.ListenerViews)
	}
	if len(f.ListenerUps) > 0 {
		e.key("ListenerUps")
		e.vectors(1, f.ListenerUps)
	}
}

func (e *encoder) data(f *sofa.File, inc includeFlags) {
	switch f.DataType {
	case sofa.DataTypeFIR:
		if inc.IR {
			e.key("ImpulseResponses")
			e.floats3(f.ImpulseResponses)
		}
	case sofa.DataTypeTF:
		e.frequencies(f)
		if inc.TF {
			e.key("TFReal")
			e.floats3(f.TFReal)
			e.key("TFImag")
			e.floats3(f.TFImag)
		}
	case sofa.DataTypeTFE:
		e.frequencies(f)
		if inc.TF {
			e.key("TFRealE")
			e.floats4(f.TFRealE)
			e.key("TFImagE")
			e.floats4(f.TFImagE)
		}
	case sofa.DataTypeSOS:
		if inc.SOS {
			e.key("SOSCoefficients")
			e.floats3(f.SOSCoefficients)
		}
	}
}

func (e *encoder) frequencies(f *sofa.File) {
	if len(f.Frequencies) > 0 {
		e.key("Frequencies")
		e.floats(f.Frequencies)
	}
}

// encoder writes one indented JSON object field by field. Write errors are
// sticky in the bufio.Writer and reported by Flush.
type encoder struct {
	w      *bufio.Writer
	fields int
	num    []byte
}

func (e *encoder) raw(s string) { _, _ = e.w.WriteString(s) }

// key starts the next top-level field.
func (e *encoder) key(name string) {
	if e.fields == 0 {
		e.raw("{\n  ")
	} else {
		e.raw(",\n  ")
	}
	e.fields++
	e.str(name)
	e.raw(": ")
}

// stringField writes a string field unless the value is empty.
func (e *encoder) stringField(name, value string) {
	if value == "" {
		return
	}
	e.key(name)
	e.str(value)
}

func (e *encoder) str(s string) {
	b, _ := json.Marshal(s) // cannot fail for a string
	_, _ = e.w.Write(b)
}

// float writes v, or null for NaN and ±Inf, which JSON cannot represent.
func (e *encoder) float(v float64) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		e.raw("null")
		return
	}
	e.num = strconv.AppendFloat(e.num[:0], v, 'g', -1, 64)
	_, _ = e.w.Write(e.num)
}

// floats writes a flat array on one line.
func (e *encoder) floats(vs []float64) {
	e.raw("[")
	for i, v := range vs {
		if i > 0 {
			e.raw(", ")
		}
		e.float(v)
	}
	e.raw("]")
}

func (e *encoder) vector(v sofa.Vector3) { e.floats([]float64{v.X, v.Y, v.Z}) }

// array writes n elements, one per line, indented for nesting depth.
func (e *encoder) array(depth, n int, elem func(i int)) {
	if n == 0 {
		e.raw("[]")
		return
	}
	indent := strings.Repeat("  ", depth+1)
	e.raw("[\n")
	for i := range n {
		if i > 0 {
			e.raw(",\n")
		}
		e.raw(indent)
		elem(i)
	}
	e.raw("\n" + strings.Repeat("  ", depth) + "]")
}

func (e *encoder) vectors(depth int, vs []sofa.Vector3) {
	e.array(depth, len(vs), func(i int) { e.vector(vs[i]) })
}

func (e *encoder) floats3(d [][][]float64) {
	e.array(1, len(d), func(i int) {
		e.array(2, len(d[i]), func(j int) { e.floats(d[i][j]) })
	})
}

func (e *encoder) floats4(d [][][][]float64) {
	e.array(1, len(d), func(i int) {
		e.array(2, len(d[i]), func(j int) {
			e.array(3, len(d[i][j]), func(k int) { e.floats(d[i][j][k]) })
		})
	})
}
