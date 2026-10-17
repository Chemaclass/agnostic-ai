package authority

import (
	"regexp"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Capability kinds a permission rule or an agent's tool-list entry can
// stand for.
const (
	capRead      = "read"
	capEdit      = "edit"
	capWrite     = "write"
	capDelete    = "delete"
	capShell     = "shell"
	capWeb       = "web"       // fetching and searching
	capWebFetch  = "webfetch"  // the WebFetch tool alone, or one domain
	capWebSearch = "websearch" // the WebSearch tool alone
	capMCP       = "mcp"
	capOther     = "other" // a tool alias the view does not place
)

// capability is one parsed rule or tool-list entry.
type capability struct {
	kind string
	// path scopes read and edit; empty covers every file.
	path string
	// shell scopes a shell rule; nil covers every command.
	shell *shellPattern
	// server and tool name an MCP server and, optionally, one tool.
	server string
	tool   string
	// domain scopes WebFetch to one domain.
	domain string
}

// shellPattern is a scoped shell rule: a command prefix, or one exact
// command.
type shellPattern struct {
	// text is the pattern as written inside shell(...).
	text string
	// prefix is the command prefix with `:*`, ` *`, or `*` removed, or the
	// whole command when exact.
	prefix string
	exact  bool
}

// parseCapability reads one rule or tool-list entry. An entry that does
// not parse stands for nothing: lint and sync report it.
func parseCapability(raw string) []capability {
	names, problem := spec.CapabilityTools([]string{raw})
	if problem != "" {
		return nil
	}
	if raw == capWeb {
		return []capability{{kind: capWeb}}
	}
	out := make([]capability, 0, len(names))
	for _, name := range names {
		out = append(out, parseToolName(name))
	}
	return out
}

func parseToolName(name string) capability {
	if neutral, ok := spec.NeutralPermission(name); ok {
		return parseNeutral(neutral)
	}
	switch name {
	case "WebFetch":
		return capability{kind: capWebFetch}
	case "WebSearch":
		return capability{kind: capWebSearch}
	}
	if scope, arg, ok := spec.SplitPermissionRule(name); ok && scope == "WebFetch" {
		if domain, ok := strings.CutPrefix(arg, "domain:"); ok && domain != "" {
			return capability{kind: capWebFetch, domain: domain}
		}
	}
	return capability{kind: capOther}
}

func parseNeutral(c string) capability {
	if rest, ok := strings.CutPrefix(c, "mcp:"); ok {
		server, tool, _ := strings.Cut(rest, "/")
		return capability{kind: capMCP, server: server, tool: tool}
	}
	name, arg, scoped := strings.Cut(c, "(")
	if !scoped {
		return capability{kind: name}
	}
	arg = strings.TrimSuffix(arg, ")")
	switch name {
	case capShell:
		p := parseShellPattern(arg)
		return capability{kind: capShell, shell: &p}
	case capRead, capEdit:
		return capability{kind: name, path: normalizePattern(arg)}
	}
	return capability{kind: capOther}
}

// parseShellPattern normalizes `go test:*`, `go test *`, and `go test*`
// to the prefix "go test". A pattern without a trailing wildcard is one
// exact command.
func parseShellPattern(text string) shellPattern {
	p := strings.TrimSpace(text)
	if rest, ok := strings.CutSuffix(p, ":*"); ok && !strings.Contains(rest, "*") {
		return shellPattern{text: text, prefix: strings.TrimSpace(rest)}
	}
	if rest, ok := strings.CutSuffix(p, "*"); ok && !strings.Contains(rest, "*") {
		return shellPattern{text: text, prefix: strings.TrimSpace(rest)}
	}
	return shellPattern{text: text, prefix: p, exact: true}
}

// normalizePattern drops a leading `./` or a single leading `/`, which
// anchors a rule at the project root.
func normalizePattern(p string) string {
	p = strings.TrimPrefix(strings.TrimSpace(p), "./")
	if strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "//") {
		p = strings.TrimPrefix(p, "/")
	}
	return p
}

// covers reports whether c applies to action on r, and a note when it
// applies only through a tool's documented reading, such as an edit
// rule that Claude Code also checks writes against. forAgent reads c as
// an entry of an agent's tool list, where no such reading applies.
func (c capability) covers(r *resourceInfo, action string, forAgent bool) (bool, string) {
	switch r.kind {
	case kindFiles:
		return c.coversFile(r.pattern, action, forAgent)
	case kindShell:
		return c.kind == capShell && (c.shell == nil || shellCovers(*c.shell, r.shell)), ""
	case kindWeb:
		return c.coversWeb(r.domain, action), ""
	case kindMCP:
		if c.kind != capMCP || c.server != r.mcp.name {
			return false, ""
		}
		if action == anyToolAction {
			return c.tool == "", ""
		}
		return c.tool == "" || c.tool == action, ""
	}
	return false, ""
}

func (c capability) coversFile(pattern, action string, forAgent bool) (bool, string) {
	switch action {
	case capRead, capEdit:
		return c.kind == action && (c.path == "" || fileCovers(c.path, pattern)), ""
	case capWrite:
		if c.kind == capWrite {
			return true, ""
		}
		if !forAgent && c.kind == capEdit && c.path != "" && fileCovers(c.path, pattern) {
			return true, "Claude Code checks writes against edit rules"
		}
	case capDelete:
		return c.kind == capDelete, ""
	}
	return false, ""
}

func (c capability) coversWeb(domain, action string) bool {
	switch action {
	case "fetch":
		if c.kind == capWeb {
			return true
		}
		return c.kind == capWebFetch && (c.domain == "" || c.domain == domain)
	case "search":
		return c.kind == capWeb || c.kind == capWebSearch
	}
	return false
}

// shellCovers reports whether rule covers the shell resource res, where
// nil stands for any command. A prefix covers every command and prefix
// that starts with it; an exact command covers only itself.
func shellCovers(rule shellPattern, res *shellPattern) bool {
	if res == nil {
		return !rule.exact && rule.prefix == ""
	}
	if rule.exact {
		return res.exact && res.prefix == rule.prefix
	}
	return strings.HasPrefix(res.prefix, rule.prefix)
}

// fileCovers approximates whether rule covers every file pattern names:
// the same pattern, a rule ending in `**` whose literal prefix starts the
// pattern, or a literal path the rule matches directly or through a
// parent directory. Patterns are compared as text, so the view records
// the containment as approximate.
func fileCovers(rule, pattern string) bool {
	if rule == pattern {
		return true
	}
	if strings.HasSuffix(rule, "**") && strings.HasPrefix(pattern, literalPrefix(rule)) {
		return true
	}
	if !hasWildcard(pattern) {
		if re, err := regexp.Compile(spec.ProtectPatternRegexp(rule)); err == nil {
			return matchesWithParents(re, pattern)
		}
	}
	return false
}

// literalPrefix is the part of a pattern before its first wildcard.
func literalPrefix(p string) string {
	if i := strings.IndexAny(p, "*?"); i >= 0 {
		return p[:i]
	}
	return p
}

func hasWildcard(p string) bool {
	return strings.ContainsAny(p, "*?")
}

// matchesWithParents reports whether re matches file or one of its
// parent directories, as a protected path covers every file under it.
func matchesWithParents(re *regexp.Regexp, file string) bool {
	for p := file; ; {
		if re.MatchString(p) {
			return true
		}
		i := strings.LastIndex(p, "/")
		if i < 0 {
			return false
		}
		p = p[:i]
	}
}
