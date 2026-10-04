package emit

import (
	"fmt"
	"regexp"
	"sort"
)

// Variable names expanded in spec bodies. Each maps to a path the target
// actually emits to, so one spec can say "put skills in {{$SKILLS_DIR}}"
// and every target reads its own location.
const (
	VarSkillsDir   = "SKILLS_DIR"
	VarAgentsDir   = "AGENTS_DIR"
	VarCommandsDir = "COMMANDS_DIR"
	VarRulesDir    = "RULES_DIR"
	VarMCPFile     = "MCP_FILE"
)

// varPattern matches {{$NAME}} with an uppercase name. The $ sigil is
// what keeps this from eating real content: Warp workflows use
// {{placeholder}} for their arguments, and specs quote Handlebars and
// Jinja in prose. Both survive untouched.
var varPattern = regexp.MustCompile(`\{\{\$([A-Z][A-Z0-9_]*)\}\}`)

// ExpandVars replaces every {{$NAME}} in body with vals[NAME] and
// returns the result plus the distinct names it could not resolve,
// sorted. An unresolved name keeps its token verbatim rather than
// collapsing to an empty string, which would silently turn
// "see {{$COMMANDS_DIR}}" into "see " on a target with no commands
// surface. An empty value counts as unresolved for the same reason: it
// means the target declares the surface but has it switched off.
func ExpandVars(body string, vals map[string]string) (string, []string) {
	if !varPattern.MatchString(body) {
		return body, nil
	}
	missing := map[string]bool{}
	out := varPattern.ReplaceAllStringFunc(body, func(match string) string {
		name := varPattern.FindStringSubmatch(match)[1]
		if v := vals[name]; v != "" {
			return v
		}
		missing[name] = true
		return match
	})
	if len(missing) == 0 {
		return out, nil
	}
	names := make([]string, 0, len(missing))
	for n := range missing {
		names = append(names, n)
	}
	sort.Strings(names)
	return out, names
}

// Reference keywords for {{$AGENT:<name>}} and {{$SKILL:<name>}}, which
// name another spec by how the target invokes it rather than by path.
const (
	RefAgent = "AGENT"
	RefSkill = "SKILL"
)

// RefPattern matches {{$AGENT:<name>}} and {{$SKILL:<name>}}. The name
// is whatever sits before the braces close, so a misspelled one still
// matches and lint can name it.
var RefPattern = regexp.MustCompile(`\{\{\$(AGENT|SKILL):([^{}\s]+)\}\}`)

// ExpandRefs replaces every reference in body with forms[keyword] filled
// with the spec name, and returns the result plus the distinct keywords
// with no form, sorted. A keyword with no form renders a neutral phrase
// ("the reviewer agent") instead of staying verbatim like a path
// variable: a raw token in a prompt reads worse to a model than plain
// words.
func ExpandRefs(body string, forms map[string]string) (string, []string) {
	if !RefPattern.MatchString(body) {
		return body, nil
	}
	missing := map[string]bool{}
	out := RefPattern.ReplaceAllStringFunc(body, func(match string) string {
		m := RefPattern.FindStringSubmatch(match)
		if form := forms[m[1]]; form != "" {
			return fmt.Sprintf(form, m[2])
		}
		missing[m[1]] = true
		return neutralRef(m[1], m[2])
	})
	if len(missing) == 0 {
		return out, nil
	}
	keywords := make([]string, 0, len(missing))
	for k := range missing {
		keywords = append(keywords, k)
	}
	sort.Strings(keywords)
	return out, keywords
}

func neutralRef(keyword, name string) string {
	if keyword == RefAgent {
		return "the " + name + " agent"
	}
	return "the " + name + " skill"
}
