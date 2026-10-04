// Command mysofa checks that libmysofa, the reader most SOFA renderers use,
// loads the files go-sofa writes. It runs the load harness built from
// scripts/libmysofa (mysofa_load, then mysofa_check) once over:
//
//   - every <dir>/*.sofa written by internal/interop/gen, held to a rule per
//     convention and DataType (see expectFor);
//   - every file gen re-saved from testdata/sofar/ into <dir>/resaved/ and
//     <dir>/resaved-deflate/, and every further original given on the
//     command line (re-saved here into <dir>/resaved-extra/, and deflated
//     into <dir>/resaved-extra-deflate/), each of which must get exactly the
//     result its original gets.
//
// It is internal and not a supported tool. Run it from the repository root.
//
// Usage: go run ./internal/interop/mysofa -load <harness> <dir> [original.sofa ...]
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	sofa "github.com/CWBudde/go-sofa"
)

// libmysofa error codes (mysofa.h) the rules refer to.
const (
	mysofaOK            = 0
	mysofaInvalidFormat = 10000
)

// result is what the harness printed for one file: mysofa_load failed with
// code, or it succeeded and mysofa_check returned code.
type result struct {
	loaded bool
	code   int
}

func (r result) String() string {
	if r.loaded {
		return fmt.Sprintf("check %d", r.code)
	}
	return fmt.Sprintf("load err %d", r.code)
}

// expect is the result a file must get: exactly want, or, with anyCheck,
// any successful load.
type expect struct {
	want     result
	anyCheck bool
}

func (e expect) match(r result) bool {
	if e.anyCheck {
		return r.loaded
	}
	return r == e.want
}

func (e expect) String() string {
	if e.anyCheck {
		return "a successful load"
	}
	return e.want.String()
}

// expectFor returns the rule for a file go-sofa wrote with the given
// convention and DataType. libmysofa is an HRIR reader: SimpleFreeFieldHRIR
// must pass mysofa_check; frequency-domain data is rejected by its loader by
// design (MYSOFA_INVALID_FORMAT, as for files written by other tools), so
// any other load failure is a regression; everything else must at least
// load.
func expectFor(conventions, dataType string) expect {
	switch {
	case conventions == "SimpleFreeFieldHRIR" && dataType == sofa.DataTypeFIR:
		return expect{want: result{loaded: true, code: mysofaOK}}
	case dataType == sofa.DataTypeTF || dataType == sofa.DataTypeTFE:
		return expect{want: result{code: mysofaInvalidFormat}}
	default:
		return expect{anyCheck: true}
	}
}

// expectSame is the rule for a re-saved file: the original's result.
func expectSame(original result) expect {
	return expect{want: original}
}

// parseHarness parses the harness output, one "<file>: check <code>" or
// "<file>: load err <code>" line per file.
func parseHarness(out string) (map[string]result, error) {
	results := map[string]result{}
	for line := range strings.Lines(out) {
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		i := strings.LastIndex(line, ": ")
		if i < 0 {
			return nil, fmt.Errorf("harness line %q: no \": \" separator", line)
		}
		path, verdict := line[:i], line[i+2:]
		var r result
		var num string
		switch {
		case strings.HasPrefix(verdict, "check "):
			r.loaded, num = true, strings.TrimPrefix(verdict, "check ")
		case strings.HasPrefix(verdict, "load err "):
			num = strings.TrimPrefix(verdict, "load err ")
		default:
			return nil, fmt.Errorf("harness line %q: unknown verdict", line)
		}
		code, err := strconv.Atoi(num)
		if err != nil {
			return nil, fmt.Errorf("harness line %q: %w", line, err)
		}
		r.code = code
		results[path] = r
	}
	return results, nil
}

// check is one file to hold to a rule. For a re-saved file, original is the
// file whose result it must reproduce instead.
type check struct {
	path     string
	label    string
	rule     expect
	original string
}

func main() {
	load := flag.String("load", "", "path to the libmysofa load harness (scripts/libmysofa/build.sh)")
	flag.Parse()
	if *load == "" || flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: mysofa -load <harness> <dir> [original.sofa ...]")
		os.Exit(2)
	}
	failed, err := run(*load, flag.Arg(0), flag.Args()[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "mysofa:", err)
		os.Exit(1)
	}
	if failed {
		os.Exit(1)
	}
}

func run(load, dir string, originals []string) (failed bool, err error) {
	checks, err := generatedChecks(dir)
	if err != nil {
		return false, err
	}
	for _, sub := range []string{"resaved", "resaved-deflate"} {
		resaved, err := sofarResaves(dir, sub)
		if err != nil {
			return false, err
		}
		checks = append(checks, resaved...)
	}
	for _, r := range []struct {
		sub  string
		opts []sofa.SaveOption
	}{
		{"resaved-extra", nil},
		{"resaved-extra-deflate", []sofa.SaveOption{sofa.WithDeflate(4)}},
	} {
		extra, err := resaveOriginals(filepath.Join(dir, r.sub), originals, r.opts...)
		if err != nil {
			return false, err
		}
		checks = append(checks, extra...)
	}

	paths := make([]string, 0, 2*len(checks))
	for _, c := range checks {
		paths = append(paths, c.path)
		if c.original != "" {
			paths = append(paths, c.original)
		}
	}
	results, err := runHarness(load, paths)
	if err != nil {
		return false, err
	}

	for _, c := range checks {
		rule := c.rule
		if c.original != "" {
			orig, ok := results[c.original]
			if !ok {
				return false, fmt.Errorf("harness printed nothing for %s", c.original)
			}
			rule = expectSame(orig)
		}
		got, ok := results[c.path]
		switch {
		case !ok:
			fmt.Printf("FAIL libmysofa %s: harness printed nothing\n", c.label)
			failed = true
		case !rule.match(got):
			fmt.Printf("FAIL libmysofa %s: %v, want %v\n", c.label, got, rule)
			failed = true
		default:
			fmt.Printf("ok   libmysofa %s (%v)\n", c.label, got)
		}
	}
	return failed, nil
}

// generatedChecks returns one check per <dir>/*.sofa, with the rule for its
// convention and DataType.
func generatedChecks(dir string) ([]check, error) {
	names, err := filepath.Glob(filepath.Join(dir, "*.sofa"))
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no .sofa files in %s (run internal/interop/gen first)", dir)
	}
	checks := make([]check, 0, len(names))
	for _, p := range names {
		f, err := sofa.Open(p)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", p, err)
		}
		checks = append(checks, check{path: p, label: filepath.Base(p), rule: expectFor(f.SOFAConventions, f.DataType)})
		f.Close()
	}
	return checks, nil
}

// sofarResaves returns one check per file gen re-saved from testdata/sofar/
// into <dir>/<sub>/, as listed in its resaved.json.
func sofarResaves(dir, sub string) ([]check, error) {
	data, err := os.ReadFile(filepath.Join(dir, sub, "resaved.json")) //nolint:gosec // dir from CLI arg (dev tool)
	if err != nil {
		return nil, err
	}
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return nil, fmt.Errorf("resaved.json: %w", err)
	}
	checks := make([]check, 0, len(names))
	for _, name := range names {
		checks = append(checks, check{
			path:     filepath.Join(dir, sub, name),
			label:    sub + "/" + name,
			original: filepath.Join("testdata", "sofar", name),
		})
	}
	return checks, nil
}

// resaveOriginals opens each original, saves it into dst with opts and
// returns one check per file.
func resaveOriginals(dst string, originals []string, opts ...sofa.SaveOption) ([]check, error) {
	if len(originals) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(dst, 0o750); err != nil { //nolint:gosec // output dir from CLI arg (dev tool)
		return nil, err
	}
	checks := make([]check, 0, len(originals))
	for _, p := range originals {
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("%w (run `just fetch-testdata`)", err)
		}
		f, err := sofa.Open(p)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", p, err)
		}
		out := filepath.Join(dst, filepath.Base(p))
		err = f.Save(out, opts...)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("save %s: %w", out, err)
		}
		checks = append(checks, check{path: out, label: filepath.Base(dst) + "/" + filepath.Base(p), original: p})
	}
	return checks, nil
}

// runHarness runs the harness over paths and parses its output. The harness
// exits 1 when a file fails to load, which the rules may expect; any other
// failure (a crash in particular) is an error.
func runHarness(load string, paths []string) (map[string]result, error) {
	cmd := exec.Command(load, paths...) //nolint:gosec // harness path from CLI arg (dev tool)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		err = nil
	}
	if err != nil {
		return nil, fmt.Errorf("run %s: %w", load, err)
	}
	return parseHarness(string(out))
}
