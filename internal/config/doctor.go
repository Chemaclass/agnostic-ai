package config

// DoctorConfig holds declarative knobs for `doctor`'s opt-in checks.
type DoctorConfig struct {
	// CheckReferences tunes `doctor --check-references`.
	CheckReferences CheckReferencesConfig `yaml:"check-references,omitempty" json:"check-references,omitempty"`
}

// CheckReferencesConfig lists link destinations `doctor --check-references`
// never reports as broken, no matter what is on disk.
type CheckReferencesConfig struct {
	// Ignore matches a link destination exactly as written in the source
	// Markdown, such as "url" for a placeholder link (`[Logs](url)`) in an
	// example template, or a path.Match glob. Matching follows the same
	// rules as sync.unmanaged.
	Ignore []string `yaml:"ignore,omitempty" json:"ignore,omitempty"`
}

// IgnoresReference reports whether dest is exempted from
// doctor --check-references by doctor.check-references.ignore. Nil-safe.
func (c *Config) IgnoresReference(dest string) bool {
	return c != nil && MatchUnmanaged(c.Doctor.CheckReferences.Ignore, dest)
}
