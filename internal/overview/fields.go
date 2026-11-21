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
		{key: PermissionModeKey, label: "Default permission mode", description: "The permission mode a session starts in. Only Claude Code applies it today, so it takes Claude Code's mode names: default, manual, acceptEdits, plan, auto, dontAsk, or bypassPermissions. Other tools get a sync note."},
		{key: ProtectedKey, label: "Protected paths", description: "Files agents must not edit without asking: paths, a decision (ask or deny), and a reason."},
	},
	SectionSettings: {
		{key: "model", label: "Default model", description: "The model each tool starts with: one value, a map per tool, or a model tier name."},
		{key: "effort", label: "Reasoning effort", description: "The default reasoning effort: one value, or a map per tool."},
		{key: "session-retention-days", label: "Session retention", description: "Days to keep session transcripts, at least 1. Only sync --scope user writes it."},
		{key: "voice.enabled", label: "Voice dictation", description: "Turn voice dictation on or off. Only sync --scope user writes it."},
		{key: "voice.mode", label: "Voice mode", description: "Hold to record, or tap to start and stop recording."},
		{key: "voice.language", label: "Voice language", description: "The spoken language passed to each tool that supports it."},
		{key: "x-", label: "Native keys", description: "Keys written as they are into one tool's own settings file, under x-claude, x-codex, and so on.", prefix: true},
	},
}

// voiceUserOnlyReason explains why a project cannot edit voice fields.
const voiceUserOnlyReason = "Voice is written only by sync --scope user. Open Settings in agnostic-ai ui --scope user."

// buildFields lists every field of section with the values each settings
// spec in items gives it.
func buildFields(section string, items []SectionItem, entries []spec.Entry, cfg *config.Config, root string, userLevel bool) []SettingField {
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
		f := SettingField{Key: d.key, Label: d.label, Description: d.description, Set: fieldSources(d, items, byName, root)}
		if section == SectionAuthority {
			f.Editable = authorityFieldEdit(d, entries, cfg, root, userLevel)
		}
		if section == SectionSettings && !d.prefix && cfg != nil {
			f.Editable = settingsFieldEdit(d, f.Set, items, entries, byName, cfg, root, userLevel)
		}
		fields = append(fields, f)
	}
	return fields
}

// fieldSources lists the settings specs in items that set d, with a
// personal file and the team spec it extends as separate sources.
func fieldSources(d fieldDef, items []SectionItem, byName map[string]spec.Entry, root string) []FieldSource {
	set := []FieldSource{}
	for _, it := range items {
		if it.Kind != string(spec.KindSettings) {
			continue
		}
		e, ok := byName[it.Name]
		if !ok {
			continue
		}
		values := d.values(it.Values)
		cleared := clearedGroup(d, e)
		if e.Base == nil {
			if cleared != "" {
				values = []Value{{Key: cleared, Type: ValueText, Value: "null"}}
			}
			if len(values) > 0 {
				set = append(set, FieldSource{Spec: it.Name, Path: it.Path, Values: values, Local: LocalLayer(e)})
			}
			continue
		}
		own := d.values(specValues(e.LocalMeta))
		if cleared != "" {
			own = []Value{{Key: cleared, Type: ValueText, Value: "null"}}
		}
		team := SpecPath(root, *e.Base)
		if !d.prefix && len(own) == 0 {
			if len(values) > 0 {
				set = append(set, FieldSource{Spec: it.Name, Path: team, Values: values})
			}
			continue
		}
		if len(own) > 0 {
			set = append(set, FieldSource{Spec: it.Name, Path: it.Path, Values: own, Local: true})
		}
		if shared := d.values(settingsValues(*e.Base)); len(shared) > 0 {
			set = append(set, FieldSource{Spec: it.Name, Path: team, Values: shared})
		}
	}
	return set
}

// clearedGroup is the settings group a personal file clears with
// key: null that covers d, or "".
func clearedGroup(d fieldDef, e spec.Entry) string {
	switch {
	case strings.HasPrefix(d.key, "permissions.") && personalPermissionsCleared(e):
		return "permissions"
	case d.voice() && personalFieldCleared(e, "voice"):
		return "voice"
	case d.key == ProtectedKey && personalFieldCleared(e, ProtectedKey):
		return ProtectedKey
	}
	return ""
}

// authorityFieldEdit is the editor of an Authority field, or nil for a
// key that has none.
func authorityFieldEdit(d fieldDef, entries []spec.Entry, cfg *config.Config, root string, userLevel bool) *FieldEdit {
	switch {
	case d.key == PermissionModeKey:
		edit := PermissionFieldEdit(entries, cfg, root, userLevel)
		edit.Kind = "text"
		edit.Allowed = slices.Clone(spec.PermissionModes)
		if !userLevel {
			edit.DisabledReason = PermissionModeProjectReason
		}
		return edit
	case d.key == ProtectedKey:
		return protectedFieldEdit(entries, cfg, root, userLevel)
	case PermissionListField(d.key):
		return PermissionFieldEdit(entries, cfg, root, userLevel)
	}
	return nil
}

// settingsFieldEdit is the editor of a Settings field. Its disabled
// reasons are applied in order, and a later rule replaces an earlier one.
func settingsFieldEdit(d fieldDef, set []FieldSource, items []SectionItem, entries []spec.Entry, byName map[string]spec.Entry, cfg *config.Config, root string, userLevel bool) *FieldEdit {
	edit := settingsFieldKind(d, cfg.Targets, userLevel)
	paths := settingsEditPaths(d, edit, items, byName, root, userLevel)
	settingsEditTarget(d, edit, paths, entries, byName, cfg, root, userLevel)
	settingsDisabledReason(d, edit, set, entries, byName, root, userLevel)
	return edit
}

// settingsFieldKind starts the editor of d with its value kind, the
// values it allows, and the rule that limits it to sync --scope user.
func settingsFieldKind(d fieldDef, targets []string, userLevel bool) *FieldEdit {
	edit := &FieldEdit{Kind: "text", SyncCommand: "agnostic-ai sync"}
	if userLevel {
		edit.SyncCommand = "agnostic-ai sync --scope user"
	}
	switch {
	case d.key == "session-retention-days":
		edit.Kind = "number"
		if !userLevel {
			edit.DisabledReason = "Session retention is written only by sync --scope user. Open Settings in agnostic-ai ui --scope user."
		}
	case d.voice():
		if d.key == "voice.enabled" {
			edit.Kind = "boolean"
		}
		if d.key == "voice.mode" {
			edit.Allowed = slices.Clone(spec.VoiceModes)
		}
		if !userLevel {
			edit.DisabledReason = voiceUserOnlyReason
		}
	case d.key == "effort":
		addEffortLevels(edit, targets)
	}
	return edit
}

// addEffortLevels sets the effort levels each target accepts, sorted,
// and their sorted union.
func addEffortLevels(edit *FieldEdit, targets []string) {
	for _, target := range targets {
		adapter, err := adapters.Resolve(target)
		if err != nil {
			continue
		}
		accepter, ok := adapter.(adapters.SettingsEffortAccepter)
		if !ok {
			continue
		}
		levels := accepter.SettingsEffortLevels()
		if edit.ToolAllowed == nil {
			edit.ToolAllowed = map[string][]string{}
		}
		edit.ToolAllowed[target] = slices.Sorted(slices.Values(levels))
		if edit.ToolAllowed[target] == nil {
			edit.ToolAllowed[target] = []string{}
		}
		for _, level := range levels {
			if !slices.Contains(edit.Allowed, level) {
				edit.Allowed = append(edit.Allowed, level)
			}
		}
	}
	slices.Sort(edit.Allowed)
}

// settingsEditPaths adds the team path of every settings spec that can
// take d, and returns those paths by spec name.
func settingsEditPaths(d fieldDef, edit *FieldEdit, items []SectionItem, byName map[string]spec.Entry, root string, userLevel bool) map[string]string {
	paths := map[string]string{}
	for _, it := range items {
		e, ok := byName[it.Name]
		if !ok || it.Kind != string(spec.KindSettings) || !SettingsEditCandidate(e, d.key) {
			continue
		}
		paths[it.Name] = SettingsTeamPath(root, e)
		edit.Paths = append(edit.Paths, paths[it.Name])
		if d.voice() {
			edit.Sources = append(edit.Sources, FieldEditSource{Path: paths[it.Name], Spec: e.Name, LocalPath: localEditPath(root, e, userLevel), DisabledReason: VoiceEditDisabledReason(e, root)})
		}
	}
	return paths
}

// settingsEditTarget picks the file an edit of d writes, or the reason
// none can, and the personal file beside it.
func settingsEditTarget(d fieldDef, edit *FieldEdit, paths map[string]string, entries []spec.Entry, byName map[string]spec.Entry, cfg *config.Config, root string, userLevel bool) {
	defaultPath := "settings/defaults.yaml"
	if !userLevel {
		defaultPath = displayPath(root, filepath.Join(config.ResolveSourcePath(root, cfg.Sources.Settings), "defaults.yaml"))
	}
	inEffect := SettingsInEffect(entries, d.key, cfg.Targets, userLevel)
	if target := SettingsWriteTarget(entries, inEffect, d.key, cfg.Targets); target >= 0 {
		edit.Path = paths[entries[target].Name]
	} else if inEffect >= 0 || len(edit.Paths) == 0 {
		if taken, ok := byName["defaults"]; ok {
			edit.DisabledReason = DefaultsTakenReason(root, defaultPath, taken)
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
		edit.LocalPath = localEditPath(root, entries[at], userLevel)
	} else if edit.Path == defaultPath {
		edit.LocalPath = SettingsOverlayPath(userLevel, "defaults")
	}
}

// settingsDisabledReason applies the rules that disable an edit of d, in
// order: a non-scalar value, a model tier, then the voice rules.
func settingsDisabledReason(d fieldDef, edit *FieldEdit, set []FieldSource, entries []spec.Entry, byName map[string]spec.Entry, root string, userLevel bool) {
	for _, source := range set {
		for _, value := range source.Values {
			if d.voice() && source.Local && value.Key == "voice" && personalFieldCleared(byName[source.Spec], "voice") {
				continue
			}
			if !d.editableValue(value) {
				edit.DisabledReason = "This field has a non-scalar value; edit the spec directly."
			}
		}
	}
	if d.key == "model" && slices.ContainsFunc(entries, func(e spec.Entry) bool {
		return e.ModelTier != "" && slices.ContainsFunc(set, func(s FieldSource) bool { return s.Spec == e.Name })
	}) {
		edit.DisabledReason = "This model names a tier from models: in the config; edit the spec directly."
	}
	if !d.voice() {
		return
	}
	if !userLevel {
		edit.DisabledReason = voiceUserOnlyReason
		return
	}
	if len(edit.Paths) > 1 {
		return
	}
	for _, e := range entries {
		if reason := VoiceEditDisabledReason(e, root); reason != "" && (edit.Path == "" || SettingsTeamPath(root, e) == edit.Path) {
			edit.DisabledReason = reason
			return
		}
	}
}

// localEditPath is the personal file an edit of e writes: e itself when
// it is a local file, its overlay file otherwise.
func localEditPath(root string, e spec.Entry, userLevel bool) string {
	if LocalLayer(e) {
		return SpecPath(root, e)
	}
	return SettingsOverlayPath(userLevel, e.Name)
}

// PermissionModeKey is the settings field that holds the default
// permission mode.
const PermissionModeKey = "permissions.default-mode"

// PermissionModeProjectReason explains why a project cannot edit the
// default mode: only sync --scope user writes Claude Code's defaultMode.
const PermissionModeProjectReason = "Applies through user-level sync only. Set it in user-level settings."

// ProtectedKey is the settings block that lists protected paths.
const ProtectedKey = "protected"

// protectedFieldEdit edits the protected block in the spec the permission
// lists edit. A personal permissions: null leaves it editable, as it
// clears only the permissions group.
func protectedFieldEdit(entries []spec.Entry, cfg *config.Config, root string, userLevel bool) *FieldEdit {
	edit := PermissionFieldEdit(entries, cfg, root, userLevel)
	edit.Kind = "protected-list"
	for i := range edit.Sources {
		edit.Sources[i].DisabledReason = ""
	}
	if userLevel {
		edit.DisabledReason = "sync --scope user does not write protected paths. Protect files from a project's Authority page."
	}
	return edit
}

func PermissionListField(key string) bool {
	list, ok := strings.CutPrefix(key, "permissions.")
	return ok && slices.Contains(spec.PermissionLists, list)
}

func PermissionEditCandidate(e spec.Entry) bool {
	if e.Layer == builtinLayer || strings.HasPrefix(e.Layer, "pack:") {
		return false
	}
	return e.Base == nil || e.Base.Layer != builtinLayer && !strings.HasPrefix(e.Base.Layer, "pack:")
}

func personalPermissionsCleared(e spec.Entry) bool {
	return personalFieldCleared(e, "permissions")
}

func personalFieldCleared(e spec.Entry, key string) bool {
	if !LocalLayer(e) {
		return false
	}
	meta := e.LocalMeta
	if e.Base == nil {
		meta = e.Meta
	}
	value, set := meta[key]
	return set && value == nil
}

func VoiceEditDisabledReason(e spec.Entry, root string) string {
	if !personalFieldCleared(e, "voice") {
		return ""
	}
	return SpecPath(root, e) + " sets voice: null, which clears every voice setting. Remove voice: null from that local file before editing an individual setting."
}

func PermissionEditDisabledReason(e spec.Entry, root string) string {
	if !personalPermissionsCleared(e) {
		return ""
	}
	return SpecPath(root, e) + " sets permissions: null, which clears every permission list. Remove permissions: null from that local file before editing an individual list."
}

func PermissionFieldEdit(entries []spec.Entry, cfg *config.Config, root string, userLevel bool) *FieldEdit {
	edit := &FieldEdit{Kind: "permission-list", SyncCommand: "agnostic-ai sync"}
	if userLevel {
		edit.SyncCommand += " --scope user"
	}
	var candidates []spec.Entry
	for _, e := range entries {
		if PermissionEditCandidate(e) && (cfg == nil || len(cfg.Targets) == 0 || slices.ContainsFunc(cfg.Targets, e.EmitsTo)) {
			candidates = append(candidates, e)
			path := SettingsTeamPath(root, e)
			edit.Paths = append(edit.Paths, path)
			edit.Sources = append(edit.Sources, FieldEditSource{Path: path, Spec: e.Name, LocalPath: localEditPath(root, e, userLevel), DisabledReason: PermissionEditDisabledReason(e, root)})
		}
	}
	if len(candidates) == 1 {
		edit.Path = edit.Paths[0]
		edit.LocalPath = edit.Sources[0].LocalPath
		return edit
	}
	if len(candidates) > 1 {
		return edit
	}
	if len(entries) > 0 {
		edit.DisabledReason = "These settings are read-only or do not apply to the configured tools; add an editable settings spec."
		return edit
	}
	file := filepath.Join(root, "settings", "permissions.yaml")
	if !userLevel {
		source := config.SourceBaseDir + "/settings"
		if cfg != nil && cfg.Sources.Settings != "" {
			source = cfg.Sources.Settings
		}
		file = filepath.Join(config.ResolveSourcePath(root, source), "permissions.yaml")
	}
	edit.Path = displayPath(root, file)
	edit.LocalPath = SettingsOverlayPath(userLevel, "permissions")
	edit.Sources = []FieldEditSource{{Path: edit.Path, Spec: "permissions", LocalPath: edit.LocalPath}}
	return edit
}

// editableValue reports whether the page can edit v in place: the
// field's own scalar, or one tool's scalar in its per-tool map.
func (d fieldDef) editableValue(v Value) bool {
	if d.key == "session-retention-days" || strings.HasPrefix(d.key, "voice.") {
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
	return e.Layer == "local" || e.Layer == "user-local"
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
// user source root, that overlays the settings spec named name.
func SettingsOverlayPath(userLevel bool, name string) string {
	dir := config.SourceBaseDir + "/local/settings"
	if userLevel {
		dir = "local/settings"
	}
	return dir + "/" + name + ".yaml"
}

func (d fieldDef) voice() bool {
	return strings.HasPrefix(d.key, "voice.")
}

func (d fieldDef) matches(key string) bool {
	if d.prefix {
		return strings.HasPrefix(key, d.key)
	}
	return key == d.key || strings.HasPrefix(key, d.key+".")
}

// DefaultsTakenReason says why an edit cannot create the settings spec
// named defaults, which defaults already is, and what to create instead.
func DefaultsTakenReason(root, defaultPath string, defaults spec.Entry) string {
	path := SpecPath(root, defaults)
	if grantsAuthority(defaults) {
		return path + " sets permission rules or protected files, so an edit adds no other field to it; create another settings spec, such as " +
			filepath.ToSlash(filepath.Join(filepath.Dir(defaultPath), "editor.yaml")) + ", and set the field there."
	}
	return path + " is already the settings spec named defaults; add the field to an editable settings spec."
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
func SettingsInEffect(settings []spec.Entry, key string, targets []string, userLevel bool) int {
	last := -1
	for i, setting := range slices.Backward(settings) {
		if !hasField(setting, key) {
			continue
		}
		if last < 0 {
			last = i
		}
		if !slices.ContainsFunc(targets, func(t string) bool { return !setsFor(setting, key, t) }) && SettingsOverride(settings, i, key, targets, userLevel) < 0 {
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
func SettingsOverride(settings []spec.Entry, i int, key string, targets []string, userLevel bool) int {
	applies := appliesTo(settings, i, targets)
	for j := len(settings) - 1; j > i; j-- {
		if hasField(settings[j], key) && !slices.ContainsFunc(applies, func(t string) bool { return !overridesFor(settings[j], key, t, userLevel) }) {
			return j
		}
	}
	return -1
}

// overridesFor is setsFor for a later spec, without the Claude model
// name target cannot load, as project sync drops it. sync --scope user
// writes it.
func overridesFor(e spec.Entry, key, target string, userLevel bool) bool {
	if key == "model" && !userLevel && foreignClaudeModel(e, target) {
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

// SettingsFieldValue reads a portable field, including its nested members.
func SettingsFieldValue(meta map[string]any, key string) (any, bool) {
	var value any = meta
	for _, part := range strings.Split(key, ".") {
		mapping, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok = mapping[part]
		if !ok {
			return nil, false
		}
	}
	return value, true
}
