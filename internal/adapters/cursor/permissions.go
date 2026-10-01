package cursor

import (
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	permissionsUntranslatedReason = "allow rule(s) with no faithful Cursor CLI spelling stay out of .cursor/cli.json; Shell matches a command by its first word only, so a multi-word or exact rule would widen"
	permissionsAskReason          = "Cursor CLI permissions have allow and deny lists but no ask list; the CLI already prompts before a call no allow rule covers"
)

// cliPermissions translates the portable allow and deny lists of every
// settings spec into Cursor CLI rules, in source order without
// duplicates. It also counts the specs that carried an untranslated
// allow rule and the specs that carried an ask rule, for one note each,
// and lists each untranslated deny rule, since dropping one loosens the
// policy.
func cliPermissions(settings []spec.Entry) (rules map[string][]string, droppedAllow, asks int, droppedDeny []string) {
	rules = map[string][]string{}
	for _, entry := range settings {
		permissions, _ := entry.Meta["permissions"].(map[string]any)
		lostAllow := false
		for _, list := range []string{"allow", "deny"} {
			for _, rule := range emit.StringSlice(permissions[list]) {
				native := cliPermissionRules(rule, list)
				if len(native) == 0 {
					if list == "deny" {
						if !slices.Contains(droppedDeny, rule) {
							droppedDeny = append(droppedDeny, rule)
						}
					} else {
						lostAllow = true
					}
					continue
				}
				for _, r := range native {
					if !slices.Contains(rules[list], r) {
						rules[list] = append(rules[list], r)
					}
				}
			}
		}
		if lostAllow {
			droppedAllow++
		}
		if len(emit.StringSlice(permissions["ask"])) > 0 {
			asks++
		}
	}
	return rules, droppedAllow, asks, droppedDeny
}

// cliPermissionRules translates one portable rule onto the Cursor CLI
// vocabulary, returning nothing for a rule with no faithful spelling
// rather than guessing one (cursor.com/docs/cli/reference/permissions):
//
//	Bash(cmd:*), Bash(cmd *) -> Shell(cmd), for one word: "The commandBase is the first token"
//	Read(path)               -> Read(path)
//	Edit(path), Write(path)  -> Write(path)
//	WebFetch(domain:host)    -> WebFetch(host), for an exact host, *.host, or *
//	mcp__server__tool        -> Mcp(server:tool), with mcp__server and mcp__* covering every tool
//
// A rule is never widened, not even on deny, where a wider rule would
// block commands the author still runs. Shell takes only a first word,
// and the command:args form the docs show only as `curl:*` does not
// say how its glob reads the rest of the line, so a multi-word command
// stays out. So does an exact one-word command: Shell(cmd) also matches
// cmd with arguments.
func cliPermissionRules(rule, list string) []string {
	if strings.HasPrefix(rule, spec.MCPToolPrefix) {
		return mcpRule(rule, list)
	}
	scope, arg, ok := spec.SplitPermissionRule(rule)
	if !ok {
		return nil
	}
	switch scope {
	case "Bash":
		return shellRule(arg)
	case "Read":
		return pathRules("Read", arg, list)
	case "Edit", "Write":
		return pathRules("Write", arg, list)
	case "WebFetch":
		return webFetchRule(arg)
	}
	return nil
}

func shellRule(arg string) []string {
	command, prefix := strings.CutSuffix(arg, ":*")
	if !prefix {
		command, prefix = strings.CutSuffix(arg, " *")
	}
	if !prefix || !isCommandWord(command) {
		return nil
	}
	return []string{"Shell(" + command + ")"}
}

// isCommandWord reports a single command name with no glob, which is
// what Shell(commandBase) matches as the first token.
func isCommandWord(s string) bool {
	return s != "" && !strings.ContainsAny(s, " \t*?[]{}()'\"\\|&;<>$`")
}

// pathRules spells a portable path rule for Cursor. A portable path
// covers a directory's files, which a Cursor glob does not: the docs
// write `Write(src/**)` for "any file under src". So a deny on a path
// with no glob also gets `<path>/**`, as protected paths do. An allow
// stays as written, so it never approves more than it names.
func pathRules(tool, arg, list string) []string {
	path, ok := cliPath(arg)
	if !ok {
		return nil
	}
	rules := []string{tool + "(" + path + ")"}
	if list == "deny" && !strings.ContainsAny(path, "*?") {
		rules = append(rules, tool+"("+path+"/**)")
	}
	return rules
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
// only". They read as Claude Code's same three forms do. A host is a
// bare name, so one with a path or port stays out.
func webFetchRule(arg string) []string {
	host, ok := strings.CutPrefix(arg, "domain:")
	if !ok || host == "" || strings.ContainsAny(host, "/:") {
		return nil
	}
	if host != "*" {
		name := strings.TrimPrefix(host, "*.")
		if name == "" || strings.Contains(name, "*") {
			return nil
		}
	}
	return []string{"WebFetch(" + host + ")"}
}

// mcpRule maps an MCP rule onto Mcp(server:tool), where the docs use
// `*` for wildcards and show `Mcp(*:search)` for "any server's search
// tool". Claude Code takes a server glob on deny only, so `mcp__*` and
// `mcp__*__<tool>` translate there and nowhere else. Another server
// glob could match a different set of servers, so it stays out.
func mcpRule(rule, list string) []string {
	rest := strings.TrimPrefix(rule, spec.MCPToolPrefix)
	if rest == "*" {
		if list != "deny" {
			return nil
		}
		return []string{"Mcp(*:*)"}
	}
	server, tool, found := strings.Cut(rest, "__")
	if !found {
		tool = "*"
	}
	if server == "" || tool == "" {
		return nil
	}
	if server == "*" {
		if list != "deny" || strings.ContainsAny(tool, "*?") {
			return nil
		}
	} else if strings.ContainsAny(server, "*?") {
		return nil
	}
	return []string{"Mcp(" + server + ":" + tool + ")"}
}
