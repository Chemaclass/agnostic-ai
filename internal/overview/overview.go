package overview

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/builtins"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/mcpprobe"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Inputs is everything Build reads. The caller loads the configuration
// and builds the snapshot; Build only arranges what it is given, so the
// same inputs always give the same view.
type Inputs struct {
	Cfg    *config.Config
	Global bool
	Bundle spec.Bundle
	// Snapshot identifies the configuration the inputs came from.
	Snapshot authority.Snapshot
	// Instructions are the always-on instruction files, such as
	// AGNOSTIC_AI.md and its local layer, in load order.
	Instructions []Document
	// Findings are the lint, doctor, and drift results the caller
	// collected, with paths as the view shows them.
	Findings []Finding
	// Compared are the targets whose generated files the drift check
	// compared with disk; drift findings name only these.
	Compared []string
	// MCPTools is the cached tool listing of the MCP servers.
	MCPTools mcpprobe.Cache
	// RefreshCommand lists the MCP servers' tools again for this
	// configuration.
	RefreshCommand string
	// BuiltinsTeam is the committed config's builtins list, and
	// BuiltinsLocal whether the personal config replaces it.
	BuiltinsTeam  []string
	BuiltinsLocal bool
	// FieldNotes are the settings model and effort notes the drift
	// check's render raised.
	FieldNotes []FieldNote
	// NativeMCPs are the MCP servers the caller found in the configured
	// tools' own config files, whether or not a spec covers them.
	NativeMCPs []NativeMCP
}

// NativeMCP is one MCP server a tool's own config file defines. It
// holds strings only: the caller never passes a server's values.
type NativeMCP struct {
	Name, Target, Path, Scope, Transport string
	// Import is the command that turns the server into a spec, or empty
	// when no import reads Path.
	Import string
}

// builtinLayer is the layer name the loader gives a built-in spec. Its
// files live in a cache outside the project, so views name it by spec
// instead of by path.
const builtinLayer = "builtin"

// Row kinds that are not spec kinds.
const (
	kindInstructions = "instructions"
	kindBuiltin      = "builtin"
	kindConfig       = "config"
)

// kindLabels names each spec kind as a section lists it.
var kindLabels = map[spec.Kind]string{
	spec.KindRule:        "Rules",
	spec.KindSkill:       "Skills",
	spec.KindIgnore:      "Ignore files",
	spec.KindAgent:       "Agents",
	spec.KindHook:        "Hooks",
	spec.KindCommand:     "Commands",
	spec.KindSettings:    "Settings",
	spec.KindMCP:         "MCP servers",
	spec.KindEnvironment: "Environments",
	spec.KindReview:      "Review instructions",
}

// row is one line of a section: a spec kind, optionally narrowed to the
// specs the section reads, or one of the rows that are not spec kinds.
type row struct {
	kind spec.Kind
	// special names a row that is not a spec kind: instructions,
	// built-ins, or config.
	special string
	// label replaces the kind's own label in this section.
	label string
	// keep narrows a kind that two sections split, such as settings.
	keep func(spec.Entry) bool
	// optional rows show only when they hold something: kinds a project
	// can have and the user-level configuration cannot.
	optional bool
	// group places each item within the row, such as a hook's
	// lifecycle point.
	group func(spec.Entry) string
}

func (r row) id() string {
	if r.special != "" {
		return r.special
	}
	return string(r.kind)
}

func (r row) title() string {
	if r.label != "" {
		return r.label
	}
	return kindLabels[r.kind]
}

type sectionDef struct {
	id, label, question string
	rows                []row
}

// sectionDefs are the harness sections in order.
var sectionDefs = []sectionDef{
	{
		id: SectionContext, label: "Context", question: "What is the agent told?",
		rows: []row{
			{special: kindInstructions, label: "Instructions"},
			{kind: spec.KindRule},
			{kind: spec.KindAgent},
			{kind: spec.KindSkill},
			{kind: spec.KindCommand, optional: true},
			{kind: spec.KindReview, optional: true},
			{kind: spec.KindIgnore, optional: true},
		},
	},
	{
		id: SectionAuthority, label: "Authority", question: "What may the agent do?",
		rows: []row{{kind: spec.KindSettings, label: "Permissions", keep: grantsAuthority}},
	},
	{
		id: SectionTools, label: "Tools", question: "What can the agent use?",
		rows: []row{{kind: spec.KindMCP}, {kind: spec.KindEnvironment, optional: true}},
	},
	{
		id: SectionLifecycle, label: "Lifecycle", question: "What runs around the agent, and when?",
		rows: []row{{kind: spec.KindHook, group: hookEvent}},
	},
	{
		id: SectionSettings, label: "Settings", question: "How is the harness configured?",
		rows: []row{
			{special: kindBuiltin, label: "Built-ins"},
			{special: kindConfig, label: "Config"},
			{kind: spec.KindSettings, label: "Other settings", keep: shapesExecution},
		},
	},
}

// grantsAuthority keeps the settings specs that set permission rules or
// protected paths.
func grantsAuthority(e spec.Entry) bool {
	return hasField(e, "permissions") || hasField(e, "protected")
}

// shapesExecution keeps every settings spec that sets no permission
// rules or protected paths: model, effort, retention, and the native
// keys of each tool.
func shapesExecution(e spec.Entry) bool {
	return !grantsAuthority(e)
}

func hasField(e spec.Entry, key string) bool {
	_, ok := e.Meta[key]
	return ok
}

// lifecycleStages are the portable hook events, in session order, with
// what a hook at each does.
var lifecycleStages = []struct{ id, label, description string }{
	{"session-start", "Session start", "When a session opens: load context or check the environment."},
	{"prompt-submit", "Prompt submit", "Before the agent reads a prompt; exit 2 blocks the prompt."},
	{"before-tool", "Before a tool runs", "Before each tool call; exit 2 blocks the call. Guards live here."},
	{"after-tool", "After a tool runs", "After each tool call: check, log, or react to the result."},
	{"after-edit", "After a file edit", "After the agent edits a file: format or lint it."},
	{"stop", "Stop", "When the agent finishes; exit 2 keeps it working."},
	{"session-end", "Session end", "When a session closes: save state or clean up."},
}

// nativeStagePrefix marks a stage named by one tool's own event.
const nativeStagePrefix = "native:"

// hookEvent is the stage a hook runs at: its portable `on` event, or
// its native `event` as the tool names it.
func hookEvent(e spec.Entry) string {
	if on, ok := e.Meta["on"].(string); ok && on != "" {
		return on
	}
	if event, ok := e.Meta["event"].(string); ok && event != "" {
		return nativeStagePrefix + event
	}
	return nativeStagePrefix + "none"
}

// groupRank orders hook groups by their place in a session; native
// event names follow, alphabetically.
func groupRank(group string) int {
	for i, st := range lifecycleStages {
		if st.id == group {
			return i
		}
	}
	return len(lifecycleStages)
}

// buildStages lists every portable stage with the configured targets
// that run it, then one stage per native event a hook names.
func buildStages(items []SectionItem, targets []string, labels labeler) []Stage {
	stages := make([]Stage, 0, len(lifecycleStages))
	for _, st := range lifecycleStages {
		stage := Stage{ID: st.id, Label: st.label, Description: st.description, Targets: []StageTarget{}}
		for _, t := range targets {
			if event, ok := spec.PortableHookEvent(t, st.id); ok {
				stage.Targets = append(stage.Targets, StageTarget{Target: t, Label: labels.of(t), Event: event})
			}
		}
		stages = append(stages, stage)
	}
	seen := map[string]bool{}
	for _, it := range items {
		if strings.HasPrefix(it.Group, nativeStagePrefix) && !seen[it.Group] {
			seen[it.Group] = true
			name := strings.TrimPrefix(it.Group, nativeStagePrefix)
			stages = append(stages, Stage{ID: it.Group, Label: name, Description: "A tool's own event name: it runs only on the tool that defines " + name + ".", Targets: []StageTarget{}})
		}
	}
	return stages
}

// Build arranges the inputs into the overview: the sections and the
// configured targets' support.
func Build(in Inputs) *View {
	caps := capabilities()
	sections := buildSections(in, caps)
	placeFindings(sections, in.Findings)
	targets := targetSupport(targetsOf(in.Cfg), caps, newLabeler())
	syncStates(targets, in.Findings, in.Compared)
	return &View{
		SchemaVersion: SchemaVersion,
		Snapshot:      normalizedSnapshot(in.Snapshot),
		Sections:      sections,
		Targets:       targets,
	}
}

// placeFindings puts each lint and doctor finding on every item whose
// file it names, or inside whose skill folder it points. A finding that
// names no configured item goes to the section it states, else to
// Settings, which holds the configuration as a whole.
func placeFindings(sections []Section, findings []Finding) {
	for _, f := range findings {
		if f.Source == SourceDrift {
			continue
		}
		item := ItemFinding{Source: f.Source, Code: f.Code, Severity: f.Severity, Message: f.Message, Path: f.Path, Line: f.Line, OpenPath: f.OpenPath, Fix: f.Fix, Explanation: f.Explanation}
		placed := false
		for si := range sections {
			for ii := range sections[si].Items {
				if names(sections[si].Items[ii], f.Path) {
					sections[si].Items[ii].Findings = append(sections[si].Items[ii].Findings, item)
					placed = true
				}
			}
		}
		if placed {
			continue
		}
		section := f.Section
		if !slices.ContainsFunc(sections, func(s Section) bool { return s.ID == section }) {
			section = SectionSettings
		}
		for si := range sections {
			if sections[si].ID == section {
				sections[si].Findings = append(sections[si].Findings, item)
			}
		}
	}
}

// names reports whether a finding at path concerns item: the same file,
// or a file inside the folder of a skill.
func names(item SectionItem, path string) bool {
	if path == "" || item.Path == "" {
		return false
	}
	if path == item.Path {
		return true
	}
	if item.Kind == string(spec.KindSkill) {
		return strings.HasPrefix(path, strings.TrimSuffix(item.Path, "/SKILL.md")+"/")
	}
	return false
}

// syncStates sets each target's drift state from the drift findings:
// drifted with its files, in-sync when it was compared and nothing
// differs, not-checked otherwise.
func syncStates(targets []TargetSupport, findings []Finding, compared []string) {
	for i := range targets {
		t := &targets[i]
		t.Sync = TargetSync{State: SyncNotChecked, Files: []SyncFile{}}
		if !slices.Contains(compared, t.Target) {
			continue
		}
		t.Sync.State = SyncInSync
		for _, f := range findings {
			if f.Source == SourceDrift && f.Target == t.Target {
				t.Sync.State = SyncDrifted
				t.Sync.Files = append(t.Sync.Files, SyncFile{Path: f.Path, State: f.Message})
			}
		}
		slices.SortFunc(t.Sync.Files, func(a, b SyncFile) int { return strings.Compare(a.Path, b.Path) })
	}
}

func buildSections(in Inputs, caps map[string][]spec.Kind) []Section {
	targets := targetsOf(in.Cfg)
	sections := make([]Section, 0, len(sectionDefs))
	for i, def := range sectionDefs {
		s := Section{
			ID:       def.id,
			Label:    def.label,
			Question: def.question,
			Order:    i + 1,
			Kinds:    []KindCount{},
			Items:    []SectionItem{},
			Support:  []KindSupport{},
			Findings: []ItemFinding{},
		}
		for _, r := range def.rows {
			var items []SectionItem
			switch r.special {
			case kindInstructions:
				items = instructionItems(in.Instructions)
			case kindBuiltin:
				items = builtinItems(in)
			case kindConfig:
				items = configItems(in.Cfg, in.Snapshot.Project.Root, in.Snapshot.ConfigFiles)
			default:
				items = specItems(in, r, def.id)
			}
			if r.optional && len(items) == 0 {
				continue
			}
			for i := range items {
				items[i].Findings = []ItemFinding{}
				if items[i].Values == nil {
					items[i].Values = []Value{}
				}
			}
			s.Items = append(s.Items, items...)
			count := len(items)
			if r.special == kindBuiltin {
				count = 0
				for _, it := range items {
					if it.Enabled != nil && *it.Enabled {
						count++
					}
				}
			}
			s.Kinds = append(s.Kinds, KindCount{Kind: r.id(), Label: r.title(), Count: count, Local: r.special == kindBuiltin && in.BuiltinsLocal})
			if r.special == "" {
				s.Support = append(s.Support, kindSupport(r.kind, r.title(), targets, caps))
			}
		}
		if def.id == SectionLifecycle {
			s.Stages = buildStages(s.Items, targets, newLabeler())
		}
		fieldItems := s.Items
		if def.id == SectionSettings {
			for _, prev := range sections {
				if prev.ID == SectionAuthority {
					fieldItems = append(slices.Clone(s.Items), prev.Items...)
				}
			}
		}
		s.Fields = buildFields(def.id, fieldItems, in.Bundle.Settings, in.Cfg, in.Snapshot.Project.Root, in.Global)
		if def.id == SectionSettings {
			labels := newLabeler()
			for i := range s.Fields {
				s.Fields[i].Resolved = resolveTargets(in, s.Fields[i].Key, labels, caps)
			}
		}
		if def.id == SectionTools {
			attachTools(s.Items, in)
			s.Native = nativeServers(in, newLabeler())
		}
		sections = append(sections, s)
	}
	return sections
}

func specItems(in Inputs, r row, section string) []SectionItem {
	entries := entriesOf(in.Bundle, r.kind)
	if r.keep != nil {
		entries = slices.DeleteFunc(slices.Clone(entries), func(e spec.Entry) bool { return !r.keep(e) })
	}
	items := make([]SectionItem, 0, len(entries))
	targets := targetsOf(in.Cfg)
	for _, e := range entries {
		if len(targets) > 0 && !slices.ContainsFunc(targets, e.EmitsTo) {
			continue
		}
		item := sectionItem(in.Snapshot.Project.Root, e)
		if r.group != nil {
			item.Group = r.group(e)
		}
		items = append(items, item)
	}
	slices.SortStableFunc(items, func(a, b SectionItem) int {
		if a.Group != b.Group {
			if ra, rb := groupRank(a.Group), groupRank(b.Group); ra != rb {
				return ra - rb
			}
			return strings.Compare(a.Group, b.Group)
		}
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return strings.Compare(a.Path, b.Path)
	})
	return items
}

func instructionItems(docs []Document) []SectionItem {
	items := make([]SectionItem, 0, len(docs))
	for _, d := range docs {
		items = append(items, SectionItem{
			Kind:        kindInstructions,
			Name:        filepath.Base(d.Path),
			Label:       filepath.Base(d.Path),
			Description: fmt.Sprintf("Loaded at the start of every session: %d words.", d.Words),
			Path:        filepath.ToSlash(d.Path),
			OpenPath:    editablePath(d.AbsPath),
			Layer:       d.Layer,
		})
	}
	return items
}

// builtinDescriptions says what each built-in feature does.
var builtinDescriptions = map[string]string{
	"memory":       "Shared memory every coding tool reads and saves facts to.",
	"handoff":      "Saves a task so the next session, in this or another tool, can resume it.",
	"handoff-hook": "Offers a saved handoff when a session starts.",
	"rtk":          "Routes shell commands through rtk to cut output tokens.",
	"caveman":      "Terse replies to cut output tokens.",
}

// builtinItems lists every built-in agnostic-ai ships, each marked on
// when the config enables it and local when the personal config's list
// decides it differently from the team's.
func builtinItems(in Inputs) []SectionItem {
	var enabled []string
	if in.Cfg != nil {
		enabled = in.Cfg.Builtins
	}
	names := builtins.Names()
	for _, name := range enabled {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	items := make([]SectionItem, 0, len(names))
	for _, name := range names {
		description := builtinDescriptions[name]
		if description == "" {
			description = "Built-in feature."
		}
		on := slices.Contains(enabled, name)
		local := in.BuiltinsLocal && on != slices.Contains(in.BuiltinsTeam, name)
		items = append(items, SectionItem{Kind: kindBuiltin, Name: name, Label: name, Description: description, Path: "builtin:" + name, Layer: builtinLayer, Enabled: &on, Local: local})
	}
	return items
}

// configItems lists the config files read, each with the keys the
// loaded configuration sets.
func configItems(cfg *config.Config, root string, files []string) []SectionItem {
	items := make([]SectionItem, 0, len(files))
	for _, f := range files {
		layer := "config"
		if strings.HasPrefix(filepath.ToSlash(f), "local/") {
			layer = "local"
		}
		items = append(items, SectionItem{Kind: kindConfig, Name: filepath.Base(f), Label: filepath.ToSlash(f), Path: filepath.ToSlash(f), OpenPath: editablePath(filepath.Join(root, f)), Layer: layer, Values: configValues(cfg)})
	}
	return items
}

func sectionItem(root string, e spec.Entry) SectionItem {
	description := e.Description()
	item := SectionItem{
		Kind:        string(e.Kind),
		Name:        e.Name,
		Label:       e.Name,
		Description: description,
		Path:        SpecPath(root, e),
		Layer:       e.Layer,
		Scope:       e.EffectiveScope(),
	}
	if e.Kind == spec.KindSettings {
		item.Values = settingsValues(e)
		if LocalLayer(e) {
			own := e.LocalMeta
			if e.Base == nil {
				own = e.Meta
			}
			for _, v := range specValues(own) {
				item.LocalKeys = append(item.LocalKeys, v.Key)
			}
		}
	}
	if e.Layer != builtinLayer {
		item.OpenPath = editablePath(e.Path)
	}
	return item
}

func entriesOf(b spec.Bundle, kind spec.Kind) []spec.Entry {
	switch kind {
	case spec.KindAgent:
		return b.Agents
	case spec.KindSkill:
		return b.Skills
	case spec.KindRule:
		return b.Rules
	case spec.KindHook:
		return b.Hooks
	case spec.KindMCP:
		return b.MCPs
	case spec.KindCommand:
		return b.Commands
	case spec.KindSettings:
		return b.Settings
	case spec.KindReview:
		return b.Reviews
	case spec.KindEnvironment:
		return b.Environments
	case spec.KindIgnore:
		return b.Ignores
	}
	return nil
}

// capabilities maps each built-in target to the spec kinds it emits
// natively, from the adapters' own declarations.
func capabilities() map[string][]spec.Kind {
	matrix := adapters.CapabilityMatrix()
	caps := make(map[string][]spec.Kind, len(matrix))
	for _, t := range matrix {
		caps[t.Name] = t.Supports
	}
	return caps
}

// kindSupport splits the configured targets by whether they emit kind.
// A target the capability matrix does not know, such as an external
// adapter, declares nothing here, so it is in neither list.
func kindSupport(kind spec.Kind, label string, targets []string, caps map[string][]spec.Kind) KindSupport {
	ks := KindSupport{Kind: string(kind), Label: label, Supported: []string{}, Unsupported: []string{}}
	for _, t := range targets {
		kinds, known := caps[t]
		if !known {
			continue
		}
		if slices.Contains(kinds, kind) {
			ks.Supported = append(ks.Supported, t)
		} else {
			ks.Unsupported = append(ks.Unsupported, t)
		}
	}
	return ks
}

func targetSupport(targets []string, caps map[string][]spec.Kind, labels labeler) []TargetSupport {
	out := make([]TargetSupport, 0, len(targets))
	for _, t := range targets {
		ts := TargetSupport{Target: t, Label: labels.of(t), Kinds: []string{}}
		for _, k := range spec.AllKinds {
			if slices.Contains(caps[t], k) {
				ts.Kinds = append(ts.Kinds, string(k))
			}
		}
		out = append(out, ts)
	}
	return out
}

func targetsOf(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	return cfg.Targets
}

// attachTools gives each MCP server item its cached tool listing.
func attachTools(items []SectionItem, in Inputs) {
	meta := map[string]map[string]any{}
	for _, e := range in.Bundle.MCPs {
		meta[e.Name] = e.Meta
	}
	command := in.RefreshCommand
	if command == "" {
		command = "agnostic-ai mcp tools"
	}
	for i := range items {
		if items[i].Kind != string(spec.KindMCP) {
			continue
		}
		listing := &ToolListing{State: ToolsNotListed, Tools: []ToolInfo{}, Command: command}
		if res, ok := in.MCPTools.Servers[items[i].Name]; ok {
			listing.ListedAt = res.ListedAt.UTC().Format("2006-01-02T15:04:05Z")
			for _, t := range res.Tools {
				listing.Tools = append(listing.Tools, ToolInfo{Name: t.Name, Title: t.Title, Description: t.Description})
			}
			switch {
			case res.Error != "":
				listing.State, listing.Error = ToolsFailed, res.Error
			case res.Spec != mcpprobe.SpecFingerprint(meta[items[i].Name]):
				listing.State = ToolsStale
			default:
				listing.State = ToolsListed
			}
		}
		items[i].Tools = listing
	}
}

// nativeServers groups the servers found in tools' own config files by
// name, leaving out each one a spec covers. Names that differ only in
// case share one spec file, so they share one row.
func nativeServers(in Inputs, labels labeler) []NativeServer {
	covered := map[string]bool{}
	for _, m := range in.Bundle.MCPs {
		covered[strings.ToLower(spec.MCPFileName(m.Name))] = true
	}
	byKey := map[string]*NativeServer{}
	var keys []string
	for _, n := range in.NativeMCPs {
		key := strings.ToLower(spec.MCPFileName(n.Name))
		if n.Name == "" || covered[key] {
			continue
		}
		row, ok := byKey[key]
		if !ok {
			row = &NativeServer{Name: n.Name}
			byKey[key] = row
			keys = append(keys, key)
		}
		holder := NativeHolder{Target: n.Target, Label: labels.of(n.Target), Path: n.Path, Scope: n.Scope, Transport: n.Transport}
		if slices.Contains(row.Holders, holder) {
			continue
		}
		row.Holders = append(row.Holders, holder)
		if row.Import == "" {
			row.Import = n.Import
		}
	}
	if len(keys) == 0 {
		return nil
	}
	slices.Sort(keys)
	out := make([]NativeServer, 0, len(keys))
	for _, key := range keys {
		out = append(out, *byKey[key])
	}
	return out
}

// editableExts are the file types a view offers to open in an editor.
var editableExts = map[string]bool{".md": true, ".yaml": true, ".yml": true}

// editablePath is the absolute path of a Markdown or YAML file, resolved
// against the working directory, which a server has changed into, or ""
// for any other file.
func editablePath(path string) string {
	if path == "" || !editableExts[strings.ToLower(filepath.Ext(path))] {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(abs)
}

// SpecPath is the path a view shows for a spec: relative to root with
// forward slashes, or builtin:<name> for a built-in spec, whose files
// live in a cache outside the project.
func SpecPath(root string, e spec.Entry) string {
	if e.Layer == builtinLayer {
		return "builtin:" + e.Name
	}
	return displayPath(root, e.Path)
}

// displayPath makes an absolute path under root relative to it, so a
// view names no machine path beyond the project root.
func displayPath(root, path string) string {
	if path == "" {
		return ""
	}
	if root != "" && filepath.IsAbs(path) {
		if base, err := filepath.Abs(root); err == nil {
			if rel, err := filepath.Rel(base, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				path = rel
			}
		}
	}
	return filepath.ToSlash(path)
}

// normalizedSnapshot turns the snapshot's nil lists into empty ones, so
// the JSON always carries arrays where the contract promises them.
func normalizedSnapshot(s authority.Snapshot) authority.Snapshot {
	if s.Targets == nil {
		s.Targets = []string{}
	}
	if s.ConfigFiles == nil {
		s.ConfigFiles = []string{}
	}
	if s.Specs == nil {
		s.Specs = []authority.SpecRef{}
	}
	return s
}

// targetLabels names each built-in target as a person knows the tool.
var targetLabels = map[string]string{
	"aider":       "Aider",
	"amp":         "Amp",
	"antigravity": "Antigravity",
	"augment":     "Augment Code",
	"claude":      "Claude Code",
	"cline":       "Cline",
	"codex":       "Codex",
	"codebuddy":   "CodeBuddy Code",
	"continue":    "Continue",
	"copilot":     "GitHub Copilot",
	"crush":       "Crush",
	"cursor":      "Cursor",
	"ellipsis":    "Ellipsis",
	"factory":     "Factory Droid",
	"gemini":      "Gemini CLI",
	"goose":       "Goose",
	"grok":        "Grok Build",
	"jules":       "Jules",
	"junie":       "Junie",
	"kilo":        "Kilo Code",
	"kimi":        "Kimi Code CLI",
	"kiro":        "Kiro",
	"opencode":    "OpenCode",
	"openhands":   "OpenHands",
	"pi":          "Pi",
	"qoder":       "Qoder",
	"qwen":        "Qwen Code",
	"trae":        "Trae",
	"vibe":        "Mistral Vibe",
	"warp":        "Warp",
	"windsurf":    "Windsurf",
	"zed":         "Zed",
}

// labeler maps a target name to its human label.
type labeler map[string]string

func newLabeler() labeler {
	l := make(labeler, len(targetLabels))
	for name, label := range targetLabels {
		l[name] = label
	}
	return l
}

func (l labeler) of(target string) string {
	if label := l[target]; label != "" {
		return label
	}
	return target
}
