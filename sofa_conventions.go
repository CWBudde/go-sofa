package sofa

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
