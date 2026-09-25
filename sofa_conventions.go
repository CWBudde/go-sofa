package sofa

import "fmt"

// Conventions with a fixed DataType, and for the Simple* ones a fixed
// layout, as given by the SOFA Toolbox convention tables: "Data" dimensions
// mRn (R = 2 receivers, the ears) and "a single Emitter only" (E = 1).
const (
	conventionSimpleFreeFieldHRIR    = "SimpleFreeFieldHRIR"
	conventionSimpleFreeFieldHRTF    = "SimpleFreeFieldHRTF"
	conventionSimpleFreeFieldHRSOS   = "SimpleFreeFieldHRSOS"
	conventionFreeFieldHRTF          = "FreeFieldHRTF"
	conventionFreeFieldDirectivityTF = "FreeFieldDirectivityTF"
)

// conventionRules holds checks specific to one SOFAConventions value, run by
// validate after the generic checks.
type conventionRules struct {
	validate func(f *File) error    // nil means no extra checks
	warnings func(f *File) []string // nil means no advisory messages
}

// conventionRegistry maps SOFAConventions values to their specific rules.
// Conventions not listed here get only the generic checks, so files using
// unknown or custom conventions keep writing unchanged.
var conventionRegistry = map[string]conventionRules{
	conventionSingleRoomDRIR:     brirRules,
	conventionMultiSpeakerBRIR:   brirRules,
	conventionSingleRoomSRIR:     srirRules,
	conventionSingleRoomMIMOSRIR: srirRules,

	conventionSimpleFreeFieldHRIR:    layoutRules(DataTypeFIR, 2, 1),
	conventionSimpleFreeFieldHRTF:    layoutRules(DataTypeTF, 2, 1),
	conventionSimpleFreeFieldHRSOS:   layoutRules(DataTypeSOS, 2, 1),
	conventionFreeFieldHRTF:          layoutRules(DataTypeTFE, 0, 0),
	conventionFreeFieldDirectivityTF: layoutRules(DataTypeTF, 0, 0),
}

// layoutRules requires DataType dataType and, when non-zero, exactly r
// receivers and e emitters.
func layoutRules(dataType string, r, e int) conventionRules {
	return conventionRules{validate: func(f *File) error {
		if f.DataType != dataType {
			return fmt.Errorf("%s requires DataType %s, got %q", f.SOFAConventions, dataType, f.DataType)
		}
		if r != 0 && f.R != r {
			return fmt.Errorf("%s requires R=%d receivers, got %d", f.SOFAConventions, r, f.R)
		}
		if e != 0 && f.E != e {
			return fmt.Errorf("%s requires E=%d emitter, got %d", f.SOFAConventions, e, f.E)
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
