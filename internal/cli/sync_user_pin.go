package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
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

// userSyncCommands lists, as "<kind> <name>: <command>", every command a
// user hook or MCP server spec asks a tool to run. A hook's line is the
// command with its args, as each of targets reads it after its override;
// a line only some targets run names them. agnostic-ai's own built-in
// hooks are left out.
func userSyncCommands(bundle spec.Bundle, targets []string) []string {
	commands := []string{}
	add := func(label string, raw any) {
		for _, line := range commandLines(raw) {
			if line = strings.TrimSpace(line); line != "" && !slices.Contains(commands, label+line) {
				commands = append(commands, label+line)
			}
		}
	}
	for _, hook := range bundle.Hooks {
		if hook.Layer == "builtin" {
			continue
		}
		label := "hook " + hook.Name
		lines, reached := userHookLines(hook, targets)
		for _, line := range lines.order {
			on := lines.targets[line]
			if len(on) == len(reached) {
				add(label+": ", line)
			} else {
				add(label+" ("+strings.Join(on, ", ")+"): ", line)
			}
		}
	}
	for _, server := range bundle.MCPs {
		if server.Layer == "builtin" {
			continue
		}
		label := "MCP server " + server.Name + ": "
		add(label, mcpCommandLine(server.Meta))
		// Sorted, so the plan id stays the same for the same specs.
		for _, field := range slices.Sorted(maps.Keys(server.Meta)) {
			override, ok := server.Meta[field].(map[string]any)
			if !ok || !strings.HasPrefix(field, "x-") {
				continue
			}
			// A tool's block replaces the top-level keys it names, one by one.
			merged := maps.Clone(server.Meta)
			maps.Copy(merged, override)
			add(label, mcpCommandLine(merged))
		}
	}
	return commands
}

// hookLines are a hook's command lines in first-seen order, each with
// the targets that run it.
type hookLines struct {
	order   []string
	targets map[string][]string
}

// userHookLines renders hook's command lines for each target it reaches,
// and returns those targets.
func userHookLines(hook spec.Entry, targets []string) (hookLines, []string) {
	lines := hookLines{targets: map[string][]string{}}
	var reached []string
	for _, target := range targets {
		if !hook.EmitsTo(target) {
			continue
		}
		reached = append(reached, target)
		for _, line := range userHookTargetLines(target, hook) {
			if line == "" || slices.Contains(lines.targets[line], target) {
				continue
			}
			if _, seen := lines.targets[line]; !seen {
				lines.order = append(lines.order, line)
			}
			lines.targets[line] = append(lines.targets[line], target)
		}
	}
	return lines, reached
}

// userHookTargetLines are the lines hook runs on target: each command
// with its args. Copilot also writes a command's cwd and env, an http
// hook's url and a prompt hook's text, so its lines carry them.
func userHookTargetLines(target string, hook spec.Entry) []string {
	resolved := adapters.TargetHook(target, hook).Meta
	if target == "copilot" {
		resolved = adapters.ResolveMeta(hook.Meta, target)
		switch kind, _ := resolved["type"].(string); kind {
		case "http":
			url, _ := resolved["url"].(string)
			return []string{preview.RedactURL(strings.TrimSpace(url))}
		case "prompt":
			prompt, _ := resolved["prompt"].(string)
			return []string{strings.TrimSpace(prompt)}
		}
	}
	args := stringSliceFromAny(resolved["args"])
	override, _ := hook.Meta["x-"+target].(map[string]any)
	var lines []string
	// A Windows command under the override is listed too, so nothing it may run is hidden.
	for _, raw := range []any{resolved["command"], resolved["commandWindows"], override["commandWindows"]} {
		for _, command := range userHookCommands(raw) {
			line := strings.TrimSpace(adapters.ExecFormCommand(command, args))
			if line != "" && target == "copilot" {
				line = withEnvAndCwd(line, resolved)
			}
			lines = append(lines, line)
		}
	}
	return lines
}

// withEnvAndCwd puts meta's env before line and its cwd after, masking
// an env value that looks like a secret.
func withEnvAndCwd(line string, meta map[string]any) string {
	if env, ok := meta["env"].(map[string]any); ok && len(env) > 0 {
		pairs := make([]string, 0, len(env))
		for name, value := range env {
			text := fmt.Sprint(value)
			if secretEnvValue(name, text) {
				text = preview.Redacted
			}
			pairs = append(pairs, name+"="+text)
		}
		slices.Sort(pairs)
		line = strings.Join(pairs, " ") + " " + line
	}
	if cwd, _ := meta["cwd"].(string); cwd != "" {
		line += " (in " + cwd + ")"
	}
	return line
}

// secretEnvValue reports whether an env value named key reads as a
// secret, by the test import uses for MCP env values.
func secretEnvValue(key, value string) bool {
	named := preview.CredentialKey(key) && !preview.RefOnly(value) && preview.SecretValue(preview.DetectorText(value), false, preview.WeakCredential)
	return named || preview.CredentialDetected(value)
}

// mcpCommandLine is a server's command with its args, cwd and env, or
// its url; empty when meta names neither.
func mcpCommandLine(meta map[string]any) string {
	line, _ := meta["command"].(string)
	if line == "" {
		url, _ := meta["url"].(string)
		return preview.RedactURL(url)
	}
	line = strings.Join(append([]string{line}, commandLines(meta["args"])...), " ")
	return withEnvAndCwd(line, meta)
}

func commandLines(raw any) []string {
	switch value := raw.(type) {
	case string:
		return []string{value}
	case []string:
		return value
	case []any:
		lines := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok {
				lines = append(lines, text)
			}
		}
		return lines
	}
	return nil
}
