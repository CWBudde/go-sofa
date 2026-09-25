package sofa

// conventionRules holds checks specific to one SOFAConventions value, run by
// validate after the generic checks.
type conventionRules struct {
	validate func(f *File) error // nil means no extra checks
}

// conventionRegistry maps SOFAConventions values to their specific rules.
// Conventions not listed here get only the generic checks, so files using
// unknown or custom conventions keep writing unchanged.
var conventionRegistry = map[string]conventionRules{}

// validateConvention runs the rules registered for f.SOFAConventions, if any.
func (f *File) validateConvention() error {
	rules, ok := conventionRegistry[f.SOFAConventions]
	if !ok || rules.validate == nil {
		return nil
	}
	return rules.validate(f)
}
