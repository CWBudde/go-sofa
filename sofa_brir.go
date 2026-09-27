package sofa

// SOFAConventions value for binaural room impulse responses. MultiSpeakerBRIR
// is not listed: its DataType FIRE (legacy FIR-E) is rejected by Open and Save.
const conventionSingleRoomDRIR = "SingleRoomDRIR"

// IsBRIR reports whether the file holds binaural room impulse responses,
// that is, whether SOFAConventions is SingleRoomDRIR. Save requires such
// files to hold FIR data and to carry a non-zero ListenerView and
// ListenerUp; an empty RoomType is written as reverberant.
// MultiSpeakerBRIR files use DataType FIRE, which is not supported.
func (f *File) IsBRIR() bool {
	return f.SOFAConventions == conventionSingleRoomDRIR
}

// validateBRIR requires what a binaural room response cannot be
// interpreted without: the orientation of the head. An empty RoomType gets
// the convention's default instead (see defaultRoomType).
func validateBRIR(f *File) error {
	if f.ListenerView == (Vector3{}) {
		return invalid("ListenerView", "%s requires a non-zero ListenerView", f.SOFAConventions)
	}
	if f.ListenerUp == (Vector3{}) {
		return invalid("ListenerUp", "%s requires a non-zero ListenerUp", f.SOFAConventions)
	}
	return nil
}
