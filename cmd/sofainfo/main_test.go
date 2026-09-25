package main

import (
	"bytes"
	"path/filepath"
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

func TestHelp(t *testing.T) {
	code, stdout, stderr := runCLI(t, "-h")
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	if !strings.Contains(stderr, "Usage: sofainfo") {
		t.Errorf("stderr lacks usage text:\n%s", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

func TestUnknownFlag(t *testing.T) {
	code, _, stderr := runCLI(t, "-bogus")
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr, "bogus") {
		t.Errorf("stderr does not name the flag:\n%s", stderr)
	}
}

func TestAllFileArguments(t *testing.T) {
	dir := t.TempDir()
	fir := clitest.Write(t, dir, "a.sofa", sofa.DataTypeFIR)
	tf := clitest.Write(t, dir, "b.sofa", sofa.DataTypeTF)

	code, stdout, stderr := runCLI(t, fir, tf)
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr:\n%s", code, stderr)
	}
	for _, want := range []string{
		"==> " + fir + " <==",
		"Title: go-sofa clitest FIR",
		"==> " + tf + " <==",
		"Title: go-sofa clitest TF",
		"Frequencies: 5 points, 0 Hz – 4000 Hz",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
}

func TestFailingFileSetsExitCode(t *testing.T) {
	dir := t.TempDir()
	good := clitest.Write(t, dir, "good.sofa", sofa.DataTypeFIR)
	missing := filepath.Join(dir, "missing.sofa")

	code, stdout, stderr := runCLI(t, missing, good)
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(stdout, "Title: go-sofa clitest FIR") {
		t.Errorf("the good file after the failing one was not processed:\n%s", stdout)
	}
	if !strings.Contains(stderr, missing) {
		t.Errorf("stderr does not name the failing file:\n%s", stderr)
	}
}

func TestConventionsAndDelay(t *testing.T) {
	path := clitest.Write(t, t.TempDir(), "a.sofa", sofa.DataTypeFIR)

	code, stdout, stderr := runCLI(t, path)
	if code != 0 {
		t.Fatalf("exit %d; stderr:\n%s", code, stderr)
	}
	for _, want := range []string{
		"Conventions: SOFA\n",
		"Version: 2.1\n",
		"SOFAConventions: SimpleFreeFieldHRIR\n",
		"SOFAConventionsVersion: 1.0\n",
		"Delay: 2 values [R], min 0, max 2.5: [0 2.5]\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "==>") {
		t.Errorf("a single file must not get a header:\n%s", stdout)
	}
}

func TestDelaySummary(t *testing.T) {
	tests := []struct {
		name  string
		m, r  int
		delay []float64
		want  string
	}{
		{"shared", 3, 2, []float64{1.5}, "Delay: 1 value [I], min 1.5, max 1.5: [1.5]"},
		{"per measurement", 3, 2, []float64{0, 1, 2}, "Delay: 3 values [M], min 0, max 2: [0 1 2]"},
		{"per receiver", 3, 2, []float64{4, 3}, "Delay: 2 values [R], min 3, max 4: [4 3]"},
		{"M×R", 3, 2, []float64{0, 1, 2, 3, 4, 5}, "Delay: 6 values [M,R], min 0, max 5: [0 1 2 3 4 5]"},
		{"long", 10, 1, []float64{9, 1, 2, 3, 4, 5, 6, 7, 8, 0}, "Delay: 10 values [M,R], min 0, max 9"},
		{"absent", 3, 2, nil, "Delay: none"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := delaySummary(&sofa.File{M: tc.m, R: tc.r, Delay: tc.delay})
			if got != tc.want {
				t.Errorf("delaySummary = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBatchModeGlobsWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	clitest.Write(t, dir, "a.sofa", sofa.DataTypeFIR)
	clitest.Write(t, dir, "b.sofa", sofa.DataTypeSOS)
	t.Chdir(dir)

	code, stdout, stderr := runCLI(t)
	if code != 0 {
		t.Fatalf("exit %d; stderr:\n%s", code, stderr)
	}
	for _, want := range []string{"==> a.sofa <==", "==> b.sofa <==", "Biquad sections per filter: 2 (N=12)"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
}

func TestBatchModeWithoutFiles(t *testing.T) {
	t.Chdir(t.TempDir())
	code, _, stderr := runCLI(t)
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(stderr, "no .sofa files") {
		t.Errorf("stderr:\n%s", stderr)
	}
}
