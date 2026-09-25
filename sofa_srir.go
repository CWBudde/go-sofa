package sofa

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	hdf5 "github.com/cwbudde/go-hdf5"
)

// SOFAConventions values for spatial room impulse responses.
const (
	conventionSingleRoomSRIR     = "SingleRoomSRIR"
	conventionSingleRoomMIMOSRIR = "SingleRoomMIMOSRIR"
)

// Room metadata variables and their units.
const (
	datasetRoomVolume      = "RoomVolume"
	datasetRoomTemperature = "RoomTemperature"
	unitsRoomVolume        = "cubic metre"
	unitsRoomTemperature   = "kelvin"
)

// srirRules are advisory only: SRIR files without room metadata, or recorded
// with a raw microphone array instead of Ambisonics channels, are valid.
var srirRules = conventionRules{warnings: srirWarnings}

// IsSRIR reports whether the file holds spatial room impulse responses, that
// is, whether SOFAConventions is SingleRoomSRIR or SingleRoomMIMOSRIR.
func (f *File) IsSRIR() bool {
	switch f.SOFAConventions {
	case conventionSingleRoomSRIR, conventionSingleRoomMIMOSRIR:
		return true
	}
	return false
}

// AmbisonicsOrder returns the Ambisonics order of an SRIR file, detected from
// its receiver count R = (order+1)². ok is false for files that are not SRIR,
// and for SRIR files whose R is not such a square, which usually means the
// receivers are the raw capsules of a microphone array.
func (f *File) AmbisonicsOrder() (order int, ok bool) {
	if !f.IsSRIR() || f.R < 1 {
		return 0, false
	}
	root := int(math.Round(math.Sqrt(float64(f.R))))
	if root*root != f.R {
		return 0, false
	}
	return root - 1, true
}

func srirWarnings(f *File) []string {
	var out []string
	if f.RoomVolume == 0 {
		out = append(out, f.SOFAConventions+" file has no RoomVolume")
	}
	if f.RoomTemperature == 0 {
		out = append(out, f.SOFAConventions+" file has no RoomTemperature")
	}
	if _, ok := f.AmbisonicsOrder(); !ok && f.R > 0 {
		out = append(out, fmt.Sprintf(
			"R=%d is not (order+1)² for any Ambisonics order; receivers are treated as unencoded channels", f.R))
	}
	return out
}

// parseRoomAttribute parses RoomVolume or RoomTemperature stored as a root
// attribute. Unparsable or non-finite values are treated as absent.
func parseRoomAttribute(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

// readRoomScalars reads the RoomVolume and RoomTemperature variables. A
// variable takes precedence over a root attribute of the same name, which
// readGlobalAttributes has already parsed.
func (f *File) readRoomScalars(datasets map[string]*hdf5.Dataset) {
	for _, rt := range []struct {
		name string
		dst  *float64
	}{
		{datasetRoomVolume, &f.RoomVolume},
		{datasetRoomTemperature, &f.RoomTemperature},
	} {
		ds, ok := datasets[rt.name]
		if !ok {
			continue
		}
		if data, err := ds.Read(); err == nil && len(data) > 0 {
			*rt.dst = data[0]
		}
	}
}

// writeRoomScalars writes RoomVolume and RoomTemperature as variables with
// their units. Zero values mean absent and are not written.
func (f *File) writeRoomScalars(fw *hdf5.FileWriter) error {
	for _, rt := range []struct {
		name, units string
		value       float64
	}{
		{datasetRoomVolume, unitsRoomVolume, f.RoomVolume},
		{datasetRoomTemperature, unitsRoomTemperature, f.RoomTemperature},
	} {
		if rt.value == 0 {
			continue
		}
		ds, err := fw.CreateDataset("/"+rt.name, hdf5.Float64, []uint64{1},
			hdf5.WithAttribute("Units", rt.units))
		if err != nil {
			return fmt.Errorf("create %s: %w", rt.name, err)
		}
		if err := ds.Write([]float64{rt.value}); err != nil {
			return fmt.Errorf("write %s: %w", rt.name, err)
		}
	}
	return nil
}
