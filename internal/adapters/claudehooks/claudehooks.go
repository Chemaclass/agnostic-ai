// Package claudehooks defines the `.claude/settings.json` hooks wire
// schema shared by the Claude emitter and the `import claude` reader.
//
// The emitter (internal/adapters/claude) marshals these structs into the
// `hooks` block; the importer (internal/cli) unmarshals the same block
// back into specs. Keeping one definition means a new hook field is added
// once and survives the round-trip instead of being silently dropped by
// whichever side was not updated.
package claudehooks

import (
	"encoding/json"
	"slices"
	"strings"
)

// CommandEntry mirrors one native handler object inside a matcher
// group's `hooks` array. A struct (not a map) makes `encoding/json` emit
// the fields in declaration order rather than the alpha-sorted order map
// iteration would produce.
//
// The optional fields carry `omitempty` so specs that don't set them
// produce the historic minimal `{type, command}` payload. `async`,
// `asyncRewake`, `shell`, and `if` are command-hook schema fields;
// dropping them on emit would strip behavior a user authored in
// settings.json (import captures them for the round-trip).
//
// Args switches a command hook from shell form to exec form: "When
// present, `command` is resolved as an executable and spawned directly
// with `args` as the argument vector, with no shell involved"
// (code.claude.com/docs/en/hooks, verified 2026-09-11). Claude Code is
// the only emitter here that sets it; Codex's hook docs list no such
// field, so an `args` on a codex-bound spec would be an invented key
// (#732).
//
// CommandWindows and AdditionalContextLimit are Codex-only
// (learn.chatgpt.com/docs/hooks); they live here anyway per this package's
// own round-trip rule, and stay absent from claude's own emit because
// internal/adapters/claude never sets it on the struct it builds.
//
// Server, Tool, and Input are shared by Claude and Codex MCP-tool hooks.
// URL, Headers, AllowedEnvVars, Prompt, and Model serve Claude and Qoder
// HTTP and prompt handlers. Other renderers leave these fields unset.
type CommandEntry struct {
	Type                   string            `json:"type"`
	Command                string            `json:"command,omitempty"`
	Args                   []string          `json:"args,omitempty"`
	Timeout                int               `json:"timeout,omitempty"`
	StatusMessage          string            `json:"statusMessage,omitempty"`
	Async                  bool              `json:"async,omitempty"`
	AsyncRewake            bool              `json:"asyncRewake,omitempty"`
	Shell                  string            `json:"shell,omitempty"`
	If                     string            `json:"if,omitempty"`
	Once                   bool              `json:"once,omitempty"`
	CommandWindows         string            `json:"commandWindows,omitempty"`
	AdditionalContextLimit *int              `json:"additionalContextLimit,omitempty"`
	Server                 string            `json:"server,omitempty"`
	Tool                   string            `json:"tool,omitempty"`
	Input                  map[string]any    `json:"input,omitempty"`
	URL                    string            `json:"url,omitempty"`
	Headers                map[string]string `json:"headers,omitempty"`
	AllowedEnvVars         []string          `json:"allowedEnvVars,omitempty"`
	Prompt                 string            `json:"prompt,omitempty"`
	Model                  string            `json:"model,omitempty"`
	ContinueOnBlock        bool              `json:"continueOnBlock,omitempty"`
	OnFailure              string            `json:"onFailure,omitempty"`
}

// Group mirrors one `{matcher, hooks}` object in a settings.json hook
// event array. Same rationale as CommandEntry: ordered struct fields beat
// sorted map keys.
type Group struct {
	Matcher string         `json:"matcher"`
	Hooks   []CommandEntry `json:"hooks"`
}

// Settings is the `.claude/settings.json` subset the importer decodes: the
// top-level `hooks` map keyed by event name.
type Settings struct {
	Hooks map[string][]Group `json:"hooks"`
}

// Order is the sidecar `import claude` writes beside the settings
// overlay, which leaves `hooks` out: the key `hooks` sat next to and
// the order of its events.
type Order struct {
	After  string   `json:"after,omitempty"`
	Before string   `json:"before,omitempty"`
	Events []string `json:"events,omitempty"`
}

// OrderOf records the key before `hooks` among keys, or the key after
// it when `hooks` comes first, with the event order.
func OrderOf(keys, events []string) Order {
	o := Order{Events: events}
	switch i := slices.Index(keys, "hooks"); {
	case i > 0:
		o.After = keys[i-1]
	case i == 0 && len(keys) > 1:
		o.Before = keys[1]
	}
	return o
}

// Index returns where a new `hooks` key goes among keys: beside the
// recorded key, or last when that key is gone.
func (o Order) Index(keys []string) int {
	if i := slices.Index(keys, o.After); o.After != "" && i >= 0 {
		return i + 1
	}
	if i := slices.Index(keys, o.Before); o.Before != "" && i >= 0 {
		return i
	}
	return len(keys)
}

// UnmarshalJSON also reads the bare event list earlier releases wrote.
func (o *Order) UnmarshalJSON(data []byte) error {
	var events []string
	if json.Unmarshal(data, &events) == nil {
		*o = Order{Events: events}
		return nil
	}
	type plain Order
	return json.Unmarshal(data, (*plain)(o))
}

// WorktreeSetupScript is the script sync writes under the Claude hooks
// directory to run an environment spec's `setup` in a new worktree.
// `import claude` skips it and the hooks that run it, since sync derives
// both from the environment spec.
const WorktreeSetupScript = "agnostic-ai-worktree-setup.sh"

// IsWorktreeSetupCommand reports whether a hook command runs the
// generated worktree setup script, in slash or Windows path form.
func IsWorktreeSetupCommand(command string) bool {
	return strings.Contains(strings.ReplaceAll(command, `\`, "/"), "/"+WorktreeSetupScript)
}
