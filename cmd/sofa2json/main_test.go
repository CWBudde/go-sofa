package main

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	sofa "github.com/cwbudde/go-sofa"
	"github.com/cwbudde/go-sofa/internal/clitest"
)

func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

// readJSON decodes the JSON file written for a .sofa path.
func readJSON(t *testing.T, sofaPath string) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(strings.TrimSuffix(sofaPath, ".sofa") + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, data)
	}
	return doc
}

func decode[T any](t *testing.T, doc map[string]json.RawMessage, key string) T {
	t.Helper()
	var v T
	raw, ok := doc[key]
	if !ok {
		t.Fatalf("key %q missing", key)
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode %q: %v", key, err)
	}
	return v
}

func TestHelp(t *testing.T) {
	code, stdout, stderr := runCLI(t, "-h")
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	for _, want := range []string{"Usage: sofa2json", "-include-ir", "-f"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("usage lacks %q:\n%s", want, stderr)
		}
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

func TestUnknownFlag(t *testing.T) {
	code, _, stderr := runCLI(t, "--include-everything")
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr, "include-everything") {
		t.Errorf("stderr does not name the flag:\n%s", stderr)
	}
}

func TestAllFileArguments(t *testing.T) {
	dir := t.TempDir()
	a := clitest.Write(t, dir, "a.sofa", sofa.DataTypeFIR)
	b := clitest.Write(t, dir, "b.sofa", sofa.DataTypeSOS)

	code, stdout, stderr := runCLI(t, a, b)
	if code != 0 {
		t.Fatalf("exit %d; stderr:\n%s", code, stderr)
	}
	if stdout != "" {
		t.Errorf("progress must go to stderr, stdout = %q", stdout)
	}
	for _, p := range []string{a, b} {
		readJSON(t, p)
		if !strings.Contains(stderr, p) {
			t.Errorf("stderr does not report %s:\n%s", p, stderr)
		}
	}
}

func TestFailingFileSetsExitCode(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.sofa")
	good := clitest.Write(t, dir, "good.sofa", sofa.DataTypeFIR)

	code, _, stderr := runCLI(t, missing, good)
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	readJSON(t, good)
	if !strings.Contains(stderr, missing) {
		t.Errorf("stderr does not name the failing file:\n%s", stderr)
	}
}

func TestRefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := clitest.Write(t, dir, "a.sofa", sofa.DataTypeFIR)
	out := filepath.Join(dir, "a.json")
	if err := os.WriteFile(out, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := runCLI(t, path)
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(stderr, "-f") {
		t.Errorf("stderr does not mention -f:\n%s", stderr)
	}
	if got, _ := os.ReadFile(out); string(got) != "keep" {
		t.Fatalf("existing output was modified: %q", got)
	}

	if code, _, stderr := runCLI(t, "-f", path); code != 0 {
		t.Fatalf("-f: exit %d; stderr:\n%s", code, stderr)
	}
	readJSON(t, path)
}

func TestMetadataKeys(t *testing.T) {
	path := clitest.Write(t, t.TempDir(), "a.sofa", sofa.DataTypeFIR)
	if code, _, stderr := runCLI(t, path); code != 0 {
		t.Fatalf("exit %d; stderr:\n%s", code, stderr)
	}
	doc := readJSON(t, path)
	want := clitest.Build(sofa.DataTypeFIR)

	strs := map[string]string{
		"Conventions":            want.Conventions,
		"Version":                want.Version,
		"SOFAConventions":        want.SOFAConventions,
		"SOFAConventionsVersion": want.SOFAConventionsVersion,
		"DataType":               want.DataType,
		"Title":                  want.Title,
		"ListenerPositionType":   want.ListenerPositionType,
		"ListenerPositionUnits":  want.ListenerPositionUnits,
		"SourcePositionType":     want.SourcePositionType,
		"SourcePositionUnits":    want.SourcePositionUnits,
		"ReceiverPositionType":   want.ReceiverPositionType,
		"EmitterPositionUnits":   want.EmitterPositionUnits,
	}
	for k, v := range strs {
		if got := decode[string](t, doc, k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	ints := map[string]int{"M": want.M, "R": want.R, "E": want.E, "N": want.N}
	for k, v := range ints {
		if got := decode[int](t, doc, k); got != v {
			t.Errorf("%s = %d, want %d", k, got, v)
		}
	}
	if got := decode[[]float64](t, doc, "SamplingRate"); !reflect.DeepEqual(got, want.SamplingRate) {
		t.Errorf("SamplingRate = %v", got)
	}
	if got := decode[[]float64](t, doc, "Delay"); !reflect.DeepEqual(got, want.Delay) {
		t.Errorf("Delay = %v", got)
	}
	if got := decode[[][3]float64](t, doc, "SourcePositions"); len(got) != want.M || got[1] != [3]float64{30, 0, 1.5} {
		t.Errorf("SourcePositions = %v", got)
	}
	if got := decode[[][3]float64](t, doc, "ReceiverPositions"); len(got) != want.R || got[0] != [3]float64{0, 0.09, 0} {
		t.Errorf("ReceiverPositions = %v", got)
	}
	decode[[][3]float64](t, doc, "ListenerPositions")
	decode[[][3]float64](t, doc, "EmitterPositions")
	if got := decode[[3]float64](t, doc, "ListenerView"); got != [3]float64{1, 0, 0} {
		t.Errorf("ListenerView = %v", got)
	}
	if got := decode[[3]float64](t, doc, "ListenerUp"); got != [3]float64{0, 0, 1} {
		t.Errorf("ListenerUp = %v", got)
	}
	for _, old := range []string{"Measurements", "Receivers", "SampleRate", "IR", "ImpulseResponses"} {
		if _, ok := doc[old]; ok {
			t.Errorf("unexpected key %q", old)
		}
	}
}

func TestIncludeData(t *testing.T) {
	tests := []struct {
		dataType string
		flag     string
		check    func(t *testing.T, doc map[string]json.RawMessage, f *sofa.File)
	}{
		{sofa.DataTypeFIR, "--include-ir", func(t *testing.T, doc map[string]json.RawMessage, f *sofa.File) {
			if got := decode[[][][]float64](t, doc, "ImpulseResponses"); !reflect.DeepEqual(got, f.ImpulseResponses) {
				t.Errorf("ImpulseResponses = %v", got)
			}
		}},
		{sofa.DataTypeTF, "--include-tf", func(t *testing.T, doc map[string]json.RawMessage, f *sofa.File) {
			if got := decode[[][][]float64](t, doc, "TFReal"); !reflect.DeepEqual(got, f.TFReal) {
				t.Errorf("TFReal = %v", got)
			}
			if got := decode[[][][]float64](t, doc, "TFImag"); !reflect.DeepEqual(got, f.TFImag) {
				t.Errorf("TFImag = %v", got)
			}
		}},
		{sofa.DataTypeTFE, "--include-tf", func(t *testing.T, doc map[string]json.RawMessage, f *sofa.File) {
			if got := decode[[][][][]float64](t, doc, "TFRealE"); !reflect.DeepEqual(got, f.TFRealE) {
				t.Errorf("TFRealE = %v", got)
			}
			if got := decode[[][][][]float64](t, doc, "TFImagE"); !reflect.DeepEqual(got, f.TFImagE) {
				t.Errorf("TFImagE = %v", got)
			}
		}},
		{sofa.DataTypeSOS, "--include-sos", func(t *testing.T, doc map[string]json.RawMessage, f *sofa.File) {
			if got := decode[[][][]float64](t, doc, "SOSCoefficients"); !reflect.DeepEqual(got, f.SOSCoefficients) {
				t.Errorf("SOSCoefficients = %v", got)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.dataType, func(t *testing.T) {
			dir := t.TempDir()
			path := clitest.Write(t, dir, "a.sofa", tc.dataType)
			want := clitest.Build(tc.dataType)

			// Without the flag only metadata (and, for TF, the small
			// frequency vector) is exported.
			if code, _, stderr := runCLI(t, path); code != 0 {
				t.Fatalf("exit %d; stderr:\n%s", code, stderr)
			}
			doc := readJSON(t, path)
			for _, k := range []string{"ImpulseResponses", "TFReal", "TFRealE", "SOSCoefficients"} {
				if _, ok := doc[k]; ok {
					t.Errorf("%s exported without %s", k, tc.flag)
				}
			}
			if want.Frequencies != nil {
				if got := decode[[]float64](t, doc, "Frequencies"); !reflect.DeepEqual(got, want.Frequencies) {
					t.Errorf("Frequencies = %v", got)
				}
			}

			if code, _, stderr := runCLI(t, "-f", tc.flag, path); code != 0 {
				t.Fatalf("exit %d; stderr:\n%s", code, stderr)
			}
			tc.check(t, readJSON(t, path), want)
		})
	}
}

func TestNonFiniteValuesBecomeNull(t *testing.T) {
	f := clitest.Build(sofa.DataTypeFIR)
	f.Delay = []float64{math.NaN(), math.Inf(1)}
	f.SamplingRate = []float64{math.Inf(-1)}
	f.ImpulseResponses[0][0][0] = math.NaN()
	f.ListenerView = sofa.Vector3{X: math.NaN(), Y: 0, Z: 0}

	var buf bytes.Buffer
	if err := encode(&buf, f, includeFlags{IR: true}); err != nil {
		t.Fatal(err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.Bytes())
	}
	for key, want := range map[string]string{
		"Delay":        "[null, null]",
		"SamplingRate": "[null]",
		"ListenerView": "[null, 0, 0]",
	} {
		if got := string(doc[key]); got != want {
			t.Errorf("%s = %s, want %s", key, got, want)
		}
	}
	ir := decode[[][][]*float64](t, doc, "ImpulseResponses")
	if ir[0][0][0] != nil || ir[0][0][1] == nil || *ir[0][0][1] != 1 {
		t.Errorf("ImpulseResponses[0][0][:2] = %v, %v", ir[0][0][0], ir[0][0][1])
	}
}

func TestBatchModeGlobsWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	clitest.Write(t, dir, "a.sofa", sofa.DataTypeFIR)
	clitest.Write(t, dir, "b.sofa", sofa.DataTypeTF)
	t.Chdir(dir)

	if code, _, stderr := runCLI(t); code != 0 {
		t.Fatalf("exit %d; stderr:\n%s", code, stderr)
	}
	readJSON(t, filepath.Join(dir, "a.sofa"))
	readJSON(t, filepath.Join(dir, "b.sofa"))
}
