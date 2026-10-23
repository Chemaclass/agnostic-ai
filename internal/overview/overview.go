package overview

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Inputs is everything Build reads. The caller loads the configuration
// and builds the snapshot; Build only arranges what it is given, so the
// same inputs always give the same view.
type Inputs struct {
	Cfg    *config.Config
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
	id, label, question, deepView string
	rows                          []row
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
		id: SectionAuthority, label: "Authority", question: "What may the agent do?", deepView: "/authority",
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

// lifecycleOrder is the order of the portable hook events in a session.
var lifecycleOrder = []string{"session-start", "prompt-submit", "before-tool", "after-tool", "after-edit", "stop", "session-end"}

var lifecycleLabels = map[string]string{
	"session-start": "Session start",
	"prompt-submit": "Prompt submit",
	"before-tool":   "Before a tool runs",
	"after-tool":    "After a tool runs",
	"after-edit":    "After a file edit",
	"stop":          "Stop",
	"session-end":   "Session end",
}

// hookEvent is the lifecycle point a hook runs at: its portable `on`
// event by its label, or the native `event` as the tool names it.
func hookEvent(e spec.Entry) string {
	if on, ok := e.Meta["on"].(string); ok && on != "" {
		if label, known := lifecycleLabels[on]; known {
			return label
		}
		return on
	}
	if event, ok := e.Meta["event"].(string); ok && event != "" {
		return event
	}
	return "No event"
}

// groupRank orders hook groups by their place in a session; native
// event names follow, alphabetically.
func groupRank(group string) int {
	for i, on := range lifecycleOrder {
		if lifecycleLabels[on] == group {
			return i
		}
	}
	return len(lifecycleOrder)
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
		item := ItemFinding{Source: f.Source, Code: f.Code, Severity: f.Severity, Message: f.Message, Path: f.Path, Line: f.Line, Fix: f.Fix, Explanation: f.Explanation}
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
			DeepView: def.deepView,
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
				items = builtinItems(in.Cfg)
			case kindConfig:
				items = configItems(in.Cfg, in.Snapshot.ConfigFiles)
			default:
				items = specItems(in, r, def.id)
			}
			if r.optional && len(items) == 0 {
				continue
			}
			for i := range items {
				items[i].Findings = []ItemFinding{}
			}
			s.Items = append(s.Items, items...)
			s.Kinds = append(s.Kinds, KindCount{Kind: r.id(), Label: r.title(), Count: len(items)})
			if r.special == "" {
				s.Support = append(s.Support, kindSupport(r.kind, r.title(), targets, caps))
			}
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
	for _, e := range entries {
		item := sectionItem(in.Snapshot.Project.Root, e, section)
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

func builtinItems(cfg *config.Config) []SectionItem {
	if cfg == nil {
		return []SectionItem{}
	}
	items := make([]SectionItem, 0, len(cfg.Builtins))
	for _, name := range cfg.Builtins {
		description := builtinDescriptions[name]
		if description == "" {
			description = "Built-in feature."
		}
		items = append(items, SectionItem{Kind: kindBuiltin, Name: name, Label: name, Description: description, Path: "builtin:" + name, Layer: builtinLayer})
	}
	return items
}

// configItems lists the config files read, each described by what the
// loaded configuration sets.
func configItems(cfg *config.Config, files []string) []SectionItem {
	var facts []string
	if cfg != nil {
		if len(cfg.Targets) > 0 {
			facts = append(facts, "targets "+strings.Join(cfg.Targets, ", "))
		}
		if cfg.Requires != "" {
			facts = append(facts, "requires "+cfg.Requires)
		}
		if len(cfg.Models) > 0 {
			tiers := make([]string, 0, len(cfg.Models))
			for name := range cfg.Models {
				tiers = append(tiers, name)
			}
			slices.Sort(tiers)
			facts = append(facts, "model tiers "+strings.Join(tiers, ", "))
		}
	}
	description := "Sets no targets, version requirement, or model tiers."
	if len(facts) > 0 {
		description = "Sets " + strings.Join(facts, "; ") + "."
	}
	items := make([]SectionItem, 0, len(files))
	for _, f := range files {
		layer := "config"
		if strings.HasPrefix(filepath.ToSlash(f), "local/") {
			layer = "local"
		}
		items = append(items, SectionItem{Kind: kindConfig, Name: filepath.Base(f), Label: filepath.ToSlash(f), Description: description, Path: filepath.ToSlash(f), Layer: layer})
	}
	return items
}

func sectionItem(root string, e spec.Entry, section string) SectionItem {
	description := e.Description()
	if description == "" && e.Kind == spec.KindSettings {
		description = settingsSummary(e, section)
	}
	return SectionItem{
		Kind:        string(e.Kind),
		Name:        e.Name,
		Label:       e.Name,
		Description: description,
		Path:        SpecPath(root, e),
		Layer:       e.Layer,
		Scope:       e.EffectiveScope(),
	}
}

// settingsSummary describes a settings spec that has no description by
// the fields that place it in section.
func settingsSummary(e spec.Entry, section string) string {
	var fields []string
	if section == SectionAuthority {
		if hasField(e, "permissions") {
			fields = append(fields, "permission rules")
		}
		if hasField(e, "protected") {
			fields = append(fields, "protected paths")
		}
	} else {
		if hasField(e, "model") {
			fields = append(fields, "the model")
		}
		if hasField(e, "effort") {
			fields = append(fields, "the effort")
		}
	}
	if len(fields) == 0 {
		return "Sets target-specific settings."
	}
	return "Sets " + strings.Join(fields, " and ") + "."
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
	"continue":    "Continue",
	"copilot":     "GitHub Copilot",
	"crush":       "Crush",
	"cursor":      "Cursor",
	"factory":     "Factory Droid",
	"gemini":      "Gemini CLI",
	"goose":       "Goose",
	"jules":       "Jules",
	"junie":       "Junie",
	"kilo":        "Kilo Code",
	"kiro":        "Kiro",
	"opencode":    "OpenCode",
	"openhands":   "OpenHands",
	"qoder":       "Qoder",
	"trae":        "Trae",
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
