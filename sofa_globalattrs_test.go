package sofa

import (
	"bytes"
	"maps"
	"slices"
	"strconv"
	"testing"

	hdf5 "github.com/cwbudde/go-hdf5"
)

// TestOpenReadsEveryGlobalField writes a file with a distinct value for
// every global attribute globalFields maps to a File field and checks
// that OpenReader reads each one back into its field.
//
// RoomVolume and RoomTemperature are written as root attributes rather
// than variables (some writers do so, and Save rejects them in
// f.Attributes), so the file is written with writeHDF5, which does not
// validate.
func TestOpenReadsEveryGlobalField(t *testing.T) {
	want := map[string]string{
		"Conventions":            "SOFA",
		"Version":                "2.1",
		"SOFAConventions":        "GeneralFIR",
		"SOFAConventionsVersion": "1.0",
		"DataType":               DataTypeFIR,
		"RoomType":               "reverberant",
		datasetRoomVolume:        "103.5",
		datasetRoomTemperature:   "293.15",
		"Title":                  "global attribute fixture",
		"DateCreated":            "2026-01-02 03:04:05",
		"DateModified":           "2026-06-07 08:09:10",
		"APIName":                "test-api",
		"APIVersion":             "9.8.7",
		"AuthorContact":          "author@example.org",
		"Organization":           "Example Org",
		"License":                "CC-BY-4.0",
		"ApplicationName":        "test-app",
		"ApplicationVersion":     "1.2.3",
		"Comment":                "a comment",
		"History":                "a history",
		"References":             "a reference",
		"Origin":                 "an origin",
	}
	var empty File
	if got, exp := slices.Sorted(maps.Keys(empty.globalFields())), slices.Sorted(maps.Keys(want)); !slices.Equal(got, exp) {
		t.Fatalf("globalFields keys = %v, test covers %v", got, exp)
	}

	f := robustFIRFile()
	f.Conventions = want["Conventions"]
	f.Version = want["Version"]
	f.SOFAConventions = want["SOFAConventions"]
	f.SOFAConventionsVersion = want["SOFAConventionsVersion"]
	f.DataType = want["DataType"]
	f.RoomType = want["RoomType"]
	f.Title = want["Title"]
	f.DateCreated = want["DateCreated"]
	f.DateModified = want["DateModified"]
	f.APIName = want["APIName"]
	f.APIVersion = want["APIVersion"]
	f.AuthorContact = want["AuthorContact"]
	f.Organization = want["Organization"]
	f.License = want["License"]
	f.ApplicationName = want["ApplicationName"]
	f.ApplicationVersion = want["ApplicationVersion"]
	f.Comment = want["Comment"]
	f.History = want["History"]
	f.References = want["References"]
	f.Origin = want["Origin"]
	f.Attributes = []Attribute{
		{Name: datasetRoomVolume, Value: want[datasetRoomVolume]},
		{Name: datasetRoomTemperature, Value: want[datasetRoomTemperature]},
	}

	var buf bytes.Buffer
	create := func(opts []interface{}) (*hdf5.FileWriter, error) {
		return hdf5.CreateForWriteTo(&buf, opts...)
	}
	if err := f.writeHDF5(create, nil); err != nil {
		t.Fatalf("writeHDF5: %v", err)
	}
	g, err := OpenReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer g.Close()

	room := func(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
	got := map[string]string{
		"Conventions":            g.Conventions,
		"Version":                g.Version,
		"SOFAConventions":        g.SOFAConventions,
		"SOFAConventionsVersion": g.SOFAConventionsVersion,
		"DataType":               g.DataType,
		"RoomType":               g.RoomType,
		datasetRoomVolume:        room(g.RoomVolume),
		datasetRoomTemperature:   room(g.RoomTemperature),
		"Title":                  g.Title,
		"DateCreated":            g.DateCreated,
		"DateModified":           g.DateModified,
		"APIName":                g.APIName,
		"APIVersion":             g.APIVersion,
		"AuthorContact":          g.AuthorContact,
		"Organization":           g.Organization,
		"License":                g.License,
		"ApplicationName":        g.ApplicationName,
		"ApplicationVersion":     g.ApplicationVersion,
		"Comment":                g.Comment,
		"History":                g.History,
		"References":             g.References,
		"Origin":                 g.Origin,
	}
	for _, name := range slices.Sorted(maps.Keys(want)) {
		if got[name] != want[name] {
			t.Errorf("%s = %q, want %q", name, got[name], want[name])
		}
	}
	if len(g.Attributes) != 0 {
		t.Errorf("Attributes = %v, want none (all mapped to fields)", g.Attributes)
	}
}
