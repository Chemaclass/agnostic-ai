package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/preview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// userSyncPin binds a user sync to one plan: a run under it records the
// fingerprint of what it would write, and a run that wants a fingerprint
// writes nothing when the plan moved.
type userSyncPin struct {
	want, got string
	// commands are the hook and MCP server commands the run installs, so
	// a confirm can show what will run, not only which files change.
	commands []string
}

type userSyncPinKey struct{}

// errUserSyncMoved stops a pinned apply whose writes or removals differ
// from the plan the person confirmed.
var errUserSyncMoved = errors.New("the pending changes changed since you checked them")

func withUserSyncPin(ctx context.Context, pin *userSyncPin) context.Context {
	return context.WithValue(ctx, userSyncPinKey{}, pin)
}

// checkPin records run's fingerprint on the pin in ctx, if any, and
// refuses a run that differs from the one it wants.
func (r userRun) checkPin(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	pin, ok := ctx.Value(userSyncPinKey{}).(*userSyncPin)
	if !ok {
		return nil
	}
	pin.got = r.fingerprint()
	if pin.want != "" && pin.got != pin.want {
		return errUserSyncMoved
	}
	return nil
}

// fingerprint sums every planned write (path, mode and content) and
// every removal, so the same plan id means the same changes.
func (r userRun) fingerprint() string {
	lines := make([]string, 0, len(r.writes)+len(r.removals))
	for _, w := range r.writes {
		sum := sha256.Sum256(w.data)
		lines = append(lines, fmt.Sprintf("write\x00%s\x00%o\x00%x", w.path, w.mode, sum))
	}
	for _, path := range r.removals {
		lines = append(lines, "remove\x00"+path)
	}
	slices.Sort(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

// syncCommands lists, as "<kind> <name>: <command>", every command a
// user hook or MCP server spec makes a tool run, read from the records
// this plan writes for each target. A line only some targets run names
// them, unless every target does. agnostic-ai's own built-in hooks and
// servers are left out.
func (p *userPlan) syncCommands() []string {
	commands := []string{}
	add := func(label string, lines map[string][]string, order []string, all int) {
		for _, line := range order {
			on := lines[line]
			entry := label + ": " + line
			if len(on) != all {
				entry = label + " (" + strings.Join(on, ", ") + "): " + line
			}
			if !slices.Contains(commands, entry) {
				commands = append(commands, entry)
			}
		}
	}
	for _, hook := range p.bundle.Hooks {
		if hook.Layer == "builtin" {
			continue
		}
		lines, order := targetLines(p.targets, func(target string) []string {
			if records, shown := p.shown[target]; shown {
				return nativeLines(records[hook.Name])
			}
			if !hook.EmitsTo(target) {
				return nil
			}
			return userHookTargetLines(target, hook)
		})
		add("hook "+hook.Name, lines, order, len(p.targets))
	}
	for _, server := range p.bundle.MCPs {
		if server.Layer == "builtin" {
			continue
		}
		lines, order := targetLines(p.targets, func(target string) []string {
			owned := p.next.MCP[target]
			// A JSON file keys a server under the target's MCP key.
			record, ok := owned[server.Name]
			if !ok {
				record, ok = owned[userTargets[target].mcp.key+"."+server.Name]
			}
			if !ok {
				return nil
			}
			return nativeLines([]any{record})
		})
		add("MCP server "+server.Name, lines, order, len(p.targets))
	}
	return commands
}

// targetLines gathers the lines each target runs, in first-seen order,
// each with the targets that run it.
func targetLines(targets []string, linesFor func(target string) []string) (map[string][]string, []string) {
	lines := map[string][]string{}
	var order []string
	for _, target := range targets {
		for _, line := range linesFor(target) {
			if line == "" || slices.Contains(lines[line], target) {
				continue
			}
			if _, seen := lines[line]; !seen {
				order = append(order, line)
			}
			lines[line] = append(lines[line], target)
		}
	}
	return lines, order
}

// userHookTargetLines are a hook's command lines on a target whose
// written records the plan does not keep: each command with its args,
// after the target's override.
func userHookTargetLines(target string, hook spec.Entry) []string {
	resolved := adapters.TargetHook(target, hook).Meta
	args := stringSliceFromAny(resolved["args"])
	var lines []string
	for _, command := range userHookCommands(resolved["command"]) {
		lines = append(lines, strings.TrimSpace(adapters.ExecFormCommand(command, args)))
	}
	for _, command := range userHookCommands(resolved["commandWindows"]) {
		lines = append(lines, strings.TrimSpace(command))
	}
	return lines
}

// runKeys are the record fields that hold something a tool runs or
// sends to.
var runKeys = []string{"command", "commandWindows", "bash", "powershell", "exec", "url", "prompt"}

// nativeLines are the lines a tool runs from records as sync writes
// them: each command with its args, env and cwd, each url with what it
// sends, and each prompt, wherever they sit in the record.
func nativeLines(records []any) []string {
	var lines []string
	var walk func(node any)
	walk = func(node any) {
		switch value := node.(type) {
		case []any:
			for _, item := range value {
				walk(item)
			}
		case map[string]any:
			if !slices.ContainsFunc(runKeys, func(key string) bool { _, ok := value[key]; return ok }) {
				for _, key := range slices.Sorted(maps.Keys(value)) {
					walk(value[key])
				}
				return
			}
			lines = append(lines, recordLines(value)...)
		}
	}
	for _, record := range records {
		walk(jsonRoundTrip(record))
	}
	return lines
}

// recordLines renders one record that holds runKeys.
func recordLines(record map[string]any) []string {
	var lines []string
	args := stringSliceFromAny(record["args"])
	for _, key := range []string{"command", "exec"} {
		for _, command := range userHookCommands(record[key]) {
			lines = append(lines, withEnvAndCwd(adapters.ExecFormCommand(command, args), record))
		}
	}
	for _, key := range []string{"commandWindows", "bash", "powershell"} {
		if command, _ := record[key].(string); strings.TrimSpace(command) != "" {
			lines = append(lines, withEnvAndCwd(command, record))
		}
	}
	if url, _ := record["url"].(string); url != "" {
		lines = append(lines, preview.RedactURL(strings.TrimSpace(url))+httpHookSends(record))
	}
	if prompt, _ := record["prompt"].(string); strings.TrimSpace(prompt) != "" {
		lines = append(lines, oneLine(preview.RedactText(prompt)))
	}
	return lines
}

// httpHookSends names the env variables and headers a request sends
// along, without the header values.
func httpHookSends(meta map[string]any) string {
	var parts []string
	if vars := stringSliceFromAny(meta["allowedEnvVars"]); len(vars) > 0 {
		parts = append(parts, "env "+strings.Join(slices.Sorted(slices.Values(vars)), ", "))
	}
	if headers, ok := meta["headers"].(map[string]any); ok && len(headers) > 0 {
		parts = append(parts, "headers "+strings.Join(slices.Sorted(maps.Keys(headers)), ", "))
	}
	if len(parts) == 0 {
		return ""
	}
	return " (sends " + strings.Join(parts, "; ") + ")"
}

func oneLine(text string) string { return strings.Join(strings.Fields(text), " ") }

// plainShellWord matches a value that reads the same unquoted.
var plainShellWord = regexp.MustCompile(`^[A-Za-z0-9_./:=,@+%{}$-]+$`)

func shownWord(value string) string {
	if value == preview.Redacted || plainShellWord.MatchString(value) {
		return value
	}
	return adapters.ShellQuote(value)
}

// withEnvAndCwd puts meta's env before line and its cwd after, masking
// an env value that looks like a secret. A value that is not a plain
// word is quoted, so it cannot read as another command.
func withEnvAndCwd(line string, meta map[string]any) string {
	line = strings.TrimSpace(line)
	if env, ok := meta["env"].(map[string]any); ok && len(env) > 0 {
		pairs := make([]string, 0, len(env))
		for name, value := range env {
			pairs = append(pairs, name+"="+shownWord(shownEnvValue(name, fmt.Sprint(value))))
		}
		slices.Sort(pairs)
		line = strings.Join(pairs, " ") + " " + line
	}
	if cwd, _ := meta["cwd"].(string); cwd != "" {
		line += " (in " + shownWord(cwd) + ")"
	}
	return line
}

// shownEnvValue masks the whole value of a variable named as a
// credential, unless it only references other variables; any other
// value keeps what the shared redaction leaves of it.
func shownEnvValue(key, value string) string {
	if preview.CredentialKey(key) && !preview.RefOnly(value) {
		return preview.Redacted
	}
	return preview.RedactText(value)
}
