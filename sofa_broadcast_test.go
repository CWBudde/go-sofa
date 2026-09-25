package sofa

import (
	"errors"
	"slices"
	"testing"
)

func TestSamplingRateScalarVarying(t *testing.T) {
	cases := []struct {
		name    string
		rates   []float64
		want    float64
		wantErr error
	}{
		{"single [I]", []float64{48000}, 48000, nil},
		{"uniform [M]", []float64{44100, 44100, 44100}, 44100, nil},
		{"varying [M]", []float64{44100, 48000, 44100}, 0, ErrVaryingSamplingRate},
		{"none", nil, 0, ErrNoSamplingRate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &File{M: 3, SamplingRate: tc.rates}
			got, err := f.SamplingRateScalar()
			if got != tc.want || !errors.Is(err, tc.wantErr) || (tc.wantErr == nil && err != nil) {
				t.Errorf("SamplingRateScalar() = (%v, %v), want (%v, %v)", got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestSamplingRateAt(t *testing.T) {
	varying := &File{M: 3, SamplingRate: []float64{44100, 48000, 96000}}
	for m, want := range varying.SamplingRate {
		if got, err := varying.SamplingRateAt(m); err != nil || got != want {
			t.Errorf("[M] SamplingRateAt(%d) = (%v, %v), want %v", m, got, err, want)
		}
	}
	shared := &File{M: 3, SamplingRate: []float64{48000}}
	if got, err := shared.SamplingRateAt(2); err != nil || got != 48000 {
		t.Errorf("[I] SamplingRateAt(2) = (%v, %v), want 48000", got, err)
	}
	for _, m := range []int{-1, 3} {
		if _, err := varying.SamplingRateAt(m); !errors.Is(err, ErrIndexOutOfRange) {
			t.Errorf("SamplingRateAt(%d) error = %v, want ErrIndexOutOfRange", m, err)
		}
	}
	if _, err := (&File{M: 3}).SamplingRateAt(0); !errors.Is(err, ErrNoSamplingRate) {
		t.Errorf("SamplingRateAt without rate: error = %v, want ErrNoSamplingRate", err)
	}
	bad := &File{M: 3, SamplingRate: []float64{1, 2}}
	if _, err := bad.SamplingRateAt(0); !errors.Is(err, ErrIndexOutOfRange) {
		t.Errorf("SamplingRateAt with 2 rates for M=3: error = %v, want ErrIndexOutOfRange", err)
	}
}

func TestSourcePositionAt(t *testing.T) {
	perM := &File{M: 2, SourcePositions: []Vector3{{1, 0, 0}, {0, 1, 0}}}
	for m, want := range perM.SourcePositions {
		if got, err := perM.SourcePositionAt(m); err != nil || got != want {
			t.Errorf("[M,C] SourcePositionAt(%d) = (%v, %v), want %v", m, got, err, want)
		}
	}
	shared := &File{M: 4, SourcePositions: []Vector3{{0, 0, 1}}}
	if got, err := shared.SourcePositionAt(3); err != nil || got != (Vector3{0, 0, 1}) {
		t.Errorf("[I,C] SourcePositionAt(3) = (%v, %v), want {0 0 1}", got, err)
	}
	for _, tc := range []struct {
		f *File
		m int
	}{
		{perM, -1}, {perM, 2}, {&File{M: 2}, 0}, {&File{M: 3, SourcePositions: make([]Vector3, 2)}, 0},
	} {
		if _, err := tc.f.SourcePositionAt(tc.m); !errors.Is(err, ErrIndexOutOfRange) {
			t.Errorf("SourcePositionAt(%d) with %d positions, M=%d: error = %v, want ErrIndexOutOfRange",
				tc.m, len(tc.f.SourcePositions), tc.f.M, err)
		}
	}
}

// TestDelayAtInMemory covers every Delay length a File built in memory can
// have; the layout then follows the rule Save writes by.
func TestDelayAtInMemory(t *testing.T) {
	const M, R = 3, 2
	at := func(delay []float64, m, r int) float64 {
		t.Helper()
		v, err := (&File{M: M, R: R, Delay: delay}).DelayAt(m, r)
		if err != nil {
			t.Fatalf("DelayAt(%d, %d) with %d values: %v", m, r, len(delay), err)
		}
		return v
	}
	if got := at(nil, 2, 1); got != 0 {
		t.Errorf("no Delay: DelayAt = %v, want 0", got)
	}
	if got := at([]float64{7}, 2, 1); got != 7 {
		t.Errorf("[I]: DelayAt(2, 1) = %v, want 7", got)
	}
	if got := at([]float64{1, 2}, 2, 1); got != 2 {
		t.Errorf("[R]: DelayAt(2, 1) = %v, want 2", got)
	}
	if got := at([]float64{10, 20, 30}, 2, 0); got != 30 {
		t.Errorf("[M]: DelayAt(2, 0) = %v, want 30", got)
	}
	if got := at([]float64{0, 1, 10, 11, 20, 21}, 2, 1); got != 21 {
		t.Errorf("[M,R]: DelayAt(2, 1) = %v, want 21", got)
	}
	f := &File{M: M, R: R, Delay: []float64{1, 2}}
	for _, idx := range [][2]int{{-1, 0}, {M, 0}, {0, -1}, {0, R}} {
		if _, err := f.DelayAt(idx[0], idx[1]); !errors.Is(err, ErrIndexOutOfRange) {
			t.Errorf("DelayAt(%d, %d) error = %v, want ErrIndexOutOfRange", idx[0], idx[1], err)
		}
	}
	bad := &File{M: M, R: R, Delay: []float64{1, 2, 3, 4}}
	if _, err := bad.DelayAt(2, 1); !errors.Is(err, ErrIndexOutOfRange) {
		t.Errorf("DelayAt with 4 values for M=3 R=2: error = %v, want ErrIndexOutOfRange", err)
	}
}

// TestDelayAtFileLayouts reads crafted files with M == R, where the
// length of Data.Delay alone cannot tell [M] from [R]: DelayAt must use
// the dimension names Open resolved.
func TestDelayAtFileLayouts(t *testing.T) {
	const M, R = 2, 2
	cases := []struct {
		dims []string
		data []float64
		want [M][R]float64
	}{
		{[]string{dimR}, []float64{10, 20}, [M][R]float64{{10, 20}, {10, 20}}},
		{[]string{dimM}, []float64{10, 20}, [M][R]float64{{10, 10}, {20, 20}}},
		{[]string{dimI, dimR}, []float64{10, 20}, [M][R]float64{{10, 20}, {10, 20}}},
		{[]string{dimM, dimR}, []float64{1, 2, 3, 4}, [M][R]float64{{1, 2}, {3, 4}}},
		{[]string{dimI}, []float64{5}, [M][R]float64{{5, 5}, {5, 5}}},
	}
	for _, tc := range cases {
		t.Run(fmtDims(tc.dims), func(t *testing.T) {
			spec := firSpec()
			spec.dims[dimM], spec.dims[dimR] = M, R
			spec.vars["Data.IR"] = craftedVar{dims: []string{dimM, dimR, dimN}}
			spec.vars["Data.Delay"] = craftedVar{dims: tc.dims, data: tc.data}
			f, err := Open(writeCraftedSpec(t, spec))
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer f.Close()
			if got := f.DelayDimensions(); !slices.Equal(got, tc.dims) {
				t.Errorf("DelayDimensions() = %v, want %v", got, tc.dims)
			}
			for m := range M {
				for r := range R {
					got, err := f.DelayAt(m, r)
					if err != nil || got != tc.want[m][r] {
						t.Errorf("DelayAt(%d, %d) = (%v, %v), want %v", m, r, got, err, tc.want[m][r])
					}
				}
			}
		})
	}
}

func fmtDims(dims []string) string {
	s := "["
	for i, d := range dims {
		if i > 0 {
			s += ","
		}
		s += d
	}
	return s + "]"
}

// TestBroadcastFixtures checks the helpers on a real file: an [I,R] delay
// with [M,C] source positions (MIT KEMAR). The [M,R] fixtures
// (SimpleFreeFieldSOS) cannot be opened yet, see PLAN.md Phase B; the
// crafted TestDelayAtFileLayouts covers that layout.
func TestBroadcastFixtures(t *testing.T) {
	f, err := Open(testdataPath(t, "MIT_KEMAR_normal_pinna.sofa"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	last := f.M - 1
	if got, err := f.SourcePositionAt(last); err != nil || got != f.SourcePositions[last] {
		t.Errorf("SourcePositionAt(%d) = (%v, %v), want %v", last, got, err, f.SourcePositions[last])
	}
	if got, err := f.DelayAt(last, 1); err != nil || got != f.Delay[1] {
		t.Errorf("[I,R] DelayAt(%d, 1) = (%v, %v), want %v", last, got, err, f.Delay[1])
	}
	if got, err := f.SamplingRateAt(last); err != nil || got != f.SamplingRate[0] {
		t.Errorf("SamplingRateAt(%d) = (%v, %v), want %v", last, got, err, f.SamplingRate[0])
	}
}

// TestDelayDimensionsInMemory checks the layout of a Delay that Open did
// not read: implied by its length, M×R before M before R.
func TestDelayDimensionsInMemory(t *testing.T) {
	cases := []struct {
		m, r  int
		delay []float64
		want  []string
	}{
		{3, 2, nil, nil},
		{3, 2, []float64{1}, []string{dimI}},
		{3, 2, []float64{1, 2, 3}, []string{dimM}},
		{3, 2, []float64{1, 2}, []string{dimR}},
		{3, 2, make([]float64, 6), []string{dimM, dimR}},
		{2, 2, []float64{1, 2}, []string{dimM}},
		{1, 2, []float64{1, 2}, []string{dimM, dimR}},
	}
	for _, tc := range cases {
		f := &File{M: tc.m, R: tc.r, Delay: tc.delay}
		if got := f.DelayDimensions(); !slices.Equal(got, tc.want) {
			t.Errorf("M=%d R=%d Delay=%v: DelayDimensions() = %v, want %v", tc.m, tc.r, tc.delay, got, tc.want)
		}
	}
}
