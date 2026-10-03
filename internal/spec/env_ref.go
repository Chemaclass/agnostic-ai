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
	// EnvRefSecrets is `${{ secrets.NAME }}`.
	EnvRefSecrets
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
	envRefSecretsPattern   = regexp.MustCompile(`\$\{\{\s*secrets\.` + envRefName + `\s*\}\}`)
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

// editorVariables are the variables a VS Code style tool fills in
// itself in MCP `args` and `url`, such as `${workspaceFolder}`
// (cursor.com/docs/mcp#config-interpolation). They name no environment
// variable.
var editorVariables = map[string]bool{
	"workspaceFolder":         true,
	"workspaceFolderBasename": true,
	"userHome":                true,
	"pathSeparator":           true,
}

// EditorVariable reports whether the token is an editor variable such
// as `${workspaceFolder}` rather than an environment reference.
func (t EnvRefToken) EditorVariable() bool {
	return !t.HasDefault && editorVariables[t.Name]
}

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
	for _, m := range envRefTokenPattern.FindAllStringSubmatch(maskEscapes(value), -1) {
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
	return envRefTokenPattern.MatchString(maskEscapes(value))
}

// OnlyEnvRefs reports whether value is `${NAME}` or `${NAME:-default}`
// references and nothing else, ignoring whitespace and a leading
// `Bearer `. Any other text may be a secret written around a reference,
// such as a password in a URL, and any other `${...}` is one sync cannot
// write.
func OnlyEnvRefs(value string) bool {
	tokens := EnvRefTokens(value)
	if len(tokens) == 0 {
		return false
	}
	for _, t := range tokens {
		if !t.Known() {
			return false
		}
	}
	rest := envRefTokenPattern.ReplaceAllString(maskEscapes(strings.TrimPrefix(value, "Bearer ")), "")
	return strings.TrimSpace(rest) == ""
}

// StripEnvRefDefaults turns each `${NAME:-default}` with a non-empty
// default into `${NAME}`, and returns the names it changed.
func StripEnvRefDefaults(value string) (string, []string) {
	var names []string
	out := envRefTokenPattern.ReplaceAllStringFunc(maskEscapes(value), func(text string) string {
		t := EnvRefTokens(text)[0]
		if !t.HasDefault || t.Default == "" {
			return text
		}
		names = append(names, t.Name)
		return EnvRef(t.Name)
	})
	return unmaskEscapes(out), names
}

// EnvRef renders a spec-form reference to name.
func EnvRef(name string) string {
	return "${" + name + "}"
}

// envRefEscape is how a spec writes a literal `${`: `$${NAME}` reaches
// every tool as the text `${NAME}`, never as a reference. Compose and
// Terraform escape the same way.
const envRefEscape = "$${"

// escapeMask stands in for an escape while a value is tokenized, so no
// pattern reads the `${` inside it.
const escapeMask = "\x00{"

var escapedPlaceholderPattern = regexp.MustCompile(`\$\$\{` + envRefName + `\}`)

func maskEscapes(value string) string {
	return strings.ReplaceAll(value, envRefEscape, escapeMask)
}

func unmaskEscapes(value string) string {
	return strings.ReplaceAll(value, escapeMask, envRefEscape)
}

// DecodeEnvRefEscapes turns each `$${` into the literal `${` a tool
// receives. Any other `$$` stays as written.
func DecodeEnvRefEscapes(value string) string {
	return strings.ReplaceAll(value, envRefEscape, "${")
}

// EscapeEnvRefs writes each `${NAME}` and `${NAME:-default}` in value as
// `$${...}`, for import from a field the tool never expands. Editor
// variables such as `${workspaceFolder}` and other tokens stay as
// written.
func EscapeEnvRefs(value string) string {
	out := envRefTokenPattern.ReplaceAllStringFunc(maskEscapes(value), func(text string) string {
		t := EnvRefTokens(text)[0]
		if !t.Known() || t.EditorVariable() {
			return text
		}
		return "$" + text
	})
	return unmaskEscapes(out)
}

// OnlyEscapedEnvRefs reports whether value is escaped `$${NAME}`
// placeholders and nothing else, ignoring whitespace and a leading
// `Bearer `. A default such as `$${NAME:-x}` is text that may be a
// secret, so it does not count.
func OnlyEscapedEnvRefs(value string) bool {
	value = strings.TrimPrefix(value, "Bearer ")
	if !strings.Contains(value, envRefEscape) {
		return false
	}
	return strings.TrimSpace(escapedPlaceholderPattern.ReplaceAllString(value, "")) == ""
}

// Write turns each `${NAME}` in value into this syntax. Other tokens are
// left as written; the caller decides whether the target reads them.
func (s EnvRefSyntax) Write(value string) string {
	return s.write(value, false)
}

// WriteLaunch is Write for an MCP `url` or `args` element, which also
// keeps each editor variable as written.
func (s EnvRefSyntax) WriteLaunch(value string) string {
	return s.write(value, true)
}

func (s EnvRefSyntax) write(value string, keepEditorVariables bool) string {
	return unmaskEscapes(envRefTokenPattern.ReplaceAllStringFunc(maskEscapes(value), func(text string) string {
		t := EnvRefTokens(text)[0]
		if !t.Known() || t.HasDefault || (keepEditorVariables && t.EditorVariable()) {
			return text
		}
		switch s {
		case EnvRefDollarEnv:
			return "${env:" + t.Name + "}"
		case EnvRefBraceEnv:
			return "{env:" + t.Name + "}"
		case EnvRefSecrets:
			return "${{ secrets." + t.Name + " }}"
		}
		return text
	}))
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
	return s.read(value, r, false)
}

// ReadLaunch is Read for an MCP `url` or `args` element. A reference to
// a variable named like an editor variable, such as Cursor's
// `${env:workspaceFolder}`, stays as written: read back, it would turn
// into the editor variable.
func (s EnvRefSyntax) ReadLaunch(value string, r EnvRefReading) string {
	return s.read(value, r, true)
}

func (s EnvRefSyntax) read(value string, r EnvRefReading, keepEditorNames bool) string {
	value = maskEscapes(value)
	replace := func(p *regexp.Regexp, value string) string {
		return p.ReplaceAllStringFunc(value, func(text string) string {
			m := p.FindStringSubmatch(text)
			prefix, name := "", m[len(m)-1]
			if len(m) == 3 {
				prefix = m[1]
			}
			if keepEditorNames && editorVariables[name] {
				return text
			}
			return prefix + EnvRef(name)
		})
	}
	switch s {
	case EnvRefDollarEnv:
		value = replace(envRefDollarEnvPattern, value)
	case EnvRefBraceEnv:
		value = replace(envRefBraceEnvPattern, value)
	case EnvRefSecrets:
		value = replace(envRefSecretsPattern, value)
	}
	if r.Unbraced {
		value = replace(envRefUnbracedPattern, value)
	}
	if r.Percent {
		value = replace(envRefPercentPattern, value)
	}
	return unmaskEscapes(value)
}

// LaunchRefs returns each reference in this syntax that ReadLaunch reads
// back, as written in value.
func (s EnvRefSyntax) LaunchRefs(value string) []string {
	var p *regexp.Regexp
	switch s {
	case EnvRefDollarEnv:
		p = envRefDollarEnvPattern
	case EnvRefBraceEnv:
		p = envRefBraceEnvPattern
	case EnvRefSecrets:
		p = envRefSecretsPattern
	default:
		return nil
	}
	var refs []string
	for _, m := range p.FindAllStringSubmatch(value, -1) {
		if editorVariables[m[len(m)-1]] {
			continue
		}
		text := m[0]
		if len(m) == 3 {
			text = text[len(m[1]):]
		}
		refs = append(refs, text)
	}
	return refs
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
