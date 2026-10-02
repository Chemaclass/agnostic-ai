package spec

import (
	"regexp"
	"strings"
)

// EnvRefSyntax is how a tool's config file spells a reference to an
// environment variable. Specs always use EnvRefDollar.
type EnvRefSyntax int

const (
	// EnvRefNone means the field has no documented reference form.
	EnvRefNone EnvRefSyntax = iota
	// EnvRefDollar is `${NAME}`.
	EnvRefDollar
	// EnvRefDollarEnv is `${env:NAME}`.
	EnvRefDollarEnv
	// EnvRefBraceEnv is `{env:NAME}`.
	EnvRefBraceEnv
)

const envRefName = `([A-Za-z_][A-Za-z0-9_]*)`

var (
	envRefPattern          = regexp.MustCompile(`\$\{` + envRefName + `\}`)
	envRefDollarEnvPattern = regexp.MustCompile(`\$\{env:` + envRefName + `\}`)
	envRefBraceEnvPattern  = regexp.MustCompile(`(^|[^$])\{env:` + envRefName + `\}`)
	envRefUnbracedPattern  = regexp.MustCompile(`^\$` + envRefName + `$`)
	envRefAnyPattern       = regexp.MustCompile(`\$\{[^}]+\}`)
)

// EnvRefNames returns the variables value references in the spec form,
// in order of appearance.
func EnvRefNames(value string) []string {
	var names []string
	for _, m := range envRefPattern.FindAllStringSubmatch(value, -1) {
		names = append(names, m[1])
	}
	return names
}

// WholeEnvRef returns the variable name when value is exactly one
// spec-form reference.
func WholeEnvRef(value string) (string, bool) {
	m := envRefPattern.FindStringSubmatch(value)
	if m == nil || m[0] != value {
		return "", false
	}
	return m[1], true
}

// HasEnvRef reports whether value already reads from the environment:
// any `${...}`, including forms with a default such as `${NAME:-x}`.
func HasEnvRef(value string) bool {
	return envRefAnyPattern.MatchString(value)
}

// EnvRef renders a spec-form reference to name.
func EnvRef(name string) string {
	return "${" + name + "}"
}

// Write turns each spec-form reference in value into this syntax.
func (s EnvRefSyntax) Write(value string) string {
	switch s {
	case EnvRefDollarEnv:
		return envRefPattern.ReplaceAllString(value, "$${env:$1}")
	case EnvRefBraceEnv:
		return envRefPattern.ReplaceAllString(value, "{env:$1}")
	}
	return value
}

// Read turns each reference in this syntax back into the spec form.
// unbraced also reads a whole value of `$NAME`, for a tool that expands
// that form.
func (s EnvRefSyntax) Read(value string, unbraced bool) string {
	switch s {
	case EnvRefDollarEnv:
		value = envRefDollarEnvPattern.ReplaceAllString(value, "$${$1}")
	case EnvRefBraceEnv:
		value = envRefBraceEnvPattern.ReplaceAllString(value, "$1$${$2}")
	}
	if unbraced {
		value = envRefUnbracedPattern.ReplaceAllString(value, "$${$1}")
	}
	return value
}

// EnvVarName turns s into a variable name: letters, digits, and
// underscores, never starting with a digit.
func EnvVarName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	name := b.String()
	if name == "" || (name[0] >= '0' && name[0] <= '9') {
		name = "_" + name
	}
	return name
}
