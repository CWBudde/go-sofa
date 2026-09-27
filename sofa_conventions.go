package sofa

import "slices"

// SOFAConventions values of the official conventions not declared next to
// their specific checks (SingleRoomSRIR in sofa_srir.go, SingleRoomDRIR in
// sofa_brir.go). Each allows exactly one DataType, and the Simple* ones a
// fixed layout, as given by the SOFA Toolbox convention tables: "Data"
// dimensions mRn (R = 2 receivers, the ears) and "a single Emitter only"
// (E = 1).
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

// conventionAliases maps legacy SOFAConventions names to the convention
// whose rules validate them: SOFA 1.0 called SimpleFreeFieldHRSOS
// SimpleFreeFieldSOS.
var conventionAliases = map[string]string{
	"SimpleFreeFieldSOS": conventionSimpleFreeFieldHRSOS,
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
	r, e     int    // the required receiver and emitter counts; 0 means any

	validate func(f *File) error    // nil means no further checks
	warnings func(f *File) []string // nil means no advisory messages
	// mandatoryGlobals are global attributes Save writes as "" when
	// Attributes has none of that name.
	mandatoryGlobals []string
	// mandatoryVariables are variables Save writes, with these defaults,
	// when Variables has none of that name.
	mandatoryVariables []Variable
	// roomType returns the RoomType Save writes when the File has none;
	// nil means free field.
	roomType func(f *File) string
}

// conventionRegistry maps SOFAConventions values to their specific rules.
// Conventions not listed here (or in conventionAliases) get only the
// generic checks, so files using unknown or custom conventions keep
// writing unchanged.
var conventionRegistry = map[string]conventionRules{
	conventionGeneralFIR: {dataType: DataTypeFIR},
	conventionGeneralTF:  {dataType: DataTypeTF},
	conventionGeneralTFE: {dataType: DataTypeTFE},

	conventionSimpleFreeFieldHRIR:  {dataType: DataTypeFIR, r: 2, e: 1, mandatoryGlobals: globalsHRTF},
	conventionSimpleFreeFieldHRTF:  {dataType: DataTypeTF, r: 2, e: 1, mandatoryGlobals: globalsHRTF},
	conventionSimpleFreeFieldHRSOS: {dataType: DataTypeSOS, r: 2, e: 1, mandatoryGlobals: globalsHRTF},
	conventionFreeFieldHRTF:        {dataType: DataTypeTFE, mandatoryGlobals: globalsHRTF},
	conventionFreeFieldDirectivityTF: {
		dataType: DataTypeTF, mandatoryGlobals: globalsDirectivityTF,
		mandatoryVariables: sourceOrientation(Vector3{X: 1}),
	},
	conventionSimpleHeadphoneIR: {dataType: DataTypeFIR, mandatoryGlobals: globalsHeadphoneIR},

	conventionSingleRoomSRIR: {
		dataType: DataTypeFIR, warnings: srirWarnings, mandatoryGlobals: globalsSRIR,
		mandatoryVariables: sourceOrientation(Vector3{X: 1}), roomType: srirRoomType,
	},
	// The (deprecated) SingleRoomDRIR table points the source at the
	// listener (SourceView defaults to -x) and defaults RoomType to
	// reverberant.
	conventionSingleRoomDRIR: {
		dataType: DataTypeFIR, validate: validateBRIR, mandatoryGlobals: globalsDRIR,
		mandatoryVariables: sourceOrientation(Vector3{X: -1}),
		roomType:           func(*File) string { return roomTypeReverberant },
	},
}

// rulesFor returns the rules registered for the SOFAConventions value
// convention, or for the convention a legacy name stands for; ok is false
// for conventions without rules.
func rulesFor(convention string) (rules conventionRules, ok bool) {
	if name, isAlias := conventionAliases[convention]; isAlias {
		convention = name
	}
	rules, ok = conventionRegistry[convention]
	return rules, ok
}

// validateConvention checks f against the rules registered for its
// SOFAConventions, if any: the DataType, then the receiver and emitter
// counts, then the convention's own checks. Errors name f.SOFAConventions,
// a legacy name included.
func (f *File) validateConvention() error {
	rules, ok := rulesFor(f.SOFAConventions)
	if !ok {
		return nil
	}
	if rules.dataType != "" && f.DataType != rules.dataType {
		return invalid("DataType", "%s requires %s, got %q", f.SOFAConventions, rules.dataType, f.DataType)
	}
	if rules.r != 0 && f.R != rules.r {
		return invalid("R", "%s requires R=%d, got %d", f.SOFAConventions, rules.r, f.R)
	}
	if rules.e != 0 && f.E != rules.e {
		return invalid("E", "%s requires E=%d, got %d", f.SOFAConventions, rules.e, f.E)
	}
	if rules.validate == nil {
		return nil
	}
	return rules.validate(f)
}

// ConventionWarnings returns advisory messages from the rules of the file's
// SOFAConventions, such as missing optional room metadata. Unlike validation
// errors they never stop Save; callers should surface them to users. Empty
// for conventions without specific rules.
func (f *File) ConventionWarnings() []string {
	rules, ok := rulesFor(f.SOFAConventions)
	if !ok || rules.warnings == nil {
		return nil
	}
	return rules.warnings(f)
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
// for it.
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

// savedVariables returns the extra variables Save writes: f.Variables, then
// the defaults of the missing mandatory ones. f.Variables is not changed.
func (f *File) savedVariables() []Variable {
	return append(slices.Clip(f.Variables), f.missingMandatoryVariables()...)
}
