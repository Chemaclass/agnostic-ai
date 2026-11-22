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
// user hook or MCP server spec asks a tool to run. agnostic-ai's own
// built-in hooks are left out.
func userSyncCommands(bundle spec.Bundle) []string {
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
		label := "hook " + hook.Name + ": "
		for _, key := range []string{"command", "commandWindows"} {
			add(label, hook.Meta[key])
		}
		for field, raw := range hook.Meta {
			if override, ok := raw.(map[string]any); ok && strings.HasPrefix(field, "x-") {
				for _, key := range []string{"command", "commandWindows"} {
					add(label, override[key])
				}
			}
		}
	}
	for _, server := range bundle.MCPs {
		if server.Layer == "builtin" {
			continue
		}
		label := "MCP server " + server.Name + ": "
		add(label, mcpCommandLine(server.Meta))
		for field, raw := range server.Meta {
			override, ok := raw.(map[string]any)
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

// mcpCommandLine is a server's command with its args, cwd and env, or
// its url; empty when meta names neither.
func mcpCommandLine(meta map[string]any) string {
	line, _ := meta["command"].(string)
	if line == "" {
		url, _ := meta["url"].(string)
		return url
	}
	line = strings.Join(append([]string{line}, commandLines(meta["args"])...), " ")
	if env, ok := meta["env"].(map[string]any); ok && len(env) > 0 {
		pairs := make([]string, 0, len(env))
		for name, value := range env {
			pairs = append(pairs, fmt.Sprintf("%s=%v", name, value))
		}
		slices.Sort(pairs)
		line = strings.Join(pairs, " ") + " " + line
	}
	if cwd, _ := meta["cwd"].(string); cwd != "" {
		line += " (in " + cwd + ")"
	}
	return line
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
