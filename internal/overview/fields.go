package overview

import (
	"maps"
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
			e := byName[it.Name]
			values := d.values(it.Values)
			if len(values) == 0 && e.Base != nil && !d.prefix && LocalSets(e, d.key) {
				// A personal null clears the team value; show it so it can be reset.
				values = []Value{{Key: d.key, Type: ValueText, Value: "null"}}
			}
			if len(values) == 0 {
				continue
			}
			if e.Base == nil || d.prefix {
				local := LocalLayer(e) && (e.Base == nil || slices.ContainsFunc(slices.Collect(maps.Keys(e.LocalMeta)), d.matches))
				f.Set = append(f.Set, FieldSource{Spec: it.Name, Path: it.Path, Values: values, Local: local})
				continue
			}
			team := SpecPath(root, *e.Base)
			if !LocalSets(e, d.key) {
				f.Set = append(f.Set, FieldSource{Spec: it.Name, Path: team, Values: values})
				continue
			}
			f.Set = append(f.Set, FieldSource{Spec: it.Name, Path: it.Path, Values: values, Local: true})
			if shared := d.values(settingsValues(*e.Base)); len(shared) > 0 {
				f.Set = append(f.Set, FieldSource{Spec: it.Name, Path: team, Values: shared})
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
					edit.DisabledReason = "Session retention is written only by sync --global. Open Settings in agnostic-ai ui --global."
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
					if edit.ToolAllowed == nil {
						edit.ToolAllowed = map[string][]string{}
					}
					edit.ToolAllowed[target] = slices.Sorted(slices.Values(accepter.SettingsEffortLevels()))
					if edit.ToolAllowed[target] == nil {
						edit.ToolAllowed[target] = []string{}
					}
					for _, level := range accepter.SettingsEffortLevels() {
						if !slices.Contains(edit.Allowed, level) {
							edit.Allowed = append(edit.Allowed, level)
						}
					}
				}
				slices.Sort(edit.Allowed)
			}
			paths := map[string]string{}
			for _, it := range items {
				if e, ok := byName[it.Name]; ok && it.Kind == string(spec.KindSettings) && SettingsEditCandidate(e, d.key) {
					paths[it.Name] = SettingsTeamPath(root, e)
					edit.Paths = append(edit.Paths, paths[it.Name])
				}
			}
			defaultPath := "settings/defaults.yaml"
			if !global {
				defaultPath = displayPath(root, filepath.Join(config.ResolveSourcePath(root, cfg.Sources.Settings), "defaults.yaml"))
			}
			inEffect := SettingsInEffect(entries, d.key, cfg.Targets)
			if target := SettingsWriteTarget(entries, inEffect, d.key, cfg.Targets); target >= 0 {
				edit.Path = paths[entries[target].Name]
			} else if inEffect >= 0 || len(edit.Paths) == 0 {
				if taken, ok := byName["defaults"]; ok {
					edit.DisabledReason = SpecPath(root, taken) + " is already the settings spec named defaults; add the field to an editable settings spec."
					if inEffect >= 0 {
						edit.DisabledReason = SpecPath(root, entries[inEffect]) + " sets this field and no editable spec loads after it."
					}
				} else {
					edit.Path = defaultPath
					if len(edit.Paths) > 0 {
						edit.Paths = append(edit.Paths, defaultPath)
					}
				}
			} else if len(edit.Paths) == 1 {
				edit.Path = edit.Paths[0]
			}
			if at := slices.IndexFunc(entries, func(e spec.Entry) bool { return edit.Path != "" && SettingsTeamPath(root, e) == edit.Path }); at >= 0 {
				edit.LocalPath = SettingsOverlayPath(global, entries[at].Name)
				if LocalLayer(entries[at]) {
					edit.LocalPath = SpecPath(root, entries[at])
				}
			} else if edit.Path == defaultPath {
				edit.LocalPath = SettingsOverlayPath(global, "defaults")
			}
			for _, source := range f.Set {
				for _, value := range source.Values {
					if !d.editableValue(value) {
						edit.DisabledReason = "This field has a non-scalar value; edit the spec directly."
					}
				}
			}
			if d.key == "model" && slices.ContainsFunc(entries, func(e spec.Entry) bool {
				return e.ModelTier != "" && slices.ContainsFunc(f.Set, func(s FieldSource) bool { return s.Spec == e.Name })
			}) {
				edit.DisabledReason = "This model names a tier from models: in the config; edit the spec directly."
			}
			f.Editable = edit
		}
		fields = append(fields, f)
	}
	return fields
}

// editableValue reports whether the page can edit v in place: the
// field's own scalar, or one tool's scalar in its per-tool map.
func (d fieldDef) editableValue(v Value) bool {
	if d.key == "session-retention-days" {
		return v.Key == d.key
	}
	entry, nested := strings.CutPrefix(v.Key, d.key+".")
	return v.Type == ValueText && (v.Key == d.key || nested && entry != "" && !strings.Contains(entry, "."))
}

func (d fieldDef) values(all []Value) []Value {
	var out []Value
	for _, v := range all {
		if d.matches(v.Key) {
			out = append(out, v)
		}
	}
	return out
}

// LocalLayer reports whether e comes from a personal, git-ignored layer.
func LocalLayer(e spec.Entry) bool {
	return e.Layer == "project-user" || e.Layer == "global-local"
}

// LocalSets reports whether a personal file itself sets the dotted key
// for e, not the shared spec it extends.
func LocalSets(e spec.Entry, key string) bool {
	if !LocalLayer(e) {
		return false
	}
	var current any = e.LocalMeta
	if e.Base == nil {
		current = e.Meta
	}
	for _, part := range strings.Split(key, ".") {
		m, ok := current.(map[string]any)
		if !ok {
			return false
		}
		if current, ok = m[part]; !ok {
			return false
		}
	}
	return true
}

// SettingsTeamPath is the shown path of the committed file behind e: its
// own, or the shared spec a local overlay extends.
func SettingsTeamPath(root string, e spec.Entry) string {
	if e.Base != nil {
		return SpecPath(root, *e.Base)
	}
	return SpecPath(root, e)
}

// SettingsValuePath is the shown path of the file that gives key its
// value in e for targets: the personal file when it changes the value
// for one of them, else the shared spec beneath it.
func SettingsValuePath(root string, e spec.Entry, key string, targets []string, tiers map[string]config.ModelTier) string {
	if e.Base == nil || LocalSets(e, key) && slices.ContainsFunc(targets, func(t string) bool {
		return settingsValueFor(e, key, t) != baseValue(e, key, t, tiers)
	}) {
		return SpecPath(root, e)
	}
	return SettingsTeamPath(root, e)
}

// SettingsOverlayPath is the personal file, relative to the project or
// global source root, that overlays the settings spec named name.
func SettingsOverlayPath(global bool, name string) string {
	dir := config.SourceBaseDir + "/local/settings"
	if global {
		dir = "local/settings"
	}
	return dir + "/" + name + ".yaml"
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

// SettingsInEffect is the index in the ordered settings of the spec whose
// key sync uses: the last one that sets it for every target and no later
// spec overrides, else the last one that sets it. -1 when none does.
func SettingsInEffect(settings []spec.Entry, key string, targets []string) int {
	last := -1
	for i := len(settings) - 1; i >= 0; i-- {
		if !hasField(settings[i], key) {
			continue
		}
		if last < 0 {
			last = i
		}
		if !slices.ContainsFunc(targets, func(t string) bool { return !setsFor(settings[i], key, t) }) && SettingsOverride(settings, i, key, targets) < 0 {
			return i
		}
	}
	return last
}

// SettingsWriteTarget is the edit candidate an edit of key writes so it
// takes effect: the spec in effect when it is a candidate, else the last
// candidate after it that applies to every target it does. -1 when none.
func SettingsWriteTarget(settings []spec.Entry, inEffect int, key string, targets []string) int {
	if inEffect < 0 {
		return -1
	}
	if SettingsEditCandidate(settings[inEffect], key) {
		return inEffect
	}
	applies := appliesTo(settings, inEffect, targets)
	for j := len(settings) - 1; j > inEffect; j-- {
		if SettingsEditCandidate(settings[j], key) && coversTargets(settings[j], applies) {
			return j
		}
	}
	return -1
}

// SettingsOverride is the index of a spec after i that gives key a value
// for every target settings[i] applies to, or -1. With i < 0 the edited
// spec applies to every target and every spec counts as later.
func SettingsOverride(settings []spec.Entry, i int, key string, targets []string) int {
	applies := appliesTo(settings, i, targets)
	for j := len(settings) - 1; j > i; j-- {
		if hasField(settings[j], key) && !slices.ContainsFunc(applies, func(t string) bool { return !overridesFor(settings[j], key, t) }) {
			return j
		}
	}
	return -1
}

// overridesFor is setsFor for a later spec, without the Claude model
// name target cannot load, as the resolver drops it.
func overridesFor(e spec.Entry, key, target string) bool {
	if key == "model" && foreignClaudeModel(e, target) {
		e = withoutSharedModel(e)
	}
	return setsFor(e, key, target)
}

// setsFor reports whether e gives target a value for key: a per-tool map
// without target or a default gives it nothing.
func setsFor(e spec.Entry, key, target string) bool {
	if !hasField(e, key) || !e.EmitsTo(target) {
		return false
	}
	return !slices.Contains(resolvedFields, key) || settingsValueFor(e, key, target) != ""
}

func appliesTo(settings []spec.Entry, i int, targets []string) []string {
	var applies []string
	for _, t := range targets {
		if i < 0 || settings[i].EmitsTo(t) {
			applies = append(applies, t)
		}
	}
	return applies
}

func coversTargets(e spec.Entry, targets []string) bool {
	return len(targets) == 0 || !slices.ContainsFunc(targets, func(t string) bool { return !e.EmitsTo(t) })
}
