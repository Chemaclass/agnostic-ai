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
func buildFields(section string, items []SectionItem, entries []spec.Entry, cfg *config.Config, global bool) []SettingField {
	defs := settingsFields[section]
	if defs == nil {
		return nil
	}
	byName := map[string]spec.Entry{}
	order := map[string]int{}
	for i, e := range entries {
		byName[e.Name] = e
		order[e.Name] = i
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
			edit := &FieldEdit{Kind: "text"}
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
			effective := -1
			for _, source := range f.Set {
				if i := order[source.Spec]; i > effective {
					effective = i
					edit.Path = source.Path
				}
			}
			if effective >= 0 && entries[effective].Layer == "builtin" {
				edit.Path = ""
			}
			for _, it := range items {
				if it.Kind != string(spec.KindSettings) || it.Layer == "builtin" {
					continue
				}
				if grantsAuthority(byName[it.Name]) && !slices.ContainsFunc(f.Set, func(source FieldSource) bool { return source.Spec == it.Name }) {
					continue
				}
				edit.Paths = append(edit.Paths, it.Path)
			}
			if edit.Path == "" {
				if len(edit.Paths) == 0 {
					if global {
						edit.Path = "settings/defaults.yaml"
					} else {
						edit.Path = filepath.ToSlash(filepath.Join(cfg.Sources.Settings, "defaults.yaml"))
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
