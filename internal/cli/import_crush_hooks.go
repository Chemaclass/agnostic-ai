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
			h.Name = strings.TrimSpace(h.Name)
			name := h.Name
			fileName := hookSpecName(event, h.Matcher, []string{h.Command})
			safeName := spec.ValidateName(spec.KindHook, name) == nil
			if !matchingCrushHook(filepath.Join(dstDir, fileName+".yaml"), event, h) {
				nameAvailable := false
				if safeName {
					_, err := os.Stat(filepath.Join(dstDir, name+".yaml"))
					nameAvailable = errors.Is(err, fs.ErrNotExist)
				}
				if safeName && (names[name] == 1 && nameAvailable || matchingCrushHook(filepath.Join(dstDir, name+".yaml"), event, h)) {
					fileName = name
				} else if _, err := os.Stat(filepath.Join(dstDir, fileName+".yaml")); err == nil {
					identity, err := json.Marshal(h)
					if err != nil {
						return count, fmt.Errorf("encode crush hook identity: %w", err)
					}
					fileName = hookSpecName(event, h.Matcher, []string{string(identity)})
				}
			}
			path := filepath.Join(dstDir, fileName+".yaml")
			if _, err := os.Stat(path); err == nil {
				if !matchingCrushHook(path, event, h) {
					return count, fmt.Errorf("%s: native hook identity conflicts with an existing spec; rename the native hook", path)
				}
			} else if !errors.Is(err, fs.ErrNotExist) {
				return count, fmt.Errorf("%s: %w", path, err)
			}
			docMap := map[string]any{
				"event":   event,
				"command": h.Command,
				"target":  "crush",
			}
			if name == "" {
				docMap["x-crush"] = map[string]any{"name": ""}
			} else {
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
		if prior.Name == strings.TrimSpace(hook.Name) && prior.Timeout == hook.Timeout &&
			prior.Matcher == hook.Matcher && adapters.StripHookTargetExport(prior.Command, "crush") == hook.Command {
			return true
		}
	}
	return false
}
