package emit

import (
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
