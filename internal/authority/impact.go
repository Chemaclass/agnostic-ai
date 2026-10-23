package authority

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/preview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// effectRank orders the decisions whose strength is known. A default or
// unresolved effect has no place in it.
var effectRank = map[Effect]int{
	EffectAllowed:     2,
	EffectAsk:         1,
	EffectDenied:      0,
	EffectUnavailable: 0,
}

// Impact applies a proposal to a copy of the specs and reports every
// operation whose declared or effective effect changes, for every
// subject on the target. Nothing is written.
func Impact(cfg *config.Config, b spec.Bundle, opts Options, p Proposal) (*ImpactResult, error) {
	if cfg == nil {
		return nil, errors.New("authority impact: no configuration loaded")
	}
	requested := p.Target
	if requested == "" {
		requested = opts.Target
	}
	target, err := resolveTarget(cfg, requested)
	if err != nil {
		return nil, err
	}
	paths := newPaths(opts.Root)
	subjects := parseSubjects(b.Agents, paths.source)
	selected, err := findSubject(subjects, p.AgentID)
	if err != nil {
		return nil, err
	}
	res := &ImpactResult{
		SchemaVersion:    SchemaVersion,
		Status:           "proposed",
		Target:           target,
		BaseRevision:     Revision(b, opts),
		Problems:         []string{},
		Changes:          []EffectChange{},
		AffectedSubjects: []string{},
		Diff:             []FileDiff{},
	}
	if p.Revision != "" && p.Revision != res.BaseRevision {
		res.Problems = append(res.Problems, fmt.Sprintf("The proposal was made against revision %s, but the configuration is now at %s; reload the view and propose again.", p.Revision, res.BaseRevision))
		return redactImpact(res), nil
	}
	after, diff, problems := applyProposal(cfg, b, opts, paths, p.Changes)
	if len(problems) > 0 {
		res.Problems = problems
		return redactImpact(res), nil
	}
	res.Valid = true
	res.Diff = diff
	res.ProposedRevision = Revision(after, opts)
	proposedInstalled := proposedCarriers(opts.Installed)
	for _, s := range subjects {
		o := opts
		o.Target, o.AgentID = target, s.id()
		o.FileMatches = s == selected && opts.FileMatches
		// Both sides share the resources either side's rules name, so a
		// removed rule's resource still shows what it changes to.
		before, err := buildWith(cfg, b, o, after.Settings)
		if err != nil {
			return nil, err
		}
		po := o
		po.Installed = proposedInstalled
		proposed, err := buildWith(cfg, after, po, b.Settings)
		if err != nil {
			return nil, err
		}
		if s == selected {
			res.After = proposed
		}
		changes := compareViews(s.id(), before, proposed)
		affected := false
		for _, c := range changes {
			countDirection(&res.Summary, c.Direction)
			affected = affected || c.EffectiveBefore != c.EffectiveAfter
		}
		res.Changes = append(res.Changes, changes...)
		if affected {
			res.AffectedSubjects = append(res.AffectedSubjects, s.id())
		}
	}
	res.Summary.Changed = len(res.Changes)
	return redactImpact(res), nil
}

func buildWith(cfg *config.Config, b spec.Bundle, opts Options, extra []spec.Entry) (*View, error) {
	bd, err := newBuilder(cfg, b, opts, extra)
	if err != nil {
		return nil, err
	}
	return bd.view(), nil
}

// compareViews lists the operations whose declared or effective effect
// differs between two views of one subject.
func compareViews(subjectID string, before, after *View) []EffectChange {
	previous := map[string]Operation{}
	for _, r := range before.Resources {
		for _, op := range r.Operations {
			previous[op.ID] = op
		}
	}
	harness := after.Harness.Label
	var out []EffectChange
	for _, r := range after.Resources {
		for _, op := range r.Operations {
			prev, ok := previous[op.ID]
			if !ok || prev.Declared == op.Declared && prev.Effect == op.Effect {
				continue
			}
			direction, reason := judgeDirection(prev.Effect, op.Effect, operationName(r, op), harness)
			out = append(out, EffectChange{
				SubjectID:       subjectID,
				ResourceID:      r.ID,
				ResourceLabel:   r.Label,
				OperationID:     op.ID,
				OperationLabel:  op.Label,
				DeclaredBefore:  prev.Declared,
				DeclaredAfter:   op.Declared,
				EffectiveBefore: prev.Effect,
				EffectiveAfter:  op.Effect,
				Direction:       direction,
				Reason:          reason,
			})
		}
	}
	return out
}

// operationName names an operation the way a rule spells it, for a
// reason sentence.
func operationName(r Resource, op Operation) string {
	switch r.Kind {
	case ResourceDeclaredRoot, ResourceUndeclaredScope:
		if op.Action == anyToolAction {
			return "the other tools of " + r.Identity.Server
		}
		return "mcp:" + r.Identity.Server + "/" + op.Action
	case ResourceCommandPattern:
		if r.Identity.Pattern == "*" {
			return capShell
		}
		return "shell commands matching " + r.Label
	case ResourceFilePattern:
		return strings.ToLower(op.Label) + " on " + r.Label
	}
	return strings.ToLower(op.Label)
}

// judgeDirection ranks only known decisions. When either side is the
// tool's default or unresolved, the direction is indeterminate and the
// reason names the unknown side.
func judgeDirection(before, after Effect, operation, harness string) (string, string) {
	rb, knownBefore := effectRank[before]
	ra, knownAfter := effectRank[after]
	if !knownBefore || !knownAfter {
		var parts []string
		if !knownBefore {
			parts = append(parts, unknownSide("before", before, operation, harness))
		}
		if !knownAfter {
			parts = append(parts, unknownSide("after", after, operation, harness))
		}
		return "indeterminate", sentence(strings.Join(parts, "; ")) + "."
	}
	switch {
	case ra > rb:
		return "widened", ""
	case ra < rb:
		return "narrowed", ""
	case before != after:
		return "moved", ""
	}
	return "moved", "Only the declared decision changes; the effect on " + harness + " stays " + string(after) + "."
}

func unknownSide(side string, e Effect, operation, harness string) string {
	if e == EffectDefault {
		return side + " the change no rule applies, and the tool's default for " + operation + " is not modeled"
	}
	return side + " the change the effect of " + operation + " on " + harness + " is unresolved"
}

func countDirection(s *ImpactSummary, direction string) {
	switch direction {
	case "widened":
		s.Widened++
	case "narrowed":
		s.Narrowed++
	case "moved":
		s.Moved++
	case "indeterminate":
		s.Indeterminate++
	}
}

// applyProposal applies the changes in order to a copy of the bundle's
// settings specs and renders each touched spec before and after.
func applyProposal(cfg *config.Config, b spec.Bundle, opts Options, p paths, changes []Change) (spec.Bundle, []FileDiff, []string) {
	after := b
	after.Settings = slices.Clone(b.Settings)
	type touched struct {
		index  int
		before *spec.Entry
	}
	var order []touched
	var problems []string
	for i, c := range changes {
		n := i + 1
		if problem := changeProblem(c); problem != "" {
			problems = append(problems, fmt.Sprintf("change %d: %s", n, problem))
			continue
		}
		index, created, err := pickSettings(&after, cfg, opts, p, c.Source)
		if err != nil {
			problems = append(problems, fmt.Sprintf("change %d: %v", n, err))
			continue
		}
		if !slices.ContainsFunc(order, func(t touched) bool { return t.index == index }) {
			var original *spec.Entry
			if !created {
				e := after.Settings[index]
				original = &e
			}
			after.Settings[index] = cloneEntry(after.Settings[index])
			order = append(order, touched{index: index, before: original})
		}
		if problem := applyChange(&after.Settings[index], c, p.display(after.Settings[index])); problem != "" {
			problems = append(problems, fmt.Sprintf("change %d: %s", n, problem))
		}
	}
	if len(problems) > 0 {
		return b, nil, problems
	}
	diff := make([]FileDiff, 0, len(order))
	for _, t := range order {
		before := ""
		if t.before != nil {
			before = shownSpecYAML(*t.before)
		}
		e := after.Settings[t.index]
		diff = append(diff, FileDiff{Path: p.display(e), Before: before, After: shownSpecYAML(e)})
	}
	return after, diff, nil
}

// changeProblem returns why a change cannot apply, or "".
func changeProblem(c Change) string {
	switch {
	case c.Op != "add" && c.Op != "remove":
		return fmt.Sprintf("op %q is not add or remove", c.Op)
	case !slices.Contains(spec.PermissionLists, c.List):
		return fmt.Sprintf("list %q is not allow, ask, or deny", c.List)
	case strings.TrimSpace(c.Rule) == "":
		return "the rule is empty"
	}
	if _, problem := spec.PermissionRules(c.List, c.Rule); problem != "" {
		return problem
	}
	return ""
}

// pickSettings finds the settings spec a change edits: the one Source
// names, else the first project spec with a permissions block, else a
// new permissions.yaml under the settings source directory.
func pickSettings(b *spec.Bundle, cfg *config.Config, opts Options, p paths, source string) (int, bool, error) {
	if source != "" {
		want := strings.TrimPrefix(filepath.ToSlash(source), "./")
		for i, e := range b.Settings {
			if p.display(e) == want || filepath.ToSlash(e.Path) == want {
				return i, false, nil
			}
		}
		return 0, false, fmt.Errorf("no settings spec at %s", source)
	}
	layer := "project"
	if opts.Global {
		layer = "global"
	}
	for i, e := range b.Settings {
		if _, ok := e.Meta["permissions"].(map[string]any); ok && (e.Layer == layer || e.Layer == "") {
			return i, false, nil
		}
	}
	dir := config.SourceBaseDir + "/settings"
	switch {
	case opts.Global:
		dir = "settings"
	case cfg.Sources.Settings != "":
		dir = cfg.Sources.Settings
	}
	entry := spec.Entry{
		Kind:     spec.KindSettings,
		Name:     "permissions",
		Path:     filepath.Join(config.ResolveSourcePath(p.root, dir), "permissions.yaml"),
		Layer:    layer,
		Meta:     map[string]any{},
		MetaKeys: []string{},
	}
	for i, e := range b.Settings {
		if p.display(e) == p.display(entry) {
			return i, false, nil
		}
	}
	b.Settings = append(b.Settings, entry)
	return len(b.Settings) - 1, true, nil
}

// applyChange edits one permission list of e.
func applyChange(e *spec.Entry, c Change, path string) string {
	perms, _ := e.Meta["permissions"].(map[string]any)
	list, _ := perms[c.List].([]any)
	if c.Op == "remove" {
		kept := slices.DeleteFunc(slices.Clone(list), func(v any) bool { return v == c.Rule })
		if len(kept) == len(list) {
			return fmt.Sprintf("%s has no %s rule %q", path, c.List, c.Rule)
		}
		if len(kept) == 0 {
			delete(perms, c.List)
		} else {
			perms[c.List] = kept
		}
		if len(perms) == 0 {
			delete(e.Meta, "permissions")
		}
		return ""
	}
	if slices.Contains(list, any(c.Rule)) {
		return ""
	}
	if perms == nil {
		perms = map[string]any{}
		e.Meta["permissions"] = perms
		if !slices.Contains(e.MetaKeys, "permissions") {
			e.MetaKeys = append(e.MetaKeys, "permissions")
		}
	}
	perms[c.List] = append(list, c.Rule)
	return ""
}

// cloneEntry copies e deeply enough that editing its fields leaves the
// original untouched.
func cloneEntry(e spec.Entry) spec.Entry {
	meta, _ := deepCopy(e.Meta).(map[string]any)
	if meta == nil {
		meta = map[string]any{}
	}
	e.Meta = meta
	e.MetaKeys = slices.Clone(e.MetaKeys)
	return e
}

func deepCopy(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, item := range t {
			out[k] = deepCopy(item)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = deepCopy(item)
		}
		return out
	}
	return v
}

// proposedCarriers copies the installed evidence with each tool's
// permission file marked stale, because a proposal changes the rules
// that file carries and nothing has written it. The proposed view then
// reads as hypothetical even when the current output is in sync. A file
// that is missing or already stale keeps its state.
func proposedCarriers(installed map[string]InstalledState) map[string]InstalledState {
	if installed == nil {
		return nil
	}
	out := make(map[string]InstalledState, len(installed))
	for target, st := range installed {
		carrier := permissionFiles[target]
		files := make([]InstalledFile, 0, len(st.Files)+1)
		touched, found := false, false
		for _, f := range st.Files {
			if f.Path == carrier {
				found = true
				if f.State == InstalledInSync {
					f.State = InstalledStale
					touched = true
				}
			}
			files = append(files, f)
		}
		// A first permission rule introduces the carrier: the current
		// output never wrote it, so the proposed view must not fall back
		// to the aggregate and read the new rule as installed.
		if carrier != "" && !found {
			files = append(files, InstalledFile{Path: carrier, State: InstalledNotChecked})
			touched = true
		}
		st.Files = files
		if touched && st.State == InstalledInSync {
			st.State = InstalledStale
		}
		out[target] = st
	}
	return out
}

func shownSpecYAML(e spec.Entry) string {
	e = cloneEntry(e)
	perms, _ := e.Meta["permissions"].(map[string]any)
	for _, list := range spec.PermissionLists {
		rules, _ := perms[list].([]any)
		for i, item := range rules {
			if rule, ok := item.(string); ok {
				rules[i] = preview.RedactRule(rule)
			}
		}
	}
	return preview.Display("settings.yaml", renderSpecYAML(e)).Text
}

func redactImpact(res *ImpactResult) *ImpactResult {
	redactDisplay(res)
	for i, problem := range res.Problems {
		res.Problems[i] = preview.RedactText(problem)
	}
	for i := range res.Changes {
		res.Changes[i].Reason = preview.RedactText(res.Changes[i].Reason)
	}
	return res
}
