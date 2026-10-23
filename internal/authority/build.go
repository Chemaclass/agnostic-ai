package authority

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Options configures one view.
type Options struct {
	// Root is the project root as the caller gives it, relative or
	// absolute. File matches walk it and Snapshot.Project names it.
	Root string
	// AgentID selects the subject; "" means the session subject.
	AgentID string
	// Target selects the harness; "" means the first configured target.
	Target string
	// Now stamps the snapshot; zero means the current time.
	Now time.Time
	// FileMatches resolves file patterns against the project tree,
	// bounded.
	FileMatches bool
	// Installed holds, per target, the comparison of generated files
	// with disk. Nil means the caller did not compare.
	Installed map[string]InstalledState
	// ConfigFiles are the config files read, relative to Root. They name
	// the snapshot's inputs and enter its revision.
	ConfigFiles []string
	// Global builds the view for the user-level configuration sync
	// --global reads, with Root as the global source directory. sync
	// --global writes no permission rules or protected paths.
	Global bool
}

// NotFoundError reports an agent or target the configuration does not
// define. Valid lists every id that would have worked.
type NotFoundError struct {
	Kind  string
	ID    string
	Valid []string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("unknown %s %q; valid %ss: %s", e.Kind, e.ID, e.Kind, strings.Join(e.Valid, ", "))
}

// builder resolves one view: one subject on one harness.
type builder struct {
	cfg      *config.Config
	b        spec.Bundle
	opts     Options
	target   string
	paths    paths
	subjects []*subjectInfo
	subject  *subjectInfo
	mcps     []*mcpInfo
	// rules and protect decide operations; patternRules and
	// patternProtect also establish resources, and may include specs a
	// proposal adds or removes so both sides of an impact share them.
	rules          []ruleRef
	protect        []protectRef
	patternRules   []ruleRef
	patternProtect []protectRef

	lines      *lineIndex
	permCache  map[string]adapters.CapabilityTranslation
	agentCache map[string]adapters.CapabilityTranslation

	// Facts the uncertainties are built from.
	installedUnchecked bool
	notExpressed       map[string][]string
	unverified         map[string][]string
}

// builtResource is one resource with its operations resolved.
type builtResource struct {
	info *resourceInfo
	res  Resource
}

// Build resolves the Authority view for one subject on one harness.
func Build(cfg *config.Config, b spec.Bundle, opts Options) (*View, error) {
	bd, err := newBuilder(cfg, b, opts, nil)
	if err != nil {
		return nil, err
	}
	return bd.view(), nil
}

// newBuilder validates the selection and parses the specs. extra are
// settings specs that only establish resources.
func newBuilder(cfg *config.Config, b spec.Bundle, opts Options, extra []spec.Entry) (*builder, error) {
	if cfg == nil {
		return nil, errors.New("authority view: no configuration loaded")
	}
	target, err := resolveTarget(cfg, opts.Target)
	if err != nil {
		return nil, err
	}
	bd := &builder{
		cfg:          cfg,
		b:            b,
		opts:         opts,
		target:       target,
		paths:        newPaths(opts.Root),
		lines:        newLineIndex(),
		permCache:    map[string]adapters.CapabilityTranslation{},
		agentCache:   map[string]adapters.CapabilityTranslation{},
		notExpressed: map[string][]string{},
		unverified:   map[string][]string{},
	}
	bd.subjects = parseSubjects(b.Agents, bd.paths.source)
	bd.subject, err = findSubject(bd.subjects, opts.AgentID)
	if err != nil {
		return nil, err
	}
	bd.rules = parseRules(b.Settings, bd.paths, bd.lines)
	if bd.protect, err = parseProtect(b.Settings, bd.paths, bd.lines); err != nil {
		return nil, err
	}
	extraProtect, err := parseProtect(extra, bd.paths, bd.lines)
	if err != nil {
		return nil, err
	}
	bd.patternRules = append(slices.Clone(bd.rules), parseRules(extra, bd.paths, bd.lines)...)
	bd.patternProtect = append(slices.Clone(bd.protect), extraProtect...)
	bd.mcps = parseMCPs(b.MCPs)
	return bd, nil
}

// resolveTarget returns the requested target, or the first configured
// one when none is requested.
func resolveTarget(cfg *config.Config, requested string) (string, error) {
	if requested == "" {
		if len(cfg.Targets) == 0 {
			return "", errors.New("authority view: the configuration lists no targets")
		}
		return cfg.Targets[0], nil
	}
	if !slices.Contains(cfg.Targets, requested) {
		return "", &NotFoundError{Kind: "target", ID: requested, Valid: slices.Clone(cfg.Targets)}
	}
	return requested, nil
}

func findSubject(subjects []*subjectInfo, id string) (*subjectInfo, error) {
	if id == "" {
		id = SessionSubjectID
	}
	ids := make([]string, 0, len(subjects))
	for _, s := range subjects {
		if s.id() == id {
			return s, nil
		}
		ids = append(ids, s.id())
	}
	return nil, &NotFoundError{Kind: "agent", ID: id, Valid: ids}
}

// parseRules reads every permission rule of the settings specs, in
// source order and list order.
func parseRules(settings []spec.Entry, p paths, li *lineIndex) []ruleRef {
	var out []ruleRef
	for _, e := range settings {
		perms, _ := e.Meta["permissions"].(map[string]any)
		for _, list := range spec.PermissionLists {
			rules, _ := perms[list].([]any)
			lines := li.itemLines(e, len(rules), "permissions", list)
			for i, raw := range rules {
				rule, ok := raw.(string)
				if !ok {
					continue
				}
				src := *p.source(e)
				if lines != nil {
					src.Line = lines[i]
				}
				out = append(out, ruleRef{list: list, raw: rule, caps: parseCapability(rule), entry: e, source: src})
			}
		}
	}
	return out
}

// parseProtect reads every protected path of the settings specs.
func parseProtect(settings []spec.Entry, p paths, li *lineIndex) ([]protectRef, error) {
	groups, err := spec.ProtectedPaths(settings)
	if err != nil {
		return nil, fmt.Errorf("read protected paths: %w", err)
	}
	var out []protectRef
	for _, g := range groups {
		var entry spec.Entry
		for _, e := range settings {
			if e.Path == g.Source {
				entry = e
				break
			}
		}
		lines := li.itemLines(entry, len(g.Paths), "protected", "paths")
		for i, path := range g.Paths {
			src := *p.source(entry)
			if lines != nil {
				src.Line = lines[i]
			}
			out = append(out, protectRef{path: path, decision: g.Decision, reason: g.Reason, entry: entry, source: src})
		}
	}
	return out, nil
}

// view assembles the whole document.
func (bd *builder) view() *View {
	s := bd.subject
	providers := builtinProviders()
	defaulted := map[string]bool{}
	for _, m := range bd.mcps {
		p, isDefault := mcpProvider(m, s, bd.paths.source(m.entry))
		providers = append(providers, p)
		defaulted[m.id] = isDefault
	}
	byID := make(map[string]*Provider, len(providers))
	domains := make(map[string][]string, len(providers))
	for i := range providers {
		byID[providers[i].ID] = &providers[i]
		domains[providers[i].ID] = providers[i].DomainIDs
	}
	var files []string
	truncated := false
	if bd.opts.FileMatches && !bd.opts.Global {
		files, truncated = projectFiles(bd.paths.root)
	}
	infos := bd.enumerateResources(s, domains)
	built := make([]*builtResource, 0, len(infos))
	for _, r := range infos {
		br := &builtResource{info: r, res: r.res}
		br.res.Operations = make([]Operation, 0, len(r.actions))
		for _, a := range r.actions {
			br.res.Operations = append(br.res.Operations, bd.operation(s, r, a))
		}
		if r.kind == kindFiles && files != nil {
			br.res.Matches = matchPattern(r.pattern, files, truncated)
		}
		if p := byID[br.res.ProviderID]; p != nil {
			p.ResourceIDs = append(p.ResourceIDs, br.res.ID)
		}
		built = append(built, br)
	}
	applyRoutes(built)
	secrets := buildSecrets(bd.mcps, byID, useRightOf(built), bd.paths.source)
	resources := make([]Resource, 0, len(built))
	edges := []Edge{}
	for _, br := range built {
		resources = append(resources, br.res)
		for _, op := range br.res.Operations {
			edges = append(edges, Edge{
				ID:          s.id() + "|" + op.ID,
				SubjectID:   s.id(),
				ResourceID:  br.res.ID,
				OperationID: op.ID,
				Effect:      op.Effect,
				Declared:    op.Declared,
				Label:       op.Label + " · " + br.res.Label,
			})
		}
	}
	if secrets == nil {
		secrets = []Secret{}
	}
	return &View{
		SchemaVersion:    SchemaVersion,
		Snapshot:         NewSnapshot(bd.cfg, bd.b, bd.opts),
		Harness:          bd.harness(),
		Agent:            s.subject,
		Agents:           summaries(bd.subjects),
		Domains:          assembleDomains(providers, resources),
		Providers:        providers,
		Resources:        resources,
		Edges:            edges,
		Secrets:          secrets,
		Uncertainties:    bd.uncertainties(built, defaulted, len(secrets) > 0),
		SupportedActions: supportedActions(),
		Explanations:     explanations(),
	}
}

// operation resolves one action on one resource for the subject, on
// every configured target.
func (bd *builder) operation(s *subjectInfo, r *resourceInfo, a actionInfo) Operation {
	d := bd.declare(s, r, a.action)
	op := Operation{
		ID:              operationID(r.res.ID, a.action),
		Action:          a.action,
		Label:           a.label,
		Declared:        d.declared,
		Runtime:         "not-observed",
		Grants:          d.grants,
		Conditions:      bd.conditions(r, a.action),
		AlternateRoutes: []string{},
		RouteCoverage:   RouteCoverage{Checked: []RouteStatus{}, NotModeled: []string{}},
		Enforcement:     make([]Enforcement, 0, len(bd.cfg.Targets)),
	}
	if d.win != nil {
		decided := d.win.grant
		op.Decided = &decided
	}
	var harness effective
	for _, t := range bd.cfg.Targets {
		eff := bd.effectiveOn(t, s, a.action, &d)
		if t == bd.target {
			harness = eff
		}
		op.Enforcement = append(op.Enforcement, Enforcement{Target: t, Label: TargetLabel(t), Status: eff.status, Effect: eff.effect, Native: eff.native, Note: eff.note})
		if eff.status == statusNotExpressed && d.declared != EffectDefault {
			bd.notExpressed[t] = append(bd.notExpressed[t], op.ID)
		}
		if eff.resolution == ResolutionTranslatedUnverified {
			bd.unverified[t] = append(bd.unverified[t], op.ID)
		}
	}
	op.Effect, op.Resolution, op.Native = harness.effect, harness.resolution, harness.native
	var file string
	op.Installed, file = bd.installedFor(harness, s)
	op.Hypothetical = hypothetical(op.Installed, op.Effect)
	op.Explanation = bd.explanation(&d, harness, op.Installed, file, op.Hypothetical)
	return op
}

func (bd *builder) conditions(r *resourceInfo, action string) []string {
	out := []string{}
	if r.root != nil {
		out = append(out, "The rule names the server, not this root; the same decision applies to every root.")
	}
	return append(out, bd.narrowerConditions(r, action)...)
}

// installedFor returns the installed state of the generated file that
// carries the harness decision, and that file when it is known.
func (bd *builder) installedFor(eff effective, s *subjectInfo) (string, string) {
	if !eff.installs {
		return installedNotApplied, ""
	}
	state, ok := bd.opts.Installed[bd.target]
	if !ok {
		bd.installedUnchecked = true
		if eff.agentFile || eff.mcpFile {
			return InstalledNotChecked, ""
		}
		return InstalledNotChecked, eff.carrier
	}
	file := eff.carrier
	switch {
	case eff.agentFile:
		file = agentFile(state.Files, s.id())
	case eff.mcpFile:
		file = mcpConfigFile(state.Files)
	}
	for _, f := range state.Files {
		if file != "" && f.Path == file {
			return f.State, file
		}
	}
	return state.State, ""
}

// agentFile finds the generated file of the named agent.
func agentFile(files []InstalledFile, name string) string {
	var fallback string
	for _, f := range files {
		base := path.Base(f.Path)
		if strings.TrimSuffix(base, path.Ext(base)) != name {
			continue
		}
		if strings.Contains(f.Path, "agent") || strings.Contains(f.Path, "droid") {
			return f.Path
		}
		if fallback == "" {
			fallback = f.Path
		}
	}
	return fallback
}

// mcpConfigFile finds the generated MCP config among the files.
func mcpConfigFile(files []InstalledFile) string {
	for _, f := range files {
		if strings.Contains(strings.ToLower(f.Path), "mcp") {
			return f.Path
		}
	}
	return ""
}

// hypothetical reports an effect that holds only once the generated
// file is in sync.
func hypothetical(installed string, effect Effect) bool {
	switch installed {
	case InstalledStale, InstalledNotGenerated, InstalledNotChecked:
	default:
		return false
	}
	_, known := conditionalPhrases[effect]
	return known
}

// harness describes the selected coding tool.
func (bd *builder) harness() Harness {
	label := TargetLabel(bd.target)
	available := make([]HarnessOption, 0, len(bd.cfg.Targets))
	for _, t := range bd.cfg.Targets {
		available = append(available, HarnessOption{Target: t, Label: TargetLabel(t)})
	}
	installed, ok := bd.opts.Installed[bd.target]
	if !ok {
		installed = InstalledState{State: InstalledNotChecked}
	}
	if installed.Files == nil {
		installed.Files = []InstalledFile{}
	}
	support, note := bd.permissionSupport(bd.target)
	return Harness{
		Target:    bd.target,
		Label:     label,
		Available: available,
		Identity: ExecutionIdentity{
			Kind:        "local-process",
			Description: label + " runs as the operating-system user who starts it. MCP servers authenticate with the environment variables listed under Secrets; no account name is resolved.",
		},
		Installed:         installed,
		PermissionSupport: support,
		Note:              note,
	}
}
