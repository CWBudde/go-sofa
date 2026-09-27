package sofa

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// SOFAConventions values of the official conventions not declared next to
// their specific checks (SingleRoomSRIR in sofa_srir.go, SingleRoomDRIR in
// sofa_brir.go). Each allows exactly one DataType, as given by the SOFA
// Toolbox convention tables, and the Simple* ones "a single Emitter only"
// (E = 1). Their tables default to two receivers, the ears, but allow any
// number.
const (
	conventionGeneralFIR             = "GeneralFIR"
	conventionGeneralTF              = "GeneralTF"
	conventionGeneralTFE             = "GeneralTF-E"
	conventionSimpleFreeFieldHRIR    = "SimpleFreeFieldHRIR"
	conventionSimpleFreeFieldHRTF    = "SimpleFreeFieldHRTF"
	conventionSimpleFreeFieldHRSOS   = "SimpleFreeFieldHRSOS"
	conventionFreeFieldHRTF          = "FreeFieldHRTF"
	conventionFreeFieldDirectivityTF = "FreeFieldDirectivityTF"
	conventionSimpleHeadphoneIR      = "SimpleHeadphoneIR"
)

// conventionAlias names the convention whose rules validate a legacy
// SOFAConventions name, and the SOFAConventionsVersion values the legacy
// name itself had.
type conventionAlias struct {
	convention string
	versions   []string
}

// conventionAliases maps legacy SOFAConventions names to their
// conventionAlias: SOFA 1.0 called SimpleFreeFieldHRSOS SimpleFreeFieldSOS.
var conventionAliases = map[string]conventionAlias{
	"SimpleFreeFieldSOS": {conventionSimpleFreeFieldHRSOS, strings.Fields("1.0")},
}

// Global attributes some conventions make mandatory beyond the ones every
// convention does (see collectRootAttributes), as listed in the SOFA
// Toolbox convention tables (SOFAtoolbox/conventions/*.csv, latest
// versions). The tables give every one of them an empty default.
const (
	attrDatabaseName      = "DatabaseName"
	attrListenerShortName = "ListenerShortName"
)

var (
	globalsHRTF          = []string{attrDatabaseName, attrListenerShortName}
	globalsDirectivityTF = []string{attrDatabaseName, "SourceType", "SourceManufacturer"}
	globalsHeadphoneIR   = []string{attrDatabaseName, attrListenerShortName, "ReceiverDescription", "EmitterDescription"}
	globalsSRIR          = []string{attrDatabaseName}
	globalsDRIR          = []string{"RoomDescription", attrDatabaseName}
)

// attrReference is a variable attribute: a narrative description of the
// spatial reference of a source position or orientation.
const attrReference = "Reference"

// referenceDirectivityTF are the Reference attributes the
// FreeFieldDirectivityTF table makes mandatory.
var referenceDirectivityTF = map[string][]string{
	datasetSourcePosition: {attrReference},
	"SourceView":          {attrReference},
	"SourceUp":            {attrReference},
}

// sourceOrientation returns the SourceView and SourceUp variables with the
// defaults the convention tables give them where they make them mandatory:
// view (cartesian, in metres) as given, up along +z, one row for all
// measurements.
func sourceOrientation(view Vector3) []Variable {
	orientation := func(name string, v Vector3) Variable {
		return Variable{
			Name: name, Dims: []string{dimI, dimC}, Shape: []int{1, 3},
			Values:     []float64{v.X, v.Y, v.Z},
			Attributes: []Attribute{{"Type", CoordinateCartesian}, {attrUnits, UnitsCartesianMetres}},
		}
	}
	return []Variable{orientation("SourceView", view), orientation("SourceUp", Vector3{Z: 1})}
}

// conventionRules holds the requirements of one SOFAConventions value,
// checked by validate after the generic checks.
type conventionRules struct {
	dataType string // the one DataType allowed; "" means any
	// r is the receiver count renderers expect; another one gets a
	// warning. e is the required emitter count. 0 means any.
	r, e int
	// versions are the known SOFAConventionsVersion values, current and
	// deprecated, of the SOFA Toolbox and pyfar convention tables; another
	// one gets a warning. nil means any.
	versions []string

	validate func(f *File) error    // nil means no further checks
	warnings func(f *File) []string // nil means no advisory messages
	// mandatoryGlobals are global attributes Save writes as "" when
	// Attributes has none of that name.
	mandatoryGlobals []string
	// mandatoryVariables are variables Save writes, with these defaults,
	// when Variables has none of that name. They are shared by every file
	// of the convention: read-only.
	mandatoryVariables []Variable
	// mandatoryVariableAttributes names, per variable, attributes Save
	// writes as "" when the variable has none of that name.
	mandatoryVariableAttributes map[string][]string
	// roomType returns the RoomType Save writes when the File has none;
	// nil means free field.
	roomType func(f *File) string
	// freeFieldOnly means sofar rejects any RoomType but free field (the
	// SOFA Toolbox tables do not restrict it); another one gets a warning.
	freeFieldOnly bool
}

// conventionRegistry maps SOFAConventions values to their specific rules.
// Conventions not listed here (or in conventionAliases) get only the
// generic checks, so files using unknown or custom conventions keep
// writing unchanged.
var conventionRegistry = map[string]conventionRules{
	conventionGeneralFIR: {dataType: DataTypeFIR, versions: strings.Fields("1.0")},
	conventionGeneralTF:  {dataType: DataTypeTF, versions: strings.Fields("1.0 2.0")},
	conventionGeneralTFE: {dataType: DataTypeTFE, versions: strings.Fields("1.0")},

	conventionSimpleFreeFieldHRIR: {
		dataType: DataTypeFIR, r: 2, e: 1, versions: strings.Fields("0.4 1.0 1.1 1.2"),
		mandatoryGlobals: globalsHRTF, freeFieldOnly: true,
	},
	conventionSimpleFreeFieldHRTF: {
		dataType: DataTypeTF, r: 2, e: 1, versions: strings.Fields("1.0 1.1 1.2"),
		mandatoryGlobals: globalsHRTF, freeFieldOnly: true,
	},
	conventionSimpleFreeFieldHRSOS: {
		dataType: DataTypeSOS, r: 2, e: 1, versions: strings.Fields("1.0 1.1 1.2"),
		mandatoryGlobals: globalsHRTF, freeFieldOnly: true,
	},
	conventionFreeFieldHRTF: {
		dataType: DataTypeTFE, versions: strings.Fields("1.0"),
		mandatoryGlobals: globalsHRTF, freeFieldOnly: true,
	},
	conventionFreeFieldDirectivityTF: {
		dataType: DataTypeTF, versions: strings.Fields("1.0 1.1"), mandatoryGlobals: globalsDirectivityTF,
		mandatoryVariables: sourceOrientation(Vector3{X: 1}), mandatoryVariableAttributes: referenceDirectivityTF,
	},
	conventionSimpleHeadphoneIR: {
		dataType: DataTypeFIR, versions: strings.Fields("0.1 0.2 1.0 1.1"),
		mandatoryGlobals: globalsHeadphoneIR,
	},

	conventionSingleRoomSRIR: {
		dataType: DataTypeFIR, versions: strings.Fields("1.0 1.1"), warnings: srirWarnings,
		mandatoryGlobals: globalsSRIR, mandatoryVariables: sourceOrientation(Vector3{X: 1}),
		roomType: srirRoomType,
	},
	// The (deprecated) SingleRoomDRIR table points the source at the
	// listener (SourceView defaults to -x) and defaults RoomType to
	// reverberant.
	conventionSingleRoomDRIR: {
		dataType: DataTypeFIR, versions: strings.Fields("0.1 0.2 0.3"), validate: validateBRIR,
		mandatoryGlobals: globalsDRIR, mandatoryVariables: sourceOrientation(Vector3{X: -1}),
		roomType: func(*File) string { return roomTypeReverberant },
	},
}

// rulesFor returns the rules registered for the SOFAConventions value
// convention, or for the convention a legacy name stands for, with the
// legacy name's own versions; ok is false for conventions without rules.
func rulesFor(convention string) (rules conventionRules, ok bool) {
	alias, isAlias := conventionAliases[convention]
	if isAlias {
		convention = alias.convention
	}
	rules, ok = conventionRegistry[convention]
	if isAlias {
		rules.versions = alias.versions
	}
	return rules, ok
}

// validateConvention checks f against the rules registered for its
// SOFAConventions, if any: the DataType, then the emitter count, then the
// convention's own checks. Errors name f.SOFAConventions,
// a legacy name included.
func (f *File) validateConvention() error {
	rules, ok := rulesFor(f.SOFAConventions)
	if !ok {
		return nil
	}
	if rules.dataType != "" && f.DataType != rules.dataType {
		return invalid("DataType", "%s requires %s, got %q", f.SOFAConventions, rules.dataType, f.DataType)
	}
	if rules.e != 0 && f.E != rules.e {
		return invalid("E", "%s requires E=%d, got %d", f.SOFAConventions, rules.e, f.E)
	}
	if rules.validate == nil {
		return nil
	}
	return rules.validate(f)
}

// ConventionWarnings returns advisory messages about the file's
// conformance: SOFA 2.x features (the FreeFieldHRTF convention, DataType
// TF-E, spherical-harmonics positions) in a file whose Version is below
// 2.0, a SOFAConventionsVersion its registered convention does not know
// (custom conventions are not checked), a SimpleFreeField* file with other
// than two receivers (libmysofa rejects it), a free-field HRTF file whose
// RoomType is not free field (sofar rejects it), and the checks of the
// convention's own rules, such as missing optional room metadata. Unlike
// validation errors they never stop Save; callers should surface them to
// users.
func (f *File) ConventionWarnings() []string {
	out := f.sofa2Warnings()
	rules, ok := rulesFor(f.SOFAConventions)
	if !ok {
		return out
	}
	if v := strings.TrimSpace(f.SOFAConventionsVersion); v != "" && len(rules.versions) > 0 && !slices.Contains(rules.versions, v) {
		out = append(out, fmt.Sprintf("SOFAConventionsVersion %q is not a known version of %s (known: %s)",
			f.SOFAConventionsVersion, f.SOFAConventions, strings.Join(rules.versions, ", ")))
	}
	if rules.r != 0 && f.R > 0 && f.R != rules.r { // R <= 0 fails validation
		out = append(out, fmt.Sprintf("%s expects R=%d receivers (the ears), got R=%d; two-ear renderers such as libmysofa reject it",
			f.SOFAConventions, rules.r, f.R))
	}
	if roomType := f.savedRoomType(); rules.freeFieldOnly && roomType != roomTypeFreeField {
		out = append(out, fmt.Sprintf("%s expects RoomType %q, got %q; sofar rejects it",
			f.SOFAConventions, roomTypeFreeField, roomType))
	}
	if rules.warnings != nil {
		out = append(out, rules.warnings(f)...)
	}
	return out
}

// sofa2Warnings flags the features the SOFA conventions introduced with
// SOFA 2.0 when f.Version is a number below 2.0. Neither the SOFA Toolbox
// nor sofar gates on Version, so these are warnings only; an empty or
// unparsable Version gets none. SOS, which SimpleFreeFieldSOS used in SOFA
// 1.0, is no 2.x feature.
func (f *File) sofa2Warnings() []string {
	version, err := strconv.ParseFloat(strings.TrimSpace(f.Version), 64)
	if err != nil || !(version < 2) { // NaN included
		return nil
	}
	suffix := fmt.Sprintf(", but Version is %q", f.Version)
	var out []string
	if f.SOFAConventions == conventionFreeFieldHRTF {
		out = append(out, conventionFreeFieldHRTF+" is a SOFA 2.x convention"+suffix)
	}
	if f.DataType == DataTypeTFE {
		out = append(out, "DataType "+DataTypeTFE+" is a SOFA 2.x feature"+suffix)
	}
	for _, p := range f.savedPositions() {
		if strings.EqualFold(strings.TrimSpace(p.typ), CoordinateSphericalHarmonics) {
			out = append(out, fmt.Sprintf("%s Type %q is a SOFA 2.x feature%s", p.name, p.typ, suffix))
		}
	}
	return out
}

// missingMandatoryGlobals returns the mandatory global attributes of the
// file's SOFAConventions that Attributes lacks, each with the empty value
// Save writes for it.
func (f *File) missingMandatoryGlobals() []Attribute {
	rules, _ := rulesFor(f.SOFAConventions)
	var missing []Attribute
	for _, name := range rules.mandatoryGlobals {
		if !slices.ContainsFunc(f.Attributes, func(a Attribute) bool { return a.Name == name }) {
			missing = append(missing, Attribute{Name: name, Value: ""})
		}
	}
	return missing
}

// missingMandatoryVariables returns the mandatory variables of the file's
// SOFAConventions that Variables lacks, each with the default Save writes
// for it. They alias the registry's defaults: callers must not modify them.
func (f *File) missingMandatoryVariables() []Variable {
	rules, _ := rulesFor(f.SOFAConventions)
	var missing []Variable
	for _, v := range rules.mandatoryVariables {
		if !slices.ContainsFunc(f.Variables, func(have Variable) bool { return have.Name == v.Name }) {
			missing = append(missing, v)
		}
	}
	return missing
}

// defaultRoomType returns the RoomType Save writes when f.RoomType is
// empty: the default of the file's SOFAConventions, free field for
// conventions without one.
func (f *File) defaultRoomType() string {
	rules, _ := rulesFor(f.SOFAConventions)
	if rules.roomType == nil {
		return roomTypeFreeField
	}
	return rules.roomType(f)
}

// savedRoomType returns the RoomType Save writes: f.RoomType, or the
// convention's default when it is empty.
func (f *File) savedRoomType() string {
	if f.RoomType != "" {
		return f.RoomType
	}
	return f.defaultRoomType()
}

// savedVariables returns the extra variables Save writes: f.Variables, then
// the defaults of the missing mandatory ones with the attributes
// VariableAttributes holds for their names appended, each completed by
// withMandatoryAttributes. f.Variables is not changed.
func (f *File) savedVariables() []Variable {
	missing := f.missingMandatoryVariables()
	saved := make([]Variable, 0, len(f.Variables)+len(missing))
	for _, v := range f.Variables {
		v.Attributes = f.withMandatoryAttributes(v.Name, v.Attributes)
		saved = append(saved, v)
	}
	for _, v := range missing {
		attrs := append(slices.Clip(v.Attributes), f.VariableAttributes[v.Name]...)
		v.Attributes = f.withMandatoryAttributes(v.Name, attrs)
		saved = append(saved, v)
	}
	return saved
}

// savedVariableAttributes returns the VariableAttributes Save writes on the
// variables it writes itself (see writtenVariables): f.VariableAttributes,
// completed by withMandatoryAttributes. The variables of savedVariables
// carry theirs. f.VariableAttributes is not changed.
func (f *File) savedVariableAttributes() map[string][]Attribute {
	rules, _ := rulesFor(f.SOFAConventions)
	if len(rules.mandatoryVariableAttributes) == 0 {
		return f.VariableAttributes
	}
	extra := f.savedVariables()
	saved, cloned := f.VariableAttributes, false
	for name := range rules.mandatoryVariableAttributes {
		if slices.ContainsFunc(extra, func(v Variable) bool { return v.Name == name }) {
			continue
		}
		attrs := f.withMandatoryAttributes(name, f.VariableAttributes[name])
		if len(attrs) == len(f.VariableAttributes[name]) {
			continue
		}
		if !cloned {
			saved, cloned = maps.Clone(f.VariableAttributes), true
			if saved == nil {
				saved = map[string][]Attribute{}
			}
		}
		saved[name] = attrs
	}
	return saved
}

// withMandatoryAttributes returns attrs, the attributes Save writes on
// variable name, with the mandatory variable attributes of the file's
// SOFAConventions that attrs lacks appended as "". attrs is not changed.
func (f *File) withMandatoryAttributes(name string, attrs []Attribute) []Attribute {
	rules, _ := rulesFor(f.SOFAConventions)
	for _, a := range rules.mandatoryVariableAttributes[name] {
		if !slices.ContainsFunc(attrs, func(have Attribute) bool { return have.Name == a }) {
			attrs = append(slices.Clip(attrs), Attribute{Name: a, Value: ""})
		}
	}
	return attrs
}
