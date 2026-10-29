package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/claudehooks"
	"github.com/chemaclass/agnostic-ai/internal/preview"
)

// importClaudeHooks reads .claude/settings.json and writes one yaml per
// matcher group into dstDir. Filenames come from claudeHookNamer so the
// same hook always lands at the same path across re-imports.
//
// A matcher block with multiple inner commands renders as a single yaml
// whose `command:` field is a list. The emit side merges the same
// event+matcher specs back into one matcher block, so the round-trip
// preserves multi-command groups without exploding into N files.
func importClaudeHooks(root, dstDir string) (int, error) {
	src := filepath.Join(root, claudeDir, "settings.json")
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	var s claudehooks.Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return 0, fmt.Errorf("parse %s: %w", src, err)
	}
	if len(s.Hooks) == 0 {
		return 0, nil
	}
	events := make([]string, 0, len(s.Hooks))
	for e := range s.Hooks {
		events = append(events, e)
	}
	sort.Strings(events)

	pin := newClaudeHookPin(root)
	namer := newClaudeHookNamer(dstDir)
	count := 0
	for _, event := range events {
		for _, g := range s.Hooks[event] {
			// Command handlers that differ in any setting become separate
			// specs, so one handler's settings never spread to its siblings.
			// A group sync does not plan whole stays in settings.json as
			// written, so leaving out one handler would make sync add the
			// others a second time.
			if field := claudeGroupCredentialField(g); field != "" {
				keptf("warning: claude: left out the %s hooks matching %q in %s: a hook's %s holds a credential; sync leaves them in that file as written but does not manage them; to manage them, add them under .agnostic-ai/local/hooks\n", event, g.Matcher, filepath.Join(claudeDir, "settings.json"), field)
				continue
			}
			var groups []*claudeCommandGroup
			for _, h := range g.Hooks {
				h.Command = adapters.StripCursorGuard(h.Command)
				if h.Type != "" && h.Type != "command" {
					n, err := importClaudeNonCommandHook(root, dstDir, event, g.Matcher, h, pin, namer)
					if err != nil {
						return count, err
					}
					count += n
					continue
				}
				if h.Command == "" || claudehooks.IsWorktreeSetupCommand(h.Command) || importLocal.dropsHookCommand("claude", event, g.Matcher, adapters.ExecFormCommand(h.Command, h.Args)) {
					continue
				}
				groups = addClaudeCommand(groups, h)
			}
			for _, cg := range groups {
				if err := writeClaudeCommandGroup(root, dstDir, event, g.Matcher, cg, pin, namer); err != nil {
					return count, err
				}
				count++
			}
		}
	}
	return count, nil
}

// claudeWebhookPaths are chat webhook URL paths, where the path is the
// secret. A match anywhere in a command or URL counts, since a false match
// only leaves a hook out.
var claudeWebhookPaths = []string{"hooks.slack.com/services/", "hooks.slack.com/workflows/", "hooks.slack.com/triggers/", "discord.com/api/webhooks/", "discordapp.com/api/webhooks/"}

func claudeWebhookURL(text string) bool {
	text = strings.ToLower(text)
	return slices.ContainsFunc(claudeWebhookPaths, func(path string) bool { return strings.Contains(text, path) })
}

// claudeGroupCredentialField names the first field of a hook in g that
// holds a literal credential, by the checks the settings overlay and MCP
// import use, or returns "". A hook spec is committed, and its name and
// description come from the command (#2668).
func claudeGroupCredentialField(g claudehooks.Group) string {
	for _, h := range g.Hooks {
		if field := claudeHookCredentialField(h); field != "" {
			return field
		}
	}
	return ""
}

func claudeHookCredentialField(h claudehooks.CommandEntry) string {
	command := adapters.ExecFormCommand(adapters.StripCursorGuard(h.Command), h.Args)
	switch {
	case h.Command != "" && (preview.CommandCredentialDetected(command) || claudeWebhookURL(command)):
		return "command"
	case h.URL != "" && (preview.CredentialDetected(h.URL) || claudeWebhookURL(h.URL)):
		return "url"
	case h.If != "" && preview.CredentialInRule(h.If):
		return "if"
	case h.StatusMessage != "" && preview.CredentialDetected(h.StatusMessage):
		return "statusMessage"
	case h.Prompt != "" && preview.CredentialDetected(h.Prompt):
		return "prompt"
	}
	for _, key := range slices.Sorted(maps.Keys(h.Headers)) {
		if credentialSettingValue(key, h.Headers[key]) {
			return "headers." + key
		}
	}
	if key := claudeHookInputCredential(h.Input, ""); key != "" {
		return "input." + key
	}
	return ""
}

// claudeHookInputCredential returns the dotted key of the first string in
// an `mcp_tool` input that holds a credential.
func claudeHookInputCredential(input map[string]any, prefix string) string {
	for _, key := range slices.Sorted(maps.Keys(input)) {
		path := prefix + key
		switch value := input[key].(type) {
		case string:
			if credentialSettingValue(key, value) || claudeWebhookURL(value) {
				return path
			}
		case map[string]any:
			if found := claudeHookInputCredential(value, path+"."); found != "" {
				return found
			}
		}
	}
	return ""
}

type claudeCommandSettings struct {
	args          string
	timeout       int
	statusMessage string
	shell         string
	ifRule        string
	async         bool
	asyncRewake   bool
	once          bool
	onFailure     string
}

type claudeCommandGroup struct {
	settings claudeCommandSettings
	args     []string
	cmds     []string
}

func addClaudeCommand(groups []*claudeCommandGroup, h claudehooks.CommandEntry) []*claudeCommandGroup {
	settings := claudeCommandSettings{
		args:          strings.Join(h.Args, "\x00"),
		timeout:       h.Timeout,
		statusMessage: h.StatusMessage,
		shell:         h.Shell,
		ifRule:        h.If,
		async:         h.Async,
		asyncRewake:   h.AsyncRewake,
		once:          h.Once,
		onFailure:     h.OnFailure,
	}
	for _, g := range groups {
		if g.settings == settings {
			g.cmds = append(g.cmds, h.Command)
			return groups
		}
	}
	return append(groups, &claudeCommandGroup{settings: settings, args: h.Args, cmds: []string{h.Command}})
}

func writeClaudeCommandGroup(root, dstDir, event, matcher string, g *claudeCommandGroup, pin claudeHookPin, namer *claudeHookNamer) error {
	cmds, set := g.cmds, g.settings
	name := namer.name(event, matcher, hookRunLabel(cmds[0]), cmds, map[string]any{"command": cmds})
	doc := map[string]any{
		"name":        name,
		"description": hookDescription(event, matcher, cmds),
		"event":       event,
		"matcher":     matcher,
	}
	if len(cmds) == 1 {
		doc["command"] = cmds[0]
	} else {
		doc["command"] = cmds
	}
	if len(g.args) > 0 {
		doc["args"] = g.args
	}
	if set.timeout != 0 {
		doc["timeout"] = set.timeout
	}
	if set.statusMessage != "" {
		doc["statusMessage"] = set.statusMessage
	}
	if set.async {
		doc["async"] = true
	}
	if set.asyncRewake {
		doc["asyncRewake"] = true
	}
	if set.once {
		doc["once"] = true
	}
	if set.shell != "" {
		doc["shell"] = set.shell
	}
	if set.ifRule != "" {
		doc["if"] = set.ifRule
	}
	setClaudeOnFailure(doc, set.onFailure, true)
	pin.apply(doc, root, filepath.Join(dstDir, name+".yaml"))
	return writeHookSpecFile(dstDir, name, doc)
}

// Claude Code documents onFailure for command and HTTP hooks only
// (2.1.295), so only there does "block" become the portable failClosed.
// Any other value or handler keeps the native key as written.
func setClaudeOnFailure(doc map[string]any, value string, documented bool) {
	switch {
	case value == "":
	case value == "block" && documented:
		doc["failClosed"] = true
	default:
		doc["x-claude"] = map[string]any{"onFailure": value}
	}
}

// Non-command handlers need separate specs because each has a distinct
// payload. Command groups retain their existing command-list format.
func importClaudeNonCommandHook(root, dstDir, event, matcher string, h claudehooks.CommandEntry, pin claudeHookPin, namer *claudeHookNamer) (int, error) {
	var target string
	switch h.Type {
	case "http":
		if h.URL == "" {
			return 0, nil
		}
		target = h.URL
	case "mcp_tool":
		if h.Server == "" || h.Tool == "" {
			return 0, nil
		}
		target = h.Server + "/" + h.Tool
	case "prompt":
		if h.Prompt == "" {
			return 0, nil
		}
	default:
		return 0, nil
	}
	payload, err := json.Marshal(h)
	if err != nil {
		return 0, fmt.Errorf("marshal %s hook: %w", event, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(payload, &doc); err != nil {
		return 0, fmt.Errorf("parse %s hook: %w", event, err)
	}
	name := namer.name(event, matcher, hookHandlerLabel(h.Type, target), []string{string(payload)},
		map[string]any{"type": h.Type, "url": h.URL, "server": h.Server, "tool": h.Tool, "prompt": h.Prompt})
	delete(doc, "onFailure")
	setClaudeOnFailure(doc, h.OnFailure, h.Type == "http")
	doc["name"], doc["event"], doc["matcher"] = name, event, matcher
	doc["description"] = hookHandlerDescription(h.Type, target, event, matcher)
	pin.apply(doc, root, filepath.Join(dstDir, name+".yaml"))
	if err := writeHookSpecFile(dstDir, name, doc); err != nil {
		return 0, err
	}
	return 1, nil
}
