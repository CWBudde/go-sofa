package sofa

// SOFAConventions value for binaural room impulse responses. MultiSpeakerBRIR
// is not listed: its DataType FIRE (legacy FIR-E) is rejected by Open and Save.
const conventionSingleRoomDRIR = "SingleRoomDRIR"

// brirRules require what a binaural room response cannot be interpreted
// without: the room it was measured in and the orientation of the head.
var brirRules = conventionRules{validate: validateBRIR}

// IsBRIR reports whether the file holds binaural room impulse responses,
// that is, whether SOFAConventions is SingleRoomDRIR. Save requires such
// files to carry a RoomType and a non-zero ListenerView and ListenerUp.
// MultiSpeakerBRIR files use DataType FIRE, which is not supported.
func (f *File) IsBRIR() bool {
	return f.SOFAConventions == conventionSingleRoomDRIR
}

func validateBRIR(f *File) error {
	if f.RoomType == "" {
		return invalid("RoomType", "%s requires the RoomType attribute", f.SOFAConventions)
	}
	if f.ListenerView == (Vector3{}) {
		return invalid("ListenerView", "%s requires a non-zero ListenerView", f.SOFAConventions)
	}
	if f.ListenerUp == (Vector3{}) {
		return invalid("ListenerUp", "%s requires a non-zero ListenerUp", f.SOFAConventions)
	}
	return nil
}
