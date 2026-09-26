package main

import (
	"reflect"
	"testing"

	sofa "github.com/CWBudde/go-sofa"
)

func TestParseHarness(t *testing.T) {
	out := "/tmp/a/fir.sofa: check 0\n" +
		"/tmp/a/tf.sofa: load err 10000\n" +
		"/tmp/odd: name.sofa: check 10004\n"
	got, err := parseHarness(out)
	if err != nil {
		t.Fatalf("parseHarness: %v", err)
	}
	want := map[string]result{
		"/tmp/a/fir.sofa":     {loaded: true, code: 0},
		"/tmp/a/tf.sofa":      {loaded: false, code: 10000},
		"/tmp/odd: name.sofa": {loaded: true, code: 10004},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseHarness = %v, want %v", got, want)
	}

	for _, bad := range []string{
		"no separator\n",
		"x.sofa: check\n",
		"x.sofa: check zero\n",
		"x.sofa: loaded 0\n",
	} {
		if _, err := parseHarness(bad); err == nil {
			t.Errorf("parseHarness(%q) succeeded, want an error", bad)
		}
	}
}

func TestResultString(t *testing.T) {
	if s := (result{loaded: true, code: 0}).String(); s != "check 0" {
		t.Errorf("String = %q, want %q", s, "check 0")
	}
	if s := (result{code: 10000}).String(); s != "load err 10000" {
		t.Errorf("String = %q, want %q", s, "load err 10000")
	}
}

func TestExpectFor(t *testing.T) {
	check0 := result{loaded: true, code: 0}
	check10004 := result{loaded: true, code: 10004}
	check10008 := result{loaded: true, code: 10008}
	loadErr10000 := result{code: 10000}
	loadErr10001 := result{code: 10001}

	for _, tc := range []struct {
		conventions, dataType string
		pass, fail            []result
	}{
		// The one combination libmysofa fully supports must check clean.
		{"SimpleFreeFieldHRIR", sofa.DataTypeFIR, []result{check0}, []result{check10008, loadErr10000}},
		// libmysofa's loader rejects frequency-domain data by design with
		// MYSOFA_INVALID_FORMAT; any other failure (such as 10001, an HDF5
		// structure it cannot parse) is a regression.
		{"SimpleFreeFieldHRTF", sofa.DataTypeTF, []result{loadErr10000}, []result{loadErr10001, check0}},
		{"GeneralTF-E", sofa.DataTypeTFE, []result{loadErr10000}, []result{loadErr10001}},
		// Everything else must load; mysofa_check may reject it.
		{"SimpleFreeFieldHRSOS", "SOS", []result{check0, check10004}, []result{loadErr10000}},
		{"SingleRoomSRIR", sofa.DataTypeFIR, []result{check10004}, []result{loadErr10001}},
		{"GeneralFIR", sofa.DataTypeFIR, []result{check10008}, []result{loadErr10000}},
	} {
		e := expectFor(tc.conventions, tc.dataType)
		for _, r := range tc.pass {
			if !e.match(r) {
				t.Errorf("%s/%s: %v rejected, want accepted (expect %v)", tc.conventions, tc.dataType, r, e)
			}
		}
		for _, r := range tc.fail {
			if e.match(r) {
				t.Errorf("%s/%s: %v accepted, want rejected (expect %v)", tc.conventions, tc.dataType, r, e)
			}
		}
	}
}

func TestExpectSame(t *testing.T) {
	e := expectSame(result{loaded: true, code: 10004})
	if !e.match(result{loaded: true, code: 10004}) {
		t.Error("same result rejected")
	}
	if e.match(result{loaded: true, code: 0}) || e.match(result{code: 10004}) {
		t.Error("different result accepted")
	}
}
