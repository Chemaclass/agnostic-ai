package overview

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// fieldDef is one settings field a section can show.
type fieldDef struct {
	key, label, description string
	// prefix matches every key that starts with it, such as x- for the
	// native keys of each tool.
	prefix bool
}

// settingsFields are the fields of a settings spec, split by the
// section that shows them, from docs/site/content/docs/spec-format/settings.md.
var settingsFields = map[string][]fieldDef{
	SectionAuthority: {
		{key: "permissions.allow", label: "Allowed without asking", description: "Rules the agent runs without a prompt."},
		{key: "permissions.ask", label: "Ask first", description: "Rules that prompt before running."},
		{key: "permissions.deny", label: "Always blocked", description: "Rules the agent may never run."},
		{key: "permissions.default-mode", label: "Default permission mode", description: "The permission mode a session starts in. Only Claude Code applies it today, so it takes Claude Code's mode names: default, manual, acceptEdits, plan, auto, dontAsk, or bypassPermissions. Other tools get a sync note."},
		{key: "protected", label: "Protected paths", description: "Files agents must not edit without asking: paths, a decision (ask or deny), and a reason."},
	},
	SectionSettings: {
		{key: "model", label: "Default model", description: "The model each tool starts with: one value, a map per tool, or a model tier name."},
		{key: "effort", label: "Reasoning effort", description: "The default reasoning effort: one value, or a map per tool."},
		{key: "session-retention-days", label: "Session retention", description: "Days to keep session transcripts, at least 1. Only sync --global writes it."},
		{key: "x-", label: "Native keys", description: "Keys written as they are into one tool's own settings file, under x-claude, x-codex, and so on.", prefix: true},
	},
}

// buildFields lists every field of section with the values each settings
// spec in items gives it.
func buildFields(section string, items []SectionItem, entries []spec.Entry, cfg *config.Config, root string, global bool) []SettingField {
	defs := settingsFields[section]
	if defs == nil {
		return nil
	}
	byName := map[string]spec.Entry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	fields := make([]SettingField, 0, len(defs))
	for _, d := range defs {
		f := SettingField{Key: d.key, Label: d.label, Description: d.description, Set: []FieldSource{}}
		for _, it := range items {
			if it.Kind != string(spec.KindSettings) {
				continue
			}
			if _, ok := byName[it.Name]; !ok {
				continue
			}
			var values []Value
			for _, v := range it.Values {
				if d.matches(v.Key) {
					values = append(values, v)
				}
			}
			if len(values) > 0 {
				f.Set = append(f.Set, FieldSource{Spec: it.Name, Path: it.Path, Values: values})
			}
		}
		if section == SectionSettings && !d.prefix && cfg != nil {
			edit := &FieldEdit{Kind: "text", SyncCommand: "agnostic-ai sync"}
			if global {
				edit.SyncCommand = "agnostic-ai sync --global"
			}
			if d.key == "session-retention-days" {
				edit.Kind = "number"
				if !global {
					edit.DisabledReason = "Session retention is written only by sync --global. Open Settings in agnostic-ai serve --global."
				}
			}
			if d.key == "effort" {
				for _, target := range cfg.Targets {
					adapter, err := adapters.Resolve(target)
					if err != nil {
						continue
					}
					accepter, ok := adapter.(adapters.SettingsEffortAccepter)
					if !ok {
						continue
					}
					for _, level := range accepter.SettingsEffortLevels() {
						if !slices.Contains(edit.Allowed, level) {
							edit.Allowed = append(edit.Allowed, level)
						}
					}
				}
				slices.Sort(edit.Allowed)
			}
			var candidates []spec.Entry
			paths := map[string]string{}
			for _, it := range items {
				if e, ok := byName[it.Name]; ok && it.Kind == string(spec.KindSettings) && SettingsEditCandidate(e, d.key) {
					paths[it.Name] = it.Path
					edit.Paths = append(edit.Paths, it.Path)
				}
			}
			for _, e := range entries {
				if _, ok := paths[e.Name]; ok {
					candidates = append(candidates, e)
				}
			}
			if i := SettingsInEffect(candidates, d.key, cfg.Targets); i >= 0 {
				edit.Path = paths[candidates[i].Name]
			}
			if edit.Path == "" {
				if len(edit.Paths) == 0 {
					if global {
						edit.Path = "settings/defaults.yaml"
					} else {
						edit.Path = displayPath(root, filepath.Join(config.ResolveSourcePath(root, cfg.Sources.Settings), "defaults.yaml"))
					}
				} else if len(edit.Paths) == 1 {
					edit.Path = edit.Paths[0]
				}
			}
			// Maps and other non-scalar shapes remain read-only in this slice.
			for _, source := range f.Set {
				for _, value := range source.Values {
					if value.Key != d.key || value.Type != ValueText && d.key != "session-retention-days" {
						edit.DisabledReason = "This field has a non-scalar value; edit the spec directly."
					}
				}
			}
			f.Editable = edit
		}
		fields = append(fields, f)
	}
	return fields
}

func (d fieldDef) matches(key string) bool {
	if d.prefix {
		return strings.HasPrefix(key, d.key)
	}
	return key == d.key || strings.HasPrefix(key, d.key+".")
}

// SettingsEditCandidate reports whether a field edit may write e. Built-in
// and pack specs are replaced on update, and a spec under Authority takes
// the edit only when it already sets key.
func SettingsEditCandidate(e spec.Entry, key string) bool {
	if e.Layer == builtinLayer || strings.HasPrefix(e.Layer, "pack:") {
		return false
	}
	return !grantsAuthority(e) || hasField(e, key)
}

// SettingsInEffect is the index of the candidate whose key an edit should
// change: the last one that sets it for every target and no later
// candidate overrides, else the last one that sets it. -1 when none does.
func SettingsInEffect(candidates []spec.Entry, key string, targets []string) int {
	last := -1
	for i := len(candidates) - 1; i >= 0; i-- {
		if !hasField(candidates[i], key) {
			continue
		}
		if last < 0 {
			last = i
		}
		if !slices.ContainsFunc(targets, func(t string) bool { return !candidates[i].EmitsTo(t) }) && SettingsOverride(candidates, i, key, targets) < 0 {
			return i
		}
	}
	return last
}

// SettingsOverride is the index of a candidate after i that sets key for
// every target candidates[i] applies to, or -1. With i < 0 the edited spec
// applies to every target and every candidate counts as later.
func SettingsOverride(candidates []spec.Entry, i int, key string, targets []string) int {
	var applies []string
	for _, t := range targets {
		if i < 0 || candidates[i].EmitsTo(t) {
			applies = append(applies, t)
		}
	}
	for j := len(candidates) - 1; j > i; j-- {
		later := candidates[j]
		if hasField(later, key) && (len(applies) == 0 || !slices.ContainsFunc(applies, func(t string) bool { return !later.EmitsTo(t) })) {
			return j
		}
	}
	return -1
}
