package goose

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const defaultHooksFile = ".agents/plugins/agnostic-ai/hooks/hooks.json"

var hookLifecycle = []string{
	"SessionStart", "SessionEnd", "Stop", "UserPromptSubmit",
	"PreToolUse", "PreToolUseResult", "PostToolUse", "PostToolUseFailure",
	"BeforeReadFile", "AfterFileEdit", "BeforeShellExecution", "AfterShellExecution",
}

type hookAction struct {
	Type      string `json:"type"`
	Command   string `json:"command"`
	Timeout   int    `json:"timeout,omitempty"`
	OnFailure string `json:"on_failure,omitempty"`
}

type hookGroup struct {
	Matcher string       `json:"matcher,omitempty"`
	Hooks   []hookAction `json:"hooks"`
}

type hooksDoc struct {
	order  []string
	events map[string][]hookGroup
}

func (d hooksDoc) MarshalJSON() ([]byte, error) {
	var body strings.Builder
	body.WriteString(`{"hooks":{`)
	for i, event := range d.order {
		if i > 0 {
			body.WriteByte(',')
		}
		key, err := json.Marshal(event)
		if err != nil {
			return nil, err
		}
		groups, err := json.Marshal(d.events[event])
		if err != nil {
			return nil, err
		}
		body.Write(key)
		body.WriteByte(':')
		body.Write(groups)
	}
	body.WriteString("}}")
	return []byte(body.String()), nil
}

// emitHooks writes the plugin's `hooks/hooks.json` and returns the
// plugin root the manifest belongs at, or "" when no hook spec
// contributes. The caller writes the manifest, so one plugin carrying
// both skills and hooks gets a single `plugin.json` (#862).
func emitHooks(sess *emit.Session, hooks []spec.Entry, cfg *config.Config, dryRun bool) (string, error) {
	doc := buildHooks(hooks)
	if doc == nil {
		return "", nil
	}
	hooksBody, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	hooksPath := emit.OutputHooksFile(cfg, target, defaultHooksFile)
	if filepath.Base(hooksPath) != "hooks.json" || filepath.Base(filepath.Dir(hooksPath)) != "hooks" {
		return "", fmt.Errorf("goose: hooks file %s must end in hooks/hooks.json so Goose can discover the plugin", hooksPath)
	}
	if err := sess.WriteFile(hooksPath, string(hooksBody)+"\n", dryRun); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Dir(filepath.Dir(hooksPath))), nil
}

func buildHooks(hooks []spec.Entry) *hooksDoc {
	type groupKey struct{ event, matcher string }
	byKey := map[groupKey][]hookAction{}
	var order []groupKey
	var invalidOnFailure, misplacedOnFailure int
	for _, hook := range hooks {
		event, _ := hook.Meta["event"].(string)
		commands := emit.HookCommands(hook.Meta["command"])
		if event == "" || len(commands) == 0 {
			continue
		}
		matcher, _ := hook.Meta["matcher"].(string)
		native, _ := emit.CustomTargetMeta(hook.Meta, target)
		onFailure, _ := native["on_failure"].(string)
		if onFailure != "" && onFailure != "allow" && onFailure != "block" {
			invalidOnFailure++
			onFailure = ""
		}
		if onFailure != "" && event != "PreToolUse" {
			misplacedOnFailure++
			onFailure = ""
		}
		key := groupKey{event: event, matcher: matcher}
		if _, exists := byKey[key]; !exists {
			order = append(order, key)
		}
		for _, command := range commands {
			byKey[key] = append(byKey[key], hookAction{
				Type: "command", Command: command,
				Timeout: emit.HookIntMeta(hook.Meta, "timeout"), OnFailure: onFailure,
			})
		}
	}
	emit.NoteFieldNoOp(target, spec.KindHook, "x-goose.on_failure", invalidOnFailure,
		"Goose accepts only allow or block")
	emit.NoteFieldNoOp(target, spec.KindHook, "x-goose.on_failure", misplacedOnFailure,
		"Goose reads the failure policy only on PreToolUse")
	if len(order) == 0 {
		return nil
	}
	doc := &hooksDoc{events: map[string][]hookGroup{}}
	for _, key := range order {
		if _, exists := doc.events[key.event]; !exists {
			doc.order = append(doc.order, key.event)
		}
		doc.events[key.event] = append(doc.events[key.event], hookGroup{Matcher: key.matcher, Hooks: byKey[key]})
	}
	for event, groups := range doc.events {
		sort.SliceStable(groups, func(i, j int) bool { return groups[i].Matcher < groups[j].Matcher })
		doc.events[event] = groups
	}
	doc.order = orderGooseEvents(doc.order)
	return doc
}

func orderGooseEvents(seen []string) []string {
	present := map[string]bool{}
	for _, event := range seen {
		present[event] = true
	}
	out := make([]string, 0, len(seen))
	for _, event := range hookLifecycle {
		if present[event] {
			out = append(out, event)
			delete(present, event)
		}
	}
	for _, event := range seen {
		if present[event] {
			out = append(out, event)
			delete(present, event)
		}
	}
	return out
}
