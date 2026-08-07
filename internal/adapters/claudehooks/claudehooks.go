// Package claudehooks defines the `.claude/settings.json` hooks wire
// schema shared by the Claude emitter and the `import claude` reader.
//
// The emitter (internal/adapters/claude) marshals these structs into the
// `hooks` block; the importer (internal/cli) unmarshals the same block
// back into specs. Keeping one definition means a new hook field is added
// once and survives the round-trip instead of being silently dropped by
// whichever side was not updated.
package claudehooks

// CommandEntry mirrors one `{type, command, ...}` object inside a matcher
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
// CommandWindows and AdditionalContextLimit are Codex-only
// (learn.chatgpt.com/docs/hooks); they live here anyway per this package's
// own round-trip rule, and stay absent from claude's own emit because
// internal/adapters/claude never sets it on the struct it builds.
type CommandEntry struct {
	Type                   string `json:"type"`
	Command                string `json:"command"`
	Timeout                int    `json:"timeout,omitempty"`
	StatusMessage          string `json:"statusMessage,omitempty"`
	Async                  bool   `json:"async,omitempty"`
	AsyncRewake            bool   `json:"asyncRewake,omitempty"`
	Shell                  string `json:"shell,omitempty"`
	If                     string `json:"if,omitempty"`
	Once                   bool   `json:"once,omitempty"`
	CommandWindows         string `json:"commandWindows,omitempty"`
	AdditionalContextLimit *int   `json:"additionalContextLimit,omitempty"`
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
