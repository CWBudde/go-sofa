package main

import (
	"bytes"
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

func TestUsage(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		code int
	}{
		{"help", []string{"-h"}, 0},
		{"no files", nil, 2},
		{"unknown flag", []string{"-bogus", "x.sofa"}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runCLI(t, tc.args...)
			if code != tc.code {
				t.Fatalf("exit %d, want %d", code, tc.code)
			}
			if !strings.Contains(stderr, "Usage: sofaprobe") {
				t.Errorf("stderr lacks usage text:\n%s", stderr)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want empty", stdout)
			}
		})
	}
}

func TestDataPreview(t *testing.T) {
	tests := []struct {
		dataType string
		want     []string
	}{
		{sofa.DataTypeFIR, []string{
			"Data.IR: shape [3 2 8], 48 values",
			"first 3: [0 1 2]",
			"last 3:  [215 216 217]",
		}},
		{sofa.DataTypeTF, []string{
			"Data.Real: shape [3 2 5], 30 values",
			"last 3:  [212 213 214]",
			"Data.Imag: shape [3 2 5], 30 values",
			"last 3:  [-212 -213 -214]",
		}},
		{sofa.DataTypeTFE, []string{
			// [M,R,N,E]: the last axis runs over the two emitters.
			"Data.Real: shape [3 2 4 2], 48 values",
			"first 2: [0 1000]",
			"last 2:  [213 1213]",
		}},
		{sofa.DataTypeSOS, []string{
			"Data.SOS: shape [3 2 12], 72 values",
			"last 3:  [219 220 221]",
		}},
	}
	for _, tc := range tests {
		t.Run(tc.dataType, func(t *testing.T) {
			path := clitest.Write(t, t.TempDir(), "a.sofa", tc.dataType)
			code, stdout, stderr := runCLI(t, path)
			if code != 0 {
				t.Fatalf("exit %d; stderr:\n%s", code, stderr)
			}
			for _, want := range append(tc.want, "=== SOFA Probe: "+path+" ===", "[Dataset] ") {
				if !strings.Contains(stdout, want) {
					t.Errorf("stdout lacks %q:\n%s", want, stdout)
				}
			}
		})
	}
}

func TestFailingFileSetsExitCode(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.sofa")
	good := clitest.Write(t, dir, "good.sofa", sofa.DataTypeFIR)

	code, stdout, stderr := runCLI(t, missing, good)
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(stdout, "=== SOFA Probe: "+good+" ===") {
		t.Errorf("the good file after the failing one was not probed:\n%s", stdout)
	}
	if !strings.Contains(stderr, missing) {
		t.Errorf("stderr does not name the failing file:\n%s", stderr)
	}
}

func TestParseShape(t *testing.T) {
	for _, tc := range []struct {
		info string
		want []uint64
	}{
		{"Dataset: Data.IR, 3D array [3 x 2 x 8], float64", []uint64{3, 2, 8}},
		{"1D array [5]", []uint64{5}},
		{"scalar", nil},
		{"garbage", nil},
	} {
		if got := parseShape(tc.info); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("parseShape(%q) = %v, want %v", tc.info, got, tc.want)
		}
	}
}
