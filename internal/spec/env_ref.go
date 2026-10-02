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
	// envRefTokenPattern is every `${...}` in a value. Import and emit
	// both classify these tokens through EnvRefTokens, so a token one
	// side keeps is a token the other side knows how to write or drop.
	envRefTokenPattern     = regexp.MustCompile(`\$\{([^}]*)\}`)
	envRefPlainPattern     = regexp.MustCompile(`^` + envRefName + `$`)
	envRefDefaultPattern   = regexp.MustCompile(`^` + envRefName + `:-(.*)$`)
	envRefDollarEnvPattern = regexp.MustCompile(`\$\{env:` + envRefName + `\}`)
	envRefBraceEnvPattern  = regexp.MustCompile(`(^|[^$])\{env:` + envRefName + `\}`)
	envRefUnbracedPattern  = regexp.MustCompile(`^\$` + envRefName + `$`)
	envRefPercentPattern   = regexp.MustCompile(`^%` + envRefName + `%$`)
)

// EnvRefToken is one `${...}` in a value.
type EnvRefToken struct {
	// Text is the token as written.
	Text string
	// Name is the variable, empty when the token is not `${NAME}` or
	// `${NAME:-default}`.
	Name string
	// Default is the fallback of `${NAME:-default}`.
	Default    string
	HasDefault bool
}

// Known reports whether the token is `${NAME}` or `${NAME:-default}`.
func (t EnvRefToken) Known() bool { return t.Name != "" }

// Display spells the token without its default value, which may be a
// secret.
func (t EnvRefToken) Display() string {
	switch {
	case !t.Known():
		return t.Text
	case t.HasDefault:
		return "${" + t.Name + ":-...}"
	}
	return EnvRef(t.Name)
}

// EnvRefTokens returns every `${...}` in value, in order.
func EnvRefTokens(value string) []EnvRefToken {
	var tokens []EnvRefToken
	for _, m := range envRefTokenPattern.FindAllStringSubmatch(value, -1) {
		t := EnvRefToken{Text: m[0]}
		if p := envRefPlainPattern.FindStringSubmatch(m[1]); p != nil {
			t.Name = p[1]
		} else if d := envRefDefaultPattern.FindStringSubmatch(m[1]); d != nil {
			t.Name, t.Default, t.HasDefault = d[1], d[2], true
		}
		tokens = append(tokens, t)
	}
	return tokens
}

// WholeEnvRef returns the variable name when value is exactly one
// `${NAME}`.
func WholeEnvRef(value string) (string, bool) {
	tokens := EnvRefTokens(value)
	if len(tokens) != 1 || tokens[0].Text != value || !tokens[0].Known() || tokens[0].HasDefault {
		return "", false
	}
	return tokens[0].Name, true
}

// HasEnvRef reports whether value holds any `${...}`.
func HasEnvRef(value string) bool {
	return envRefTokenPattern.MatchString(value)
}

// OnlyEnvRefs reports whether value is references and nothing else,
// ignoring whitespace and a leading `Bearer `. Any other text may be a
// secret written around a reference, such as a password in a URL.
func OnlyEnvRefs(value string) bool {
	if !HasEnvRef(value) {
		return false
	}
	rest := envRefTokenPattern.ReplaceAllString(strings.TrimPrefix(value, "Bearer "), "")
	return strings.TrimSpace(rest) == ""
}

// StripEnvRefDefaults turns each `${NAME:-default}` with a non-empty
// default into `${NAME}`, and returns the names it changed.
func StripEnvRefDefaults(value string) (string, []string) {
	var names []string
	out := envRefTokenPattern.ReplaceAllStringFunc(value, func(text string) string {
		t := EnvRefTokens(text)[0]
		if !t.HasDefault || t.Default == "" {
			return text
		}
		names = append(names, t.Name)
		return EnvRef(t.Name)
	})
	return out, names
}

// EnvRef renders a spec-form reference to name.
func EnvRef(name string) string {
	return "${" + name + "}"
}

// Write turns each `${NAME}` in value into this syntax. Other tokens are
// left as written; the caller decides whether the target reads them.
func (s EnvRefSyntax) Write(value string) string {
	return envRefTokenPattern.ReplaceAllStringFunc(value, func(text string) string {
		t := EnvRefTokens(text)[0]
		if !t.Known() || t.HasDefault {
			return text
		}
		switch s {
		case EnvRefDollarEnv:
			return "${env:" + t.Name + "}"
		case EnvRefBraceEnv:
			return "{env:" + t.Name + "}"
		}
		return text
	})
}

// EnvRefReading lists the extra forms a tool expands besides its own
// syntax, which import reads back. Each is read only as a whole value,
// the form the vendors document, so a literal such as `pa55$word` is
// never taken for a reference.
type EnvRefReading struct {
	// Unbraced is `$NAME`.
	Unbraced bool
	// Percent is `%NAME%`.
	Percent bool
}

// Read turns each reference in this syntax, and in the extra forms of
// r, back into the spec form.
func (s EnvRefSyntax) Read(value string, r EnvRefReading) string {
	switch s {
	case EnvRefDollarEnv:
		value = envRefDollarEnvPattern.ReplaceAllString(value, "$${$1}")
	case EnvRefBraceEnv:
		value = envRefBraceEnvPattern.ReplaceAllString(value, "$1$${$2}")
	}
	if r.Unbraced {
		value = envRefUnbracedPattern.ReplaceAllString(value, "$${$1}")
	}
	if r.Percent {
		value = envRefPercentPattern.ReplaceAllString(value, "$${$1}")
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
