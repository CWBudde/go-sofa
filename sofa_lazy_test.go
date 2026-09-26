package sofa

import (
	"reflect"
	"strings"
	"testing"

	"github.com/cwbudde/go-hdf5"
)

// iotaData returns 0, 1, …, n-1: every value of a crafted dataset differs,
// so a misplaced sample is detected.
func iotaData(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = float64(i)
	}
	return out
}

// tfSpec is a TF file with M=3, R=2, N=4 whose Data.Real and Data.Imag
// hold distinct values.
func tfSpec() craftedSpec {
	mrn := []string{dimM, dimR, dimN}
	return craftedSpec{
		dataType: DataTypeTF,
		dims:     map[string]int{dimM: 3, dimR: 2, dimE: 1, dimN: 4, dimC: 3, dimI: 1},
		vars: map[string]craftedVar{
			"Data.Real": {dims: mrn, data: iotaData(24)},
			"Data.Imag": {dims: mrn, data: iotaData(48)[24:]},
		},
	}
}

// sosSpec is an SOS file with M=3, R=2, N=12 (two biquads) of distinct
// coefficients.
func sosSpec() craftedSpec {
	return craftedSpec{
		dataType: DataTypeSOS,
		dims:     map[string]int{dimM: 3, dimR: 2, dimE: 1, dimN: 12, dimC: 3, dimI: 1},
		vars: map[string]craftedVar{
			"Data.SOS":          {dims: []string{dimM, dimR, dimN}, data: iotaData(72)},
			"Data.SamplingRate": {dims: []string{dimI}, data: []float64{48000}},
		},
	}
}

// TestOpenLazyMatchesOpen checks that OpenLazy accepts every CI fixture and
// fills the same fields as Open apart from the audio data.
func TestOpenLazyMatchesOpen(t *testing.T) {
	for _, name := range []string{
		"CIPIC_subject_003_hrir_final.sofa",
		"MIT_KEMAR_normal_pinna.sofa",
		"Mesh2HRTF.sofa",
		"Mesh2HRTF_HRTF_FourPointHorPlane_r100cm.sofa",
		"tester.sofa",
	} {
		t.Run(name, func(t *testing.T) {
			path := testdataPath(t, name)
			eager, err := Open(path)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			lazy, err := OpenLazy(path)
			if err != nil {
				t.Fatalf("OpenLazy: %v", err)
			}
			if err := lazy.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			eager.ImpulseResponses, eager.TFReal, eager.TFImag = nil, nil, nil
			eager.TFRealE, eager.TFImagE, eager.SOSCoefficients = nil, nil, nil
			lazy.lazy = nil
			if !reflect.DeepEqual(lazy, eager) {
				t.Errorf("OpenLazy and Open differ beyond the audio data")
			}
		})
	}
}

// TestOpenLazyRejectsNonNumericAudio checks that OpenLazy, like Open,
// rejects an audio dataset that does not hold numbers.
func TestOpenLazyRejectsNonNumericAudio(t *testing.T) {
	for _, tc := range []struct {
		spec craftedSpec
		name string
	}{
		{firSpec(), "Data.IR"},
		{tfSpec(), "Data.Real"},
		{tfSpec(), "Data.Imag"},
		{tfeSpec(3, 4, []string{dimM, dimR, dimN, dimE}, false), "Data.Imag"},
		{sosSpec(), "Data.SOS"},
	} {
		t.Run(tc.spec.dataType+" "+tc.name, func(t *testing.T) {
			shape := make([]uint64, 0, 4)
			order := tc.spec.vars[tc.name].dims
			if order == nil {
				order = []string{dimM, dimR, dimN, dimE}
			}
			for _, d := range order {
				shape = append(shape, uint64(tc.spec.dims[d])) //nolint:gosec // small test sizes
			}
			delete(tc.spec.vars, tc.name)
			tc.spec.extra = func(t *testing.T, fw *hdf5.FileWriter) {
				t.Helper()
				n := 1
				for _, d := range shape {
					n *= int(d) //nolint:gosec // small test sizes
				}
				ds, err := fw.CreateDataset("/"+tc.name, hdf5.String, shape, hdf5.WithStringSize(4))
				if err != nil {
					t.Fatalf("create /%s: %v", tc.name, err)
				}
				if err := ds.Write(make([]string, n)); err != nil {
					t.Fatalf("write /%s: %v", tc.name, err)
				}
			}
			path := writeCraftedSpec(t, tc.spec)
			if f, err := Open(path); err == nil {
				f.Close()
				t.Fatalf("Open accepted a string %s", tc.name)
			}
			f, err := OpenLazy(path)
			if err == nil {
				f.Close()
				t.Fatalf("OpenLazy accepted a string %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.name) {
				t.Errorf("error %q does not name %s", err, tc.name)
			}
		})
	}
}
