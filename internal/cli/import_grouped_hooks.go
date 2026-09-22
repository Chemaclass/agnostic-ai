package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// groupedHookEntry is the inner hook object every claude-style grouped
// hook file carries: `{type, command, timeout}` inside a
// `{matcher, hooks: [...]}` group, itself inside a map keyed by event
// name. Claude's own settings.json carries a dozen more keys, which
// importClaudeHooks reads through claudehooks.CommandEntry; trae and
// goose document only these three, so reading more would invent fields
// their vendors never write.
type groupedHookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

// groupedHookGroup is one `{matcher, hooks: [...]}` object under an
// event key.
type groupedHookGroup struct {
	Matcher string             `json:"matcher"`
	Hooks   []groupedHookEntry `json:"hooks"`
}

// sortedHookEvents returns the event keys of a decoded hook map in a
// stable order, so re-importing the same file writes the same specs in
// the same order every time.
func sortedHookEvents[T any](byEvent map[string][]T) []string {
	events := make([]string, 0, len(byEvent))
	for event := range byEvent {
		events = append(events, event)
	}
	sort.Strings(events)
	return events
}

// writeGroupedHookSpec collapses one matcher group into a single hook
// spec and writes it into dstDir. Multiple commands sharing a matcher
// render as one yaml whose `command:` field is a list, the same
// collapsing importClaudeHooks applies, because the emit side merges
// same-event-same-matcher specs back into one group: exploding them
// into N files would not survive a second round-trip.
//
// extra carries the target's own group-level keys (trae's `loop_limit`,
// goose's `x-goose.on_failure`) onto the spec. Returns 1 when a spec was
// written and 0 when the group carried no command at all.
func writeGroupedHookSpec(dstDir, target, event, matcher string, entries []groupedHookEntry, extra map[string]any) (int, error) {
	var commands []string
	timeout := 0
	for _, h := range entries {
		if h.Type != "" && h.Type != "command" {
			continue
		}
		if h.Command == "" {
			continue
		}
		commands = append(commands, h.Command)
		if h.Timeout != 0 && timeout == 0 {
			timeout = h.Timeout
		}
	}
	if len(commands) == 0 {
		return 0, nil
	}
	name := hookSpecName(event, matcher, commands)
	doc := map[string]any{
		"name":    name,
		"event":   event,
		"matcher": matcher,
		"target":  target,
	}
	if len(commands) == 1 {
		doc["command"] = commands[0]
	} else {
		doc["command"] = commands
	}
	if timeout != 0 {
		doc["timeout"] = timeout
	}
	for k, v := range extra {
		doc[k] = v
	}
	if err := writeHookSpecFile(dstDir, name, doc); err != nil {
		return 0, err
	}
	return 1, nil
}

// writeHookSpecFile marshals one hook spec document into
// `<dstDir>/<name>.yaml`. Importers that build the document key by key
// (crush, kiro) call this directly instead of writeGroupedHookSpec,
// which owns the claude-style matcher-group collapse they have no
// groups to apply.
func writeHookSpecFile(dstDir, name string, doc map[string]any) error {
	raw, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal hook %s: %w", name, err)
	}
	path := filepath.Join(dstDir, name+".yaml")
	if err := importWriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// readEventKeyedHooks decodes a standalone hooks.json into its event
// map. Factory keys the file directly by event name, OpenHands accepts
// that and the Claude `{"hooks": {...}}` wrapper, so both shapes read
// here. A missing file returns nil.
func readEventKeyedHooks[T any](path string) (map[string][]T, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if wrapped, ok := top["hooks"]; ok && bytes.HasPrefix(bytes.TrimSpace(wrapped), []byte("{")) {
		data = wrapped
	}
	var byEvent map[string][]T
	if err := json.Unmarshal(data, &byEvent); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return byEvent, nil
}
