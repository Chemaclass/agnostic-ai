package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/crush"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

type crushHookEntry struct {
	Name    string `json:"name"`
	Matcher string `json:"matcher"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

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

	names := map[string]int{}
	for rawEvent, hooks := range doc.Hooks {
		event := crush.CanonicalHookEvent(rawEvent)
		if event == "" {
			continue
		}
		for _, h := range hooks {
			if h.Matcher != "" && !crush.HookAcceptsMatcher(event) {
				return 0, fmt.Errorf("parse %s: %s does not accept a matcher; remove matcher or use a tool event", src, event)
			}
			if name := strings.TrimSpace(h.Name); name != "" && h.Command != "" {
				names[name]++
			}
		}
	}
	count := 0
	for _, rawEvent := range sortedHookEvents(doc.Hooks) {
		event := crush.CanonicalHookEvent(rawEvent)
		if event == "" {
			continue
		}
		for _, h := range doc.Hooks[rawEvent] {
			h.Command = adapters.StripHookTargetExport(h.Command, "crush")
			if h.Command == "" {
				continue
			}
			name := strings.TrimSpace(h.Name)
			fileName := hookSpecName(event, h.Matcher, []string{h.Command})
			safeName := spec.ValidateName(spec.KindHook, name) == nil
			if !matchingCrushHook(filepath.Join(dstDir, fileName+".yaml"), event, h) && safeName &&
				(names[name] == 1 || matchingCrushHook(filepath.Join(dstDir, name+".yaml"), event, h)) {
				fileName = name
			}
			docMap := map[string]any{
				"event":   event,
				"command": h.Command,
				"target":  "crush",
			}
			if name != "" {
				docMap["name"] = name
				if fileName != name && (safeName || names[name] > 1) {
					docMap["name"] = fileName
					docMap["x-crush"] = map[string]any{"name": name}
				}
			}
			if h.Matcher != "" {
				docMap["matcher"] = h.Matcher
			}
			if h.Timeout != 0 {
				docMap["timeout"] = h.Timeout
			}
			if err := writeHookSpecFile(dstDir, fileName, docMap); err != nil {
				return count, err
			}
			count++
		}
	}
	return count, nil
}

func matchingCrushHook(path, event string, hook crushHookEntry) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	entry, err := spec.ParseYAMLBytes(spec.KindHook, raw)
	if err != nil || !entry.EmitsTo("crush") {
		return false
	}
	native, reason := entry.NativeHook("crush")
	if reason != "" {
		return false
	}
	defer adapters.SetAsideNotes()()
	body, err := crush.HookDoc(adapters.TargetHook("crush", native))
	if err != nil {
		return false
	}
	var doc struct {
		Hooks map[string][]crushHookEntry `json:"hooks"`
	}
	if json.Unmarshal(body, &doc) != nil {
		return false
	}
	for _, prior := range doc.Hooks[event] {
		if prior.Matcher == hook.Matcher && adapters.StripHookTargetExport(prior.Command, "crush") == hook.Command {
			return true
		}
	}
	return false
}
