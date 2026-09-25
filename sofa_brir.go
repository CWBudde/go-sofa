package sofa

import "fmt"

// SOFAConventions values for binaural room impulse responses.
const (
	conventionSingleRoomDRIR   = "SingleRoomDRIR"
	conventionMultiSpeakerBRIR = "MultiSpeakerBRIR"
)

// brirRules require what a binaural room response cannot be interpreted
// without: the room it was measured in and the orientation of the head.
var brirRules = conventionRules{validate: validateBRIR}

// IsBRIR reports whether the file holds binaural room impulse responses,
// that is, whether SOFAConventions is SingleRoomDRIR or MultiSpeakerBRIR.
// Save requires such files to carry a RoomType and a non-zero ListenerView
// and ListenerUp.
func (f *File) IsBRIR() bool {
	switch f.SOFAConventions {
	case conventionSingleRoomDRIR, conventionMultiSpeakerBRIR:
		return true
	}
	return false
}

func validateBRIR(f *File) error {
	if f.RoomType == "" {
		return fmt.Errorf("%s requires the RoomType attribute", f.SOFAConventions)
	}
	if f.ListenerView == (Vector3{}) {
		return fmt.Errorf("%s requires a non-zero ListenerView", f.SOFAConventions)
	}
	if f.ListenerUp == (Vector3{}) {
		return fmt.Errorf("%s requires a non-zero ListenerUp", f.SOFAConventions)
	}
	return nil
}
