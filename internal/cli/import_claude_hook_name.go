package cli

import (
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

// hookDescriptionCommandMax caps how much of a command a hook
// description quotes.
const hookDescriptionCommandMax = 60

// claudeHookNamer names the hook specs one `import claude` run writes:
// `<event>[-<matcher>]-<what it runs>`, so the file says what it does.
// Two hooks that share that name keep it apart with hookSpecName's hash,
// in import order. A spec an older release wrote under the hash name
// keeps it, so a re-import updates that file instead of adding a copy.
type claudeHookNamer struct {
	dstDir string
	used   map[string]bool
}

func newClaudeHookNamer(dstDir string) *claudeHookNamer {
	return &claudeHookNamer{dstDir: dstDir, used: map[string]bool{}}
}

// name returns the spec name for a hook. label says what the hook runs,
// as hookRunLabel or a handler's own label reads it; seed is what
// hookSpecName hashes.
func (n *claudeHookNamer) name(event, matcher, label string, seed []string) string {
	hashed := hookSpecName(event, matcher, seed)
	if fileExists(filepath.Join(n.dstDir, hashed+".yaml")) {
		n.used[hashed] = true
		return hashed
	}
	parts := []string{strings.ToLower(strings.TrimSpace(event))}
	if parts[0] == "" {
		parts[0] = "hook"
	}
	if slug := hookMatcherSlug(matcher); slug != "" {
		parts = append(parts, slug)
	}
	label = hookMatcherSlug(label)
	if label == "" {
		n.used[hashed] = true
		return hashed
	}
	name := strings.Join(append(parts, label), "-")
	if n.used[name] {
		name += "-" + hookContentHash(event, matcher, seed)
	}
	n.used[name] = true
	return name
}

// hookRunLabel picks the words that say what a shell command runs: the
// base name of the first script path, or else its first two words that
// are not flags or variable assignments. Only the first command of a
// pipeline or list counts.
func hookRunLabel(command string) string {
	var words []string
	for _, field := range strings.Fields(command) {
		if field == "|" || field == "&&" || field == "||" || field == ";" {
			break
		}
		field = strings.Trim(field, `"'`)
		switch {
		case field == "" || strings.HasPrefix(field, "-"):
			continue
		case len(words) == 0 && strings.Contains(field, "=") && !strings.Contains(field, "/"):
			continue
		case strings.Contains(field, "/"):
			base := path.Base(field)
			return strings.TrimSuffix(base, path.Ext(base))
		}
		words = append(words, field)
		if len(words) == 2 {
			break
		}
	}
	return strings.Join(words, " ")
}

// hookHandlerLabel is hookRunLabel for a hook that is not a command.
func hookHandlerLabel(kind, target string) string {
	switch kind {
	case "http":
		if u, err := url.Parse(target); err == nil && u.Host != "" {
			return u.Hostname()
		}
		return "http"
	case "mcp_tool":
		return target
	}
	return kind
}

// hookDescription says what a hook runs and when, from its event,
// matcher, and commands, so an imported spec explains itself.
func hookDescription(event, matcher string, commands []string) string {
	if len(commands) == 0 {
		return ""
	}
	what := "Runs `" + shortHookCommand(commands[0]) + "`"
	if len(commands) > 1 {
		what += fmt.Sprintf(" and %d more", len(commands)-1)
	}
	return what + hookWhen(event, matcher)
}

// hookHandlerDescription is hookDescription for a hook that is not a
// command: an http call, an MCP tool, or a prompt.
func hookHandlerDescription(kind, target, event, matcher string) string {
	var what string
	switch kind {
	case "http":
		what = "Sends the event to " + target
	case "mcp_tool":
		what = "Calls MCP tool `" + target + "`"
	default:
		what = "Asks the model to check the event"
	}
	return what + hookWhen(event, matcher)
}

func hookWhen(event, matcher string) string {
	when := " on " + event
	if matcher != "" && matcher != "*" {
		when += " for " + matcher
	}
	return when + "."
}

func shortHookCommand(command string) string {
	command = strings.ReplaceAll(strings.Join(strings.Fields(command), " "), "`", "'")
	if r := []rune(command); len(r) > hookDescriptionCommandMax {
		return string(r[:hookDescriptionCommandMax]) + "..."
	}
	return command
}
