package sofa

import (
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestValidateRejectsNonFinite checks that Save refuses NaN and ±Inf in
// every value it writes, and names the offending field.
func TestValidateRejectsNonFinite(t *testing.T) {
	nan, inf := math.NaN(), math.Inf(1)
	for _, tc := range []struct {
		field  string
		base   func() *File
		mutate func(*File)
	}{
		{"ImpulseResponses", minimalFIRFile, func(f *File) { f.ImpulseResponses[1][0][2] = nan }},
		{"SamplingRate", minimalFIRFile, func(f *File) { f.SamplingRate[0] = inf }},
		{"Delay", minimalFIRFile, func(f *File) { f.Delay[3] = nan }},
		{"ListenerPositions", minimalFIRFile, func(f *File) { f.ListenerPositions[0].Y = nan }},
		{"ReceiverPositions", minimalFIRFile, func(f *File) { f.ReceiverPositions[1].X = -inf }},
		{"SourcePositions", minimalFIRFile, func(f *File) { f.SourcePositions[2].Z = nan }},
		{"EmitterPositions", minimalFIRFile, func(f *File) { f.EmitterPositions[0].Z = inf }},
		{"ListenerView", minimalFIRFile, func(f *File) { f.ListenerView.X = nan }},
		{"ListenerUp", minimalFIRFile, func(f *File) { f.ListenerUp.Z = inf }},
		{"ReceiverPositionsM", minimalFIRFile, func(f *File) {
			f.ReceiverPositionsM = [][]Vector3{{{0, 1, 0}}, {{0, 1, 0}}, {{0, nan, 0}}}
		}},
		{"EmitterPositionsM", minimalFIRFile, func(f *File) {
			f.EmitterPositionsM = [][]Vector3{{{0, 0, 0}}, {{inf, 0, 0}}, {{0, 0, 0}}}
		}},
		{"ListenerViews", minimalFIRFile, func(f *File) {
			f.ListenerViews = []Vector3{{1, 0, 0}, {1, nan, 0}, {1, 0, 0}}
		}},
		{"ListenerUps", minimalFIRFile, func(f *File) {
			f.ListenerUps = []Vector3{{0, 0, 1}, {0, 0, 1}, {0, 0, inf}}
		}},
		{"RoomVolume", minimalFIRFile, func(f *File) { f.RoomVolume = nan }},
		{"RoomTemperature", minimalFIRFile, func(f *File) { f.RoomTemperature = inf }},
		{"Frequencies", minimalTFFile, func(f *File) { f.Frequencies[2] = inf }},
		{"TFReal", minimalTFFile, func(f *File) { f.TFReal[1][0][1] = nan }},
		{"TFImag", minimalTFFile, func(f *File) { f.TFImag[0][0][0] = -inf }},
		{"TFRealE", minimalTFEFile, func(f *File) { f.TFRealE[1][0][1][2] = nan }},
		{"TFImagE", minimalTFEFile, func(f *File) { f.TFImagE[0][0][0][0] = inf }},
		{"SOSCoefficients", minimalSOSFile, func(f *File) { f.SOSCoefficients[0][1][3] = nan }},
	} {
		t.Run(tc.field, func(t *testing.T) {
			f := tc.base()
			if err := f.validate(); err != nil {
				t.Fatalf("base file invalid: %v", err)
			}
			tc.mutate(f)
			err := f.validate()
			if err == nil {
				t.Fatalf("validate accepted a non-finite %s", tc.field)
			}
			if !strings.Contains(err.Error(), tc.field) {
				t.Errorf("error %q does not name %s", err, tc.field)
			}
		})
	}
}

// TestValidateRejectsBadData checks the value constraints beyond finiteness:
// positive sampling rates, ascending non-negative frequencies and non-zero
// listener orientations.
func TestValidateRejectsBadData(t *testing.T) {
	for _, tc := range []struct {
		name   string
		base   func() *File
		mutate func(*File)
		want   string // substring of the error; empty means valid
	}{
		{"zero sampling rate", minimalFIRFile, func(f *File) { f.SamplingRate = []float64{0} }, "SamplingRate"},
		{"negative sampling rate", minimalFIRFile, func(f *File) { f.SamplingRate = []float64{44100, -1, 44100} }, "SamplingRate"},
		{"SOS zero sampling rate", minimalSOSFile, func(f *File) { f.SamplingRate = []float64{0} }, "SamplingRate"},
		{"descending frequencies", minimalTFFile, func(f *File) { f.Frequencies = []float64{100, 400, 200} }, "Frequencies"},
		{"repeated frequency", minimalTFFile, func(f *File) { f.Frequencies = []float64{100, 200, 200} }, "Frequencies"},
		{"negative frequency", minimalTFFile, func(f *File) { f.Frequencies = []float64{-100, 200, 400} }, "Frequencies"},
		{"TF-E descending frequencies", minimalTFEFile, func(f *File) { f.Frequencies = []float64{400, 200, 100} }, "Frequencies"},
		{"frequencies from DC", minimalTFFile, func(f *File) { f.Frequencies = []float64{0, 200, 400} }, ""},
		{"zero row in ListenerViews", minimalFIRFile, func(f *File) {
			f.ListenerViews = []Vector3{{1, 0, 0}, {}, {1, 0, 0}}
		}, "ListenerViews[1]"},
		{"zero row in ListenerUps", minimalFIRFile, func(f *File) {
			f.ListenerUps = []Vector3{{0, 0, 1}, {0, 0, 1}, {}}
		}, "ListenerUps[2]"},
		{"spherical ListenerViews row with zero radius", minimalFIRFile, func(f *File) {
			setSphericalOrientation(f)
			f.ListenerViews = []Vector3{{0, 0, 1}, {90, 0, 0}, {0, 0, 1}}
		}, "ListenerViews[1]"},
		{"spherical ListenerView with zero radius", minimalFIRFile, func(f *File) {
			f.ListenerViewType = CoordinateSpherical
			f.ListenerView = Vector3{90, 0, 0}
		}, "ListenerView"},
		{"spherical ListenerViews", minimalFIRFile, func(f *File) {
			setSphericalOrientation(f)
			f.ListenerViews = []Vector3{{0, 0, 1}, {90, 0, 1}, {180, 0, 1}}
		}, ""},
		{"unset ListenerView and ListenerUp", minimalFIRFile, func(f *File) {
			f.ListenerView, f.ListenerUp = Vector3{}, Vector3{}
		}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := tc.base()
			tc.mutate(f)
			err := f.validate()
			switch {
			case tc.want == "" && err != nil:
				t.Errorf("validate: %v", err)
			case tc.want != "" && err == nil:
				t.Errorf("validate accepted %s", tc.name)
			case tc.want != "" && !strings.Contains(err.Error(), tc.want):
				t.Errorf("error %q does not name %s", err, tc.want)
			}
		})
	}
}

// TestSaveDefaultsListenerOrientation checks that an unset ListenerView or
// ListenerUp is written as the conventions' default, in the file only.
func TestSaveDefaultsListenerOrientation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		typ      string
		view, up Vector3
		wantView []float64
		wantUp   []float64
	}{
		{"no type", "", Vector3{}, Vector3{}, []float64{1, 0, 0}, []float64{0, 0, 1}},
		{"cartesian", CoordinateCartesian, Vector3{}, Vector3{}, []float64{1, 0, 0}, []float64{0, 0, 1}},
		{"spherical", CoordinateSpherical, Vector3{}, Vector3{}, []float64{0, 0, 1}, []float64{0, 90, 1}},
		{"set view kept", CoordinateCartesian, Vector3{0, 1, 0}, Vector3{}, []float64{0, 1, 0}, []float64{0, 0, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := minimalFIRFile()
			f.ListenerViewType = tc.typ
			f.ListenerView, f.ListenerUp = tc.view, tc.up
			path := filepath.Join(t.TempDir(), "orientation.sofa")
			if err := f.Save(path); err != nil {
				t.Fatalf("Save: %v", err)
			}
			if f.ListenerView != tc.view || f.ListenerUp != tc.up {
				t.Errorf("Save changed the File: view %v, up %v", f.ListenerView, f.ListenerUp)
			}
			if got, _ := readRawDataset(t, path, datasetListenerView); !reflect.DeepEqual(got, tc.wantView) {
				t.Errorf("ListenerView = %v, want %v", got, tc.wantView)
			}
			if got, _ := readRawDataset(t, path, datasetListenerUp); !reflect.DeepEqual(got, tc.wantUp) {
				t.Errorf("ListenerUp = %v, want %v", got, tc.wantUp)
			}
		})
	}
}

// TestConventionConstraints checks the dimension and DataType constraints of
// the conventions with fixed layouts.
func TestConventionConstraints(t *testing.T) {
	for _, tc := range []struct {
		name string
		f    File
		want string // substring of the error; empty means valid
	}{
		{"SimpleFreeFieldHRIR", File{SOFAConventions: "SimpleFreeFieldHRIR", DataType: dataTypeFIR, R: 2, E: 1}, ""},
		{"SimpleFreeFieldHRIR one receiver", File{SOFAConventions: "SimpleFreeFieldHRIR", DataType: dataTypeFIR, R: 1, E: 1}, "R=2"},
		{"SimpleFreeFieldHRIR two emitters", File{SOFAConventions: "SimpleFreeFieldHRIR", DataType: dataTypeFIR, R: 2, E: 2}, "E=1"},
		{"SimpleFreeFieldHRIR as TF", File{SOFAConventions: "SimpleFreeFieldHRIR", DataType: dataTypeTF, R: 2, E: 1}, "DataType"},
		{"SimpleFreeFieldHRTF", File{SOFAConventions: "SimpleFreeFieldHRTF", DataType: dataTypeTF, R: 2, E: 1}, ""},
		{"SimpleFreeFieldHRTF three receivers", File{SOFAConventions: "SimpleFreeFieldHRTF", DataType: dataTypeTF, R: 3, E: 1}, "R=2"},
		{"SimpleFreeFieldHRSOS", File{SOFAConventions: "SimpleFreeFieldHRSOS", DataType: dataTypeSOS, R: 2, E: 1}, ""},
		{"SimpleFreeFieldHRSOS as FIR", File{SOFAConventions: "SimpleFreeFieldHRSOS", DataType: dataTypeFIR, R: 2, E: 1}, "DataType"},
		{"FreeFieldHRTF SH", File{SOFAConventions: "FreeFieldHRTF", DataType: dataTypeTFE, R: 2, E: 16}, ""},
		{"FreeFieldHRTF as TF", File{SOFAConventions: "FreeFieldHRTF", DataType: dataTypeTF, R: 2, E: 1}, "DataType"},
		{"FreeFieldDirectivityTF", File{SOFAConventions: "FreeFieldDirectivityTF", DataType: dataTypeTF, R: 7, E: 1}, ""},
		{"FreeFieldDirectivityTF as FIR", File{SOFAConventions: "FreeFieldDirectivityTF", DataType: dataTypeFIR, R: 7, E: 1}, "DataType"},
		{"unknown convention", File{SOFAConventions: "MyConvention", DataType: dataTypeFIR, R: 5, E: 3}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.f.validateConvention()
			switch {
			case tc.want == "" && err != nil:
				t.Errorf("validateConvention: %v", err)
			case tc.want != "" && err == nil:
				t.Errorf("validateConvention accepted %s", tc.name)
			case tc.want != "" && !strings.Contains(err.Error(), tc.want):
				t.Errorf("error %q does not mention %s", err, tc.want)
			}
		})
	}

	// The rules run as part of validate, i.e. on Save.
	f := minimalFIRFile()
	f.SOFAConventions = "SimpleFreeFieldHRTF"
	if err := f.validate(); err == nil || !strings.Contains(err.Error(), "DataType") {
		t.Errorf("validate of FIR data as SimpleFreeFieldHRTF: %v, want a DataType error", err)
	}
}

// setSphericalOrientation switches f's listener orientation to spherical
// coordinates: view to the front, up to the zenith, both at radius 1.
func setSphericalOrientation(f *File) {
	f.ListenerViewType = CoordinateSpherical
	f.ListenerView, f.ListenerUp = Vector3{0, 0, 1}, Vector3{0, 90, 1}
}
