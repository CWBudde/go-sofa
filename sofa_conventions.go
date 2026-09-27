package sofa

import "slices"

// Conventions with a fixed DataType, and for the Simple* ones a fixed
// layout, as given by the SOFA Toolbox convention tables: "Data" dimensions
// mRn (R = 2 receivers, the ears) and "a single Emitter only" (E = 1).
const (
	conventionSimpleFreeFieldHRIR    = "SimpleFreeFieldHRIR"
	conventionSimpleFreeFieldHRTF    = "SimpleFreeFieldHRTF"
	conventionSimpleFreeFieldHRSOS   = "SimpleFreeFieldHRSOS"
	conventionFreeFieldHRTF          = "FreeFieldHRTF"
	conventionFreeFieldDirectivityTF = "FreeFieldDirectivityTF"
	conventionSimpleHeadphoneIR      = "SimpleHeadphoneIR"
)

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
)

// conventionRules holds checks specific to one SOFAConventions value, run by
// validate after the generic checks.
type conventionRules struct {
	validate func(f *File) error    // nil means no extra checks
	warnings func(f *File) []string // nil means no advisory messages
	// mandatoryGlobals are global attributes Save writes as "" when
	// Attributes has none of that name.
	mandatoryGlobals []string
}

// withGlobals returns r with mandatoryGlobals set to names.
func (r conventionRules) withGlobals(names []string) conventionRules {
	r.mandatoryGlobals = names
	return r
}

// conventionRegistry maps SOFAConventions values to their specific rules.
// Conventions not listed here get only the generic checks, so files using
// unknown or custom conventions keep writing unchanged.
var conventionRegistry = map[string]conventionRules{
	conventionSingleRoomDRIR:     brirRules,
	conventionMultiSpeakerBRIR:   brirRules,
	conventionSingleRoomSRIR:     srirRules.withGlobals(globalsSRIR),
	conventionSingleRoomMIMOSRIR: srirRules,
	conventionSimpleHeadphoneIR:  conventionRules{}.withGlobals(globalsHeadphoneIR),

	conventionSimpleFreeFieldHRIR:    layoutRules(DataTypeFIR, 2, 1).withGlobals(globalsHRTF),
	conventionSimpleFreeFieldHRTF:    layoutRules(DataTypeTF, 2, 1).withGlobals(globalsHRTF),
	conventionSimpleFreeFieldHRSOS:   layoutRules(DataTypeSOS, 2, 1).withGlobals(globalsHRTF),
	conventionFreeFieldHRTF:          layoutRules(DataTypeTFE, 0, 0).withGlobals(globalsHRTF),
	conventionFreeFieldDirectivityTF: layoutRules(DataTypeTF, 0, 0).withGlobals(globalsDirectivityTF),
}

// layoutRules requires DataType dataType and, when non-zero, exactly r
// receivers and e emitters.
func layoutRules(dataType string, r, e int) conventionRules {
	return conventionRules{validate: func(f *File) error {
		if f.DataType != dataType {
			return invalid("DataType", "%s requires %s, got %q", f.SOFAConventions, dataType, f.DataType)
		}
		if r != 0 && f.R != r {
			return invalid("R", "%s requires R=%d, got %d", f.SOFAConventions, r, f.R)
		}
		if e != 0 && f.E != e {
			return invalid("E", "%s requires E=%d, got %d", f.SOFAConventions, e, f.E)
		}
		return nil
	}}
}

// validateConvention runs the rules registered for f.SOFAConventions, if any.
func (f *File) validateConvention() error {
	rules, ok := conventionRegistry[f.SOFAConventions]
	if !ok || rules.validate == nil {
		return nil
	}
	return rules.validate(f)
}

// ConventionWarnings returns advisory messages from the rules of the file's
// SOFAConventions, such as missing optional room metadata. Unlike validation
// errors they never stop Save; callers should surface them to users. Empty
// for conventions without specific rules.
func (f *File) ConventionWarnings() []string {
	rules, ok := conventionRegistry[f.SOFAConventions]
	if !ok || rules.warnings == nil {
		return nil
	}
	return rules.warnings(f)
}

// missingMandatoryGlobals returns the mandatory global attributes of the
// file's SOFAConventions that Attributes lacks, each with the empty value
// Save writes for it.
func (f *File) missingMandatoryGlobals() []Attribute {
	var missing []Attribute
	for _, name := range conventionRegistry[f.SOFAConventions].mandatoryGlobals {
		if !slices.ContainsFunc(f.Attributes, func(a Attribute) bool { return a.Name == name }) {
			missing = append(missing, Attribute{Name: name, Value: ""})
		}
	}
	return missing
}
