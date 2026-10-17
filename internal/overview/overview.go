package overview

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Inputs is everything Build reads. The caller loads the project, runs
// the read-only checks, and builds the snapshot; Build only arranges
// what it is given, so the same inputs always give the same view.
type Inputs struct {
	Cfg    *config.Config
	Bundle spec.Bundle
	// Snapshot identifies the configuration the inputs came from. Build
	// copies it into the view and stamps its revision on every card.
	Snapshot authority.Snapshot
	// Authority is the session subject's Authority view on the first
	// configured target, or nil when the caller did not build one.
	Authority *authority.View
	// Findings are the lint, doctor, and drift results the caller
	// collected.
	Findings []Finding
}

// Section statuses.
const (
	statusOverview = "overview"
	statusDeepView = "deep-view"
)

// builtinLayer is the layer name the loader gives a built-in spec. Its
// files live in a cache outside the project, so views name it by spec
// instead of by path.
const builtinLayer = "builtin"

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
// specs the section reads, or a built-in feature from the config.
type row struct {
	kind spec.Kind
	// label replaces the kind's own label in this section.
	label string
	// keep narrows a kind that two sections split, such as settings.
	keep func(spec.Entry) bool
	// builtin names a config builtin this row counts instead of specs.
	builtin     string
	description string
}

func (r row) id() string {
	if r.builtin != "" {
		return r.builtin
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
	id, label, question, description string
	status, deepView                 string
	rows                             []row
}

// sectionDefs are the six harness sections in order.
var sectionDefs = []sectionDef{
	{
		id: SectionContext, label: "Context", question: "What is the agent told?",
		description: "Rules, skills, and ignore files that decide what each coding tool reads.",
		status:      statusOverview,
		rows:        []row{{kind: spec.KindRule}, {kind: spec.KindSkill}, {kind: spec.KindIgnore}},
	},
	{
		id: SectionMemory, label: "Memory", question: "What survives a session?",
		description: "Built-in shared memory and session handoffs that carry facts from one session to the next.",
		status:      statusOverview,
		rows: []row{
			{builtin: "memory", label: "Shared memory", description: "Project memory every coding tool shares, recalled and curated through the shared-memory skill and its policy rule."},
			{builtin: "handoff", label: "Session handoffs", description: "A handoff one session writes and the next resumes, in the same or another coding tool."},
		},
	},
	{
		id: SectionControl, label: "Control", question: "How is execution shaped?",
		description: "Agents, hooks, commands a person invokes, and the model and effort settings that shape how work runs.",
		status:      statusOverview,
		rows: []row{
			{kind: spec.KindAgent},
			{kind: spec.KindHook},
			{kind: spec.KindCommand, label: "Commands (human-invoked prompts)"},
			{kind: spec.KindSettings, keep: shapesExecution},
		},
	},
	{
		id: SectionAuthority, label: "Authority", question: "What may the agent do?",
		description: "Permission rules and protected paths that decide what an agent may do without asking.",
		status:      statusDeepView, deepView: "/authority",
		rows: []row{{kind: spec.KindSettings, keep: grantsAuthority}},
	},
	{
		id: SectionTools, label: "Tools", question: "What can the agent use?",
		description: "MCP servers and environments that give the agent systems to work with.",
		status:      statusOverview,
		rows:        []row{{kind: spec.KindMCP}, {kind: spec.KindEnvironment}},
	},
	{
		id: SectionVerification, label: "Verification", question: "How is work checked?",
		description: "Review instructions that tell a coding tool how to check work.",
		status:      statusOverview,
		rows:        []row{{kind: spec.KindReview}},
	},
}

// grantsAuthority keeps the settings specs that set permission rules or
// protected paths.
func grantsAuthority(e spec.Entry) bool {
	return hasField(e, "permissions") || hasField(e, "protected")
}

// shapesExecution keeps the settings specs that set a model or an
// effort, and those that set none of the portable fields: their
// target-specific keys shape how a tool runs, and no section would
// list them otherwise.
func shapesExecution(e spec.Entry) bool {
	return hasField(e, "model") || hasField(e, "effort") || !grantsAuthority(e)
}

func hasField(e spec.Entry, key string) bool {
	_, ok := e.Meta[key]
	return ok
}

// Build arranges the inputs into the overview: the six sections, the
// feed, the configured targets' support, and a glossary.
func Build(in Inputs) *View {
	labels := newLabeler(in.Authority)
	feed := buildFeed(in, labels)
	caps := capabilities()
	return &View{
		SchemaVersion: SchemaVersion,
		Snapshot:      normalizedSnapshot(in.Snapshot),
		Sections:      buildSections(in, feed, caps),
		Feed:          feed,
		Targets:       targetSupport(targetsOf(in.Cfg), caps, labels),
		Explanations:  explanations(),
	}
}

// BuildFeed returns the feed alone, for agents that only want the cards.
func BuildFeed(in Inputs) *FeedView {
	return &FeedView{
		SchemaVersion: SchemaVersion,
		Snapshot:      normalizedSnapshot(in.Snapshot),
		Feed:          buildFeed(in, newLabeler(in.Authority)),
	}
}

func buildSections(in Inputs, feed []Card, caps map[string][]spec.Kind) []Section {
	targets := targetsOf(in.Cfg)
	var builtins []string
	if in.Cfg != nil {
		builtins = in.Cfg.Builtins
	}
	sections := make([]Section, 0, len(sectionDefs))
	for i, def := range sectionDefs {
		s := Section{
			ID:          def.id,
			Label:       def.label,
			Question:    def.question,
			Description: def.description,
			Order:       i + 1,
			Status:      def.status,
			DeepView:    def.deepView,
			Kinds:       []KindCount{},
			Items:       []SectionItem{},
			Support:     []KindSupport{},
			CardIDs:     []string{},
		}
		for _, r := range def.rows {
			if r.builtin != "" {
				count := 0
				if slices.Contains(builtins, r.builtin) {
					count = 1
					s.Items = append(s.Items, SectionItem{
						Kind:        r.builtin,
						Name:        r.builtin,
						Label:       r.title(),
						Description: r.description,
						Path:        "builtin:" + r.builtin,
						Layer:       builtinLayer,
					})
				}
				s.Kinds = append(s.Kinds, KindCount{Kind: r.id(), Label: r.title(), Count: count})
				continue
			}
			entries := entriesOf(in.Bundle, r.kind)
			if r.keep != nil {
				entries = slices.DeleteFunc(slices.Clone(entries), func(e spec.Entry) bool { return !r.keep(e) })
			}
			items := make([]SectionItem, 0, len(entries))
			for _, e := range entries {
				items = append(items, sectionItem(in.Snapshot.Project.Root, e, def.id))
			}
			slices.SortStableFunc(items, func(a, b SectionItem) int {
				if c := strings.Compare(a.Name, b.Name); c != 0 {
					return c
				}
				return strings.Compare(a.Path, b.Path)
			})
			s.Items = append(s.Items, items...)
			s.Kinds = append(s.Kinds, KindCount{Kind: r.id(), Label: r.title(), Count: len(items)})
			s.Support = append(s.Support, kindSupport(r.kind, r.title(), targets, caps))
		}
		for _, c := range feed {
			if c.SectionID == def.id {
				s.CardIDs = append(s.CardIDs, c.ID)
			}
		}
		sections = append(sections, s)
	}
	return sections
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

func explanations() map[string]string {
	return map[string]string{
		"feed":        "The feed lists up to 50 cards, most consequential first, each derived from a finding a read-only check produced.",
		"priority":    "Cards rank by consequence: security, then drift, coverage, hygiene, and information.",
		"fingerprint": "A fingerprint names a finding across revisions, so a dismissal holds until the finding itself changes.",
		"dismiss":     "Dismissals are kept in your browser in this version.",
		"derived":     "Every card comes from a read-only check and links to its evidence; a deterministic check can still be wrong.",
	}
}

// targetLabels names each built-in target as a person knows the tool.
// A label the Authority view supplies takes precedence.
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

func newLabeler(v *authority.View) labeler {
	l := make(labeler, len(targetLabels))
	for name, label := range targetLabels {
		l[name] = label
	}
	if v != nil {
		for _, o := range v.Harness.Available {
			if o.Target != "" && o.Label != "" {
				l[o.Target] = o.Label
			}
		}
		if v.Harness.Target != "" && v.Harness.Label != "" {
			l[v.Harness.Target] = v.Harness.Label
		}
	}
	return l
}

func (l labeler) of(target string) string {
	if label := l[target]; label != "" {
		return label
	}
	return target
}
