package sofa

import "strings"

// IsDirectivity reports whether the file holds source directivities, that is,
// whether SOFAConventions names a Directivity convention such as
// FreeFieldDirectivityTF.
//
// In these files the measurement dimension M indexes the orientation of the
// source being characterised, not a source position around a listener as in
// HRTF sets. Save only checks that FreeFieldDirectivityTF files hold TF
// data; further rules wait for an example file to check them against.
func (f *File) IsDirectivity() bool {
	return strings.Contains(f.SOFAConventions, "Directivity")
}
