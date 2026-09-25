package sofa

import "testing"

func TestIsDirectivity(t *testing.T) {
	for convention, want := range map[string]bool{
		"FreeFieldDirectivityTF": true,
		"SimpleFreeFieldHRIR":    false,
		conventionSingleRoomSRIR: false,
		"":                       false,
	} {
		f := &File{SOFAConventions: convention}
		if got := f.IsDirectivity(); got != want {
			t.Errorf("IsDirectivity() for %q = %v, want %v", convention, got, want)
		}
	}
}
