package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// crushPreToolUseEvent is the only hook event Crush's own runtime
// consumes today (docs/hooks/README.md: "Crush currently supports
// just one hook, PreToolUse"; re-verified 2026-09-10). Any other key
// under crush.json's `hooks` map is a future-vendor reservation this
// importer does not reach.
const crushPreToolUseEvent = "PreToolUse"

// crushHookEntry mirrors one flat object in crush.json's
// `hooks.PreToolUse` array: `{"name": ..., "matcher": ..., "command":
// ..., "timeout": ...}` (schema.json `$defs.HookConfig`; `command` is
// the only required field). No Claude-style `{"matcher": ...,
// "hooks": [...]}` grouping to unwrap, unlike windsurf's own hook
// importer.
type crushHookEntry struct {
	Name    string `json:"name"`
	Matcher string `json:"matcher"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

// importCrushHooks reads crush.json's `hooks.PreToolUse` array and
// writes one yaml spec per entry into dstDir. No-op when the file, the
// `hooks` key, or the `PreToolUse` key is absent.
//
// Crush's own `name` (the friendly TUI label) becomes both the spec's
// `name:` field and, when present, the spec filename: a hook the user
// named survives a re-import at the same path instead of drifting to a
// new hash-derived one. An unnamed entry falls back to the same
// deterministic `hookSpecName` every other hook importer uses.
func importCrushHooks(root, dstDir string) (int, error) {
	src := filepath.Join(root, crushMCPFile)
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	var doc struct {
		Hooks map[string][]crushHookEntry `json:"hooks"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return 0, fmt.Errorf("parse %s: %w", src, err)
	}

	count := 0
	for _, h := range doc.Hooks[crushPreToolUseEvent] {
		if h.Command == "" {
			continue
		}
		name := strings.TrimSpace(h.Name)
		fileName := name
		if fileName == "" {
			fileName = hookSpecName(crushPreToolUseEvent, h.Matcher, []string{h.Command})
		}
		docMap := map[string]any{
			"event":   crushPreToolUseEvent,
			"command": h.Command,
			"target":  "crush",
		}
		if name != "" {
			docMap["name"] = name
		}
		if h.Matcher != "" {
			docMap["matcher"] = h.Matcher
		}
		if h.Timeout != 0 {
			docMap["timeout"] = h.Timeout
		}
		if err := writeCrushHookSpec(dstDir, fileName, docMap); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func writeCrushHookSpec(dstDir, name string, docMap map[string]any) error {
	raw, err := yaml.Marshal(docMap)
	if err != nil {
		return fmt.Errorf("marshal hook %s: %w", name, err)
	}
	path := filepath.Join(dstDir, name+".yaml")
	if err := importWriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
