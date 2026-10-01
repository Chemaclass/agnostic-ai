package cursor

import (
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	permissionsUntranslatedReason = "rule(s) with no faithful Cursor CLI spelling stay out of .cursor/cli.json; Shell matches a command by its first word only, so a multi-word or exact allow rule would widen"
	permissionsAskReason          = "Cursor CLI permissions have allow and deny lists but no ask list; the CLI already prompts before a call no allow rule covers"
)

// cliPermissions translates the portable allow and deny lists of every
// settings spec into Cursor CLI rules, in source order without
// duplicates. It also counts the specs that carried an untranslated
// rule and the specs that carried an ask rule, for one note each.
func cliPermissions(settings []spec.Entry) (rules map[string][]string, dropped, asks int) {
	rules = map[string][]string{}
	for _, entry := range settings {
		permissions, _ := entry.Meta["permissions"].(map[string]any)
		lost := false
		for _, list := range []string{"allow", "deny"} {
			for _, rule := range emit.StringSlice(permissions[list]) {
				native, ok := cliPermissionRule(rule, list)
				if !ok {
					lost = true
					continue
				}
				if !slices.Contains(rules[list], native) {
					rules[list] = append(rules[list], native)
				}
			}
		}
		if lost {
			dropped++
		}
		if len(emit.StringSlice(permissions["ask"])) > 0 {
			asks++
		}
	}
	return rules, dropped, asks
}

// cliPermissionRule translates one portable rule onto the Cursor CLI
// vocabulary, reporting false for anything with no faithful spelling
// rather than guessing one (cursor.com/docs/cli/reference/permissions):
//
//	Bash(cmd:*), Bash(cmd *) -> Shell(cmd), for one word: "The commandBase is the first token"
//	Bash(cmd)                -> Shell(cmd) on deny only, since Shell also matches cmd with arguments
//	Read(path)               -> Read(path)
//	Edit(path), Write(path)  -> Write(path)
//	WebFetch(domain:host)    -> WebFetch(host), for an exact host, *.host, or *
//	mcp__server__tool        -> Mcp(server:tool), with mcp__server and mcp__* covering every tool
//
// A multi-word command has no faithful form. Shell takes only a first
// word, and the command:args form the docs show only as `curl:*` does
// not say how its glob reads the rest of the line. Widening it to the
// first word would block or allow every subcommand, so the rule stays
// out on both lists.
func cliPermissionRule(rule, list string) (string, bool) {
	if strings.HasPrefix(rule, spec.MCPToolPrefix) {
		return mcpRule(rule, list)
	}
	scope, arg, ok := spec.SplitPermissionRule(rule)
	if !ok {
		return "", false
	}
	switch scope {
	case "Bash":
		return shellRule(arg, list)
	case "Read":
		if path, ok := cliPath(arg); ok {
			return "Read(" + path + ")", true
		}
	case "Edit", "Write":
		if path, ok := cliPath(arg); ok {
			return "Write(" + path + ")", true
		}
	case "WebFetch":
		return webFetchRule(arg)
	}
	return "", false
}

func shellRule(arg, list string) (string, bool) {
	command, prefix := strings.CutSuffix(arg, ":*")
	if !prefix {
		command, prefix = strings.CutSuffix(arg, " *")
	}
	if !isCommandWord(command) {
		return "", false
	}
	if !prefix && list != "deny" {
		return "", false
	}
	return "Shell(" + command + ")", true
}

// isCommandWord reports a single command name with no glob, which is
// what Shell(commandBase) matches as the first token.
func isCommandWord(s string) bool {
	return s != "" && !strings.ContainsAny(s, " \t*?[]{}()'\"\\|&;<>$`")
}

// cliPath maps a portable path onto Cursor's: "Relative paths are
// scoped to the current workspace" and "Absolute paths can target files
// outside the project". A portable `/path` anchors at the project and
// `//path` is absolute. The page lists `**`, `*`, and `?` and no other
// glob syntax, and says nothing of `~`, so those stay out.
func cliPath(arg string) (string, bool) {
	if strings.HasPrefix(arg, "~") || strings.HasPrefix(arg, "!") || strings.ContainsAny(arg, "[]{}\\") {
		return "", false
	}
	if rest, ok := strings.CutPrefix(arg, "//"); ok {
		if rest == "" {
			return "", false
		}
		return "/" + rest, true
	}
	path := strings.TrimPrefix(strings.TrimPrefix(arg, "./"), "/")
	if path == "" {
		return "", false
	}
	return path, true
}

// webFetchRule maps the portable `domain:` form onto the three host
// patterns Cursor documents: "`*` matches all domains", "`*.example.com`
// matches subdomains", and "`example.com` matches that exact domain
// only". They read as Claude Code's same three forms do.
func webFetchRule(arg string) (string, bool) {
	host, ok := strings.CutPrefix(arg, "domain:")
	if !ok || host == "" {
		return "", false
	}
	if host != "*" && strings.Contains(strings.TrimPrefix(host, "*."), "*") {
		return "", false
	}
	return "WebFetch(" + host + ")", true
}

// mcpRule maps an MCP rule onto Mcp(server:tool), where the docs use
// `*` for wildcards. A server name with a glob would match servers
// beyond the one named; only a deny rule for every server keeps that
// meaning, as `mcp__*` on deny does.
func mcpRule(rule, list string) (string, bool) {
	rest := strings.TrimPrefix(rule, spec.MCPToolPrefix)
	if rest == "*" {
		if list != "deny" {
			return "", false
		}
		return "Mcp(*:*)", true
	}
	server, tool, found := strings.Cut(rest, "__")
	if !found {
		tool = "*"
	}
	if server == "" || tool == "" || strings.ContainsAny(server, "*?") {
		return "", false
	}
	return "Mcp(" + server + ":" + tool + ")", true
}
