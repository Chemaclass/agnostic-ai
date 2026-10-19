package overview

import (
	"cmp"
	"encoding/hex"
	"hash/fnv"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// maxCards bounds the feed. Cards past it are dropped, lowest priority
// first, and nothing stands in for them.
const maxCards = 50

var priorityLabels = map[int]string{
	PrioritySecurity: "Security",
	PriorityDrift:    "Drift",
	PriorityCoverage: "Coverage",
	PriorityHygiene:  "Hygiene",
	PriorityInfo:     "Information",
}

// securityLintCodes are the lint codes whose finding widens or weakens a
// permission: LINT009 approves more than a rule says (or a deny blocks
// nothing), and LINT038 turns a bare capability into a grant of every
// operation it names.
var securityLintCodes = map[string]bool{"LINT009": true, "LINT038": true}

// shellAnyCode names the card for a session that may run any shell
// command without asking. No check emits it as a code of its own: it
// reads the Authority view's effective effect.
const shellAnyCode = "shell-any-allowed"

// draft is a card before finishing, with the parts of the finding its
// fingerprint covers.
type draft struct {
	card                        Card
	code, path, message, target string
}

func buildFeed(in Inputs, labels labeler) []Card {
	where := newPlacer(in.Bundle)
	var drafts []draft
	var drift []Finding
	for _, f := range in.Findings {
		switch f.Source {
		case SourceLint:
			drafts = append(drafts, lintCard(f, where))
		case SourceDoctor:
			drafts = append(drafts, doctorCard(f, where))
		case SourceDrift:
			drift = append(drift, f)
		}
	}
	drafts = append(drafts, driftCards(drift, labels)...)
	if in.Authority != nil {
		drafts = append(drafts, authorityCards(in.Authority, labels)...)
	}
	return finishCards(drafts, in.Snapshot.Revision)
}

// finishCards fingerprints each draft, drops repeats, appends the
// actions the client keeps itself, sorts by priority then title, and
// caps the feed.
func finishCards(drafts []draft, revision string) []Card {
	seen := map[string]bool{}
	cards := make([]Card, 0, len(drafts))
	for _, d := range drafts {
		c := d.card
		c.Fingerprint = fingerprint(c.Source, d.code, d.path, d.message, d.target)
		c.ID = c.Source + ":" + c.Fingerprint
		if seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		c.PriorityLabel = priorityLabels[c.Priority]
		c.Revision = revision
		if c.Evidence == nil {
			c.Evidence = []CardEvidence{}
		}
		c.Actions = append(slices.Clone(c.Actions),
			CardAction{ID: "dismiss", Kind: ActionDismiss, Label: "Dismiss"},
			CardAction{ID: "snooze", Kind: ActionSnooze, Label: "Snooze"},
		)
		cards = append(cards, c)
	}
	slices.SortStableFunc(cards, func(a, b Card) int {
		return cmp.Or(cmp.Compare(a.Priority, b.Priority), strings.Compare(a.Title, b.Title), strings.Compare(a.ID, b.ID))
	})
	if len(cards) > maxCards {
		cards = cards[:maxCards]
	}
	return cards
}

// fingerprint is the first 12 hex digits of a 64-bit FNV-1a hash over
// the parts that identify a finding. The revision is not one of them, so
// the same finding keeps its fingerprint while unrelated specs change.
// The fingerprint only identifies a card, so a repeat is dropped and a
// dismissal outlives a rebuild; it protects nothing, which is why it is
// not a cryptographic digest.
func fingerprint(source, code, path, message, target string) string {
	h := fnv.New64a()
	h.Write([]byte(strings.Join([]string{source, code, path, message, target}, "|")))
	return hex.EncodeToString(h.Sum(nil))[:12]
}

func lintCard(f Finding, where placer) draft {
	priority := PriorityHygiene
	if securityLintCodes[f.Code] {
		priority = PrioritySecurity
	}
	section := where.section(f)
	var actions []CardAction
	if f.Code != "" {
		actions = append(actions, CardAction{ID: "explain", Kind: ActionCopyCommand, Label: "Copy the explain command", Command: "agnostic-ai explain " + f.Code})
	}
	actions = append(actions, openSection(section))
	consequence := f.Explanation
	if consequence == "" {
		consequence = "This is a lint warning: lint fails on it only with --strict."
		if f.Severity == "error" {
			consequence = "This is a lint error: lint and doctor fail until it is fixed."
		}
	}
	return draft{
		card: Card{
			Kind:        CardInsight,
			Priority:    priority,
			Title:       firstSentence(f.Message),
			Consequence: consequence,
			SectionID:   section,
			Source:      SourceLint,
			Code:        f.Code,
			Evidence:    []CardEvidence{{Path: f.Path, Line: f.Line, Detail: f.Message}},
			Actions:     actions,
		},
		code: f.Code, path: f.Path, message: cmp.Or(f.Identity, f.Message), target: f.Target,
	}
}

func doctorCard(f Finding, where placer) draft {
	section := where.section(f)
	var actions []CardAction
	if f.Fix != "" {
		actions = append(actions, CardAction{ID: "fix", Kind: ActionCopyCommand, Label: "Copy the fix command", Command: "agnostic-ai doctor --fix"})
	}
	actions = append(actions, openSection(section))
	consequence := f.Explanation
	if consequence == "" {
		consequence = "Doctor reports this as a problem in the project setup."
	}
	return draft{
		card: Card{
			Kind:        CardInsight,
			Priority:    PriorityHygiene,
			Title:       firstSentence(f.Message),
			Consequence: consequence,
			SectionID:   section,
			Source:      SourceDoctor,
			Code:        f.Code,
			Evidence:    []CardEvidence{{Path: f.Path, Line: f.Line, Detail: f.Message}},
			Actions:     actions,
		},
		code: f.Code, path: f.Path, message: cmp.Or(f.Identity, f.Message), target: f.Target,
	}
}

// driftCards turns drift findings into one decision card per target
// that has at least one missing or stale generated file.
func driftCards(findings []Finding, labels labeler) []draft {
	byTarget := map[string][]Finding{}
	var order []string
	for _, f := range findings {
		if f.Target == "" {
			continue
		}
		if _, ok := byTarget[f.Target]; !ok {
			order = append(order, f.Target)
		}
		byTarget[f.Target] = append(byTarget[f.Target], f)
	}
	out := make([]draft, 0, len(order))
	for _, target := range order {
		files := byTarget[target]
		slices.SortStableFunc(files, func(a, b Finding) int { return strings.Compare(a.Path, b.Path) })
		files = slices.CompactFunc(files, func(a, b Finding) bool { return a.Path == b.Path })
		evidence := make([]CardEvidence, 0, len(files))
		paths := make([]string, 0, len(files))
		states := make([]string, 0, len(files))
		for _, f := range files {
			evidence = append(evidence, CardEvidence{Path: f.Path, Line: f.Line, Detail: f.Message})
			paths = append(paths, f.Path)
			states = append(states, f.Message)
		}
		tool := labels.of(target)
		out = append(out, draft{
			card: Card{
				Kind:        CardDecision,
				Priority:    PriorityDrift,
				Title:       tool + " runs on stale generated configuration",
				Consequence: "The files sync would write for " + tool + " differ from what is on disk, so the tool is not using the current specs.",
				SectionID:   SectionControl,
				Source:      SourceDrift,
				Evidence:    evidence,
				Actions: []CardAction{
					{ID: "sync", Kind: ActionCopyCommand, Label: "Copy the sync command", Command: "agnostic-ai sync -t " + target},
					openSection(SectionControl),
				},
			},
			path: strings.Join(paths, ","), message: strings.Join(states, ","), target: target,
		})
	}
	return out
}

// authorityCards reads the Authority view's own uncertainties and
// effects. It never resolves a permission itself: every id, effect, and
// grant comes from the view.
func authorityCards(v *authority.View, labels labeler) []draft {
	ix := newAuthorityIndex(v)
	var out []draft
	for _, u := range v.Uncertainties {
		switch u.Code {
		case authority.UncertaintyTargetNotExpressed:
			out = append(out, notExpressedCard(ix, u, labels))
		case authority.UncertaintyScopeUndeclared:
			out = append(out, scopeUndeclaredCard(ix, u))
		}
	}
	if d, ok := shellAnyCard(ix, labels); ok {
		out = append(out, d)
	}
	return out
}

func notExpressedCard(ix authorityIndex, u authority.Uncertainty, labels labeler) draft {
	// The Authority view names the tool in the uncertainty id, as
	// "target-not-expressed:<target>"; the related ids are operations.
	target, ok := strings.CutPrefix(u.ID, u.Code+":")
	if !ok || target == "" {
		target = ix.targetFor(u.RelatedIDs)
	}
	tool := cmp.Or(labels.of(target), "A coding tool")
	decisions := 0
	for _, id := range u.RelatedIDs {
		if strings.Contains(id, ":") {
			decisions++
		}
	}
	consequence := u.Message
	if consequence == "" {
		consequence = "Sync writes nothing on " + tool + " for these rules, so the tool's own defaults apply to them."
	}
	evidence := ix.relatedEvidence(u.RelatedIDs)
	if len(evidence) == 0 {
		evidence = []CardEvidence{{Detail: u.Message}}
	}
	return draft{
		card: Card{
			Kind:        CardInsight,
			Priority:    PriorityCoverage,
			Title:       tool + " receives no rule for " + plural(decisions, "declared decision", "declared decisions"),
			Consequence: consequence,
			SectionID:   SectionAuthority,
			DomainID:    ix.domainFor(u.RelatedIDs),
			Source:      SourceAuthority,
			Code:        u.Code,
			Evidence:    evidence,
			Actions:     []CardAction{{ID: "open", Kind: ActionOpen, Label: "Open " + tool + " in Authority", Href: authorityHref(url.Values{"target": {target}})}},
		},
		code: u.Code, path: sortedJoin(u.RelatedIDs), message: u.Message, target: target,
	}
}

func scopeUndeclaredCard(ix authorityIndex, u authority.Uncertainty) draft {
	provider, _ := ix.providerFor(u.RelatedIDs)
	name := cmp.Or(provider.Label, provider.ID, firstOf(u.RelatedIDs), "An MCP server")
	target := ix.view.Harness.Target
	query := url.Values{"target": {target}}
	if resource := ix.undeclaredResource(provider, u.RelatedIDs); resource != "" {
		query.Set("resource", resource)
	} else if provider.ID != "" {
		query.Set("provider", provider.ID)
	}
	domain := ix.domainFor(u.RelatedIDs)
	if domain == "" && len(provider.DomainIDs) > 0 {
		domain = provider.DomainIDs[0]
	}
	evidence := CardEvidence{Detail: u.Message}
	if provider.Source != nil {
		evidence.Path, evidence.Line = provider.Source.Path, provider.Source.Line
	}
	consequence := u.Message
	if consequence == "" {
		consequence = name + " names no roots, so the boundaries it works within are not declared and its credential's reach is not inspected."
	}
	return draft{
		card: Card{
			Kind:        CardInsight,
			Priority:    PriorityInfo,
			Title:       name + " declares no scope",
			Consequence: consequence,
			SectionID:   SectionAuthority,
			DomainID:    domain,
			Source:      SourceAuthority,
			Code:        u.Code,
			Evidence:    []CardEvidence{evidence},
			Actions:     []CardAction{{ID: "open", Kind: ActionOpen, Label: "Open " + name + " in Authority", Href: authorityHref(query)}},
		},
		code: u.Code, path: sortedJoin(u.RelatedIDs), message: u.Message, target: target,
	}
}

// shellAnyCard returns a decision card when the session subject's run
// operation on the shell's any-command resource is effectively allowed
// on the view's harness. The proposal removes the allow rule the view
// names as the winner; nothing is resolved here.
func shellAnyCard(ix authorityIndex, labels labeler) (draft, bool) {
	v := ix.view
	if v.Agent.Kind != authority.SubjectSession && v.Agent.ID != authority.SessionSubjectID {
		return draft{}, false
	}
	for _, r := range v.Resources {
		if r.Kind != authority.ResourceCommandPattern {
			continue
		}
		for _, op := range r.Operations {
			if op.Effect != authority.EffectAllowed || !anyCommand(r, op) {
				continue
			}
			return shellAnyDraft(v, r, op, labels), true
		}
	}
	return draft{}, false
}

// anyCommand reports whether a command-pattern resource or its operation
// stands for every shell command, by the label or pattern the view gave.
func anyCommand(r authority.Resource, op authority.Operation) bool {
	return strings.EqualFold(strings.TrimSpace(r.Label), "Any command") ||
		strings.EqualFold(strings.TrimSpace(op.Label), "Any command") ||
		r.Identity.Pattern == "*"
}

func shellAnyDraft(v *authority.View, r authority.Resource, op authority.Operation, labels labeler) draft {
	target := v.Harness.Target
	tool := labels.of(target)
	var actions []CardAction
	evidence := []CardEvidence{}
	g, ok := winningAllow(op)
	switch {
	case ok && g.Redacted:
		evidence = append(evidence, CardEvidence{Path: g.Source.Path, Line: g.Source.Line, Detail: "permissions.allow: " + g.Rule + " (holds a credential; edit the file directly)"})
	case ok:
		evidence = append(evidence, CardEvidence{Path: g.Source.Path, Line: g.Source.Line, Detail: "permissions.allow: " + g.Rule})
		actions = append(actions, CardAction{
			ID:    "propose",
			Kind:  ActionPropose,
			Label: "Propose removing allow rule " + g.Rule,
			Proposal: &authority.Proposal{
				AgentID:  v.Agent.ID,
				Target:   target,
				Revision: v.Snapshot.Revision,
				Changes:  []authority.Change{{Op: "remove", List: "allow", Rule: g.Rule, Source: g.Source.Path}},
			},
		})
	default:
		evidence = append(evidence, CardEvidence{Detail: cmp.Or(op.Explanation, "The view resolves this operation as allowed.")})
	}
	query := url.Values{"resource": {r.ID}, "operation": {op.ID}, "target": {target}}
	actions = append(actions, CardAction{ID: "open", Kind: ActionOpen, Label: "Open in Authority", Href: authorityHref(query)})
	var domain string
	if len(r.DomainIDs) > 0 {
		domain = r.DomainIDs[0]
	}
	// The card states what the permission rules allow, never that a
	// command ran, and stays conditional while the generated file is
	// stale, missing, or unchecked.
	title := "Any shell command is permitted without asking"
	consequence := cmp.Or(tool, "The coding tool") + "'s permission rules allow any shell command without asking first; only a narrower ask or deny rule stops one."
	if op.Hypothetical {
		title = "Any shell command would be permitted without asking once synced"
		consequence = "Once the generated permission file is synced (it is " + cmp.Or(op.Installed, "not checked") + "), " + cmp.Or(tool, "the coding tool") + " would allow any shell command without asking first; only a narrower ask or deny rule stops one."
	}
	return draft{
		card: Card{
			Kind:        CardDecision,
			Priority:    PrioritySecurity,
			Title:       title,
			Consequence: consequence,
			SectionID:   SectionAuthority,
			DomainID:    domain,
			Source:      SourceAuthority,
			Code:        shellAnyCode,
			Evidence:    evidence,
			Actions:     actions,
		},
		code: shellAnyCode, path: r.ID, message: op.ID, target: target,
	}
}

// winningAllow returns the project allow rule the view says decided op.
func winningAllow(op authority.Operation) (authority.Grant, bool) {
	if g := op.Decided; g != nil && g.List == "allow" && g.Scope != "agent" {
		return *g, true
	}
	for _, g := range op.Grants {
		if g.Wins && g.List == "allow" && g.Scope != "agent" {
			return g, true
		}
	}
	return authority.Grant{}, false
}

// authorityIndex looks up the view's ids.
type authorityIndex struct {
	view       *authority.View
	targets    map[string]bool
	domains    map[string]bool
	providers  map[string]authority.Provider
	resources  map[string]authority.Resource
	operations map[string]operationRef
}

type operationRef struct {
	resource  authority.Resource
	operation authority.Operation
}

func newAuthorityIndex(v *authority.View) authorityIndex {
	ix := authorityIndex{
		view:       v,
		targets:    map[string]bool{},
		domains:    map[string]bool{},
		providers:  map[string]authority.Provider{},
		resources:  map[string]authority.Resource{},
		operations: map[string]operationRef{},
	}
	for _, t := range v.Snapshot.Targets {
		ix.targets[t] = true
	}
	for _, o := range v.Harness.Available {
		ix.targets[o.Target] = true
	}
	if v.Harness.Target != "" {
		ix.targets[v.Harness.Target] = true
	}
	for _, d := range []string{
		authority.DomainSourceControl, authority.DomainCloud, authority.DomainData, authority.DomainFilesystem,
		authority.DomainWeb, authority.DomainCommunication, authority.DomainSecrets, authority.DomainAgentOps,
	} {
		ix.domains[d] = true
	}
	for _, d := range v.Domains {
		ix.domains[d.ID] = true
	}
	for _, p := range v.Providers {
		ix.providers[p.ID] = p
	}
	for _, r := range v.Resources {
		ix.resources[r.ID] = r
		for _, op := range r.Operations {
			ix.operations[op.ID] = operationRef{resource: r, operation: op}
		}
	}
	return ix
}

// targetFor returns the first related id that names a target, or the
// view's harness.
func (ix authorityIndex) targetFor(ids []string) string {
	for _, id := range ids {
		if name := strings.TrimPrefix(id, "target:"); ix.targets[name] {
			return name
		}
	}
	return ix.view.Harness.Target
}

func (ix authorityIndex) domainFor(ids []string) string {
	for _, id := range ids {
		if name := strings.TrimPrefix(id, "domain:"); ix.domains[name] {
			return name
		}
	}
	return ""
}

func (ix authorityIndex) providerFor(ids []string) (authority.Provider, bool) {
	for _, id := range ids {
		if p, ok := ix.providers[id]; ok {
			return p, true
		}
		if p, ok := ix.providers[strings.TrimPrefix(id, "provider:")]; ok {
			return p, true
		}
		if r, ok := ix.resources[id]; ok {
			if p, ok := ix.providers[r.ProviderID]; ok {
				return p, true
			}
		}
	}
	return authority.Provider{}, false
}

// undeclaredResource returns the resource that stands for the provider's
// undeclared scope, or a related resource id.
func (ix authorityIndex) undeclaredResource(p authority.Provider, ids []string) string {
	if p.ID != "" {
		for _, r := range ix.view.Resources {
			if r.ProviderID == p.ID && r.Kind == authority.ResourceUndeclaredScope {
				return r.ID
			}
		}
	}
	for _, id := range ids {
		if _, ok := ix.resources[id]; ok {
			return id
		}
	}
	return ""
}

// relatedEvidence lists the grant that decided each related operation,
// or each operation of a related resource.
func (ix authorityIndex) relatedEvidence(ids []string) []CardEvidence {
	var out []CardEvidence
	seen := map[CardEvidence]bool{}
	add := func(r authority.Resource, op authority.Operation) {
		g := op.Decided
		if g == nil && len(op.Grants) > 0 {
			g = &op.Grants[0]
		}
		if g == nil {
			return
		}
		e := CardEvidence{Path: g.Source.Path, Line: g.Source.Line, Detail: g.List + " rule " + g.Rule + " on " + cmp.Or(r.Label, r.ID)}
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	for _, id := range ids {
		if ref, ok := ix.operations[id]; ok {
			add(ref.resource, ref.operation)
			continue
		}
		if r, ok := ix.resources[id]; ok {
			for _, op := range r.Operations {
				add(r, op)
			}
		}
	}
	return out
}

func authorityHref(query url.Values) string {
	for key, values := range query {
		if len(values) == 0 || values[0] == "" {
			query.Del(key)
		}
	}
	if len(query) == 0 {
		return "/authority"
	}
	return "/authority?" + query.Encode()
}

// sectionHref is where a card's open action leads: a section's deep view
// when it has one, else the overview focused on that section.
func sectionHref(id string) string {
	for _, def := range sectionDefs {
		if def.id == id && def.deepView != "" {
			return def.deepView
		}
	}
	return "/?" + url.Values{"section": {id}}.Encode()
}

func openSection(id string) CardAction {
	label := id
	for _, def := range sectionDefs {
		if def.id == id {
			label = def.label
		}
	}
	return CardAction{ID: "open", Kind: ActionOpen, Label: "Open " + label, Href: sectionHref(id)}
}

// firstSentence returns the message up to its first sentence break: a
// period, question mark, or exclamation mark followed by a space, or the
// semicolon that lint messages put before their remedy.
func firstSentence(message string) string {
	message = strings.TrimSpace(message)
	for i := 0; i+1 < len(message); i++ {
		switch message[i] {
		case '.', '?', '!', ';':
			if message[i+1] == ' ' {
				return message[:i]
			}
		}
	}
	return strings.TrimSuffix(message, ".")
}

func sortedJoin(ids []string) string {
	return strings.Join(slices.Sorted(slices.Values(ids)), ",")
}

func firstOf(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

// placer decides which section a lint or doctor finding belongs to.
type placer struct {
	entries map[string]spec.Entry
}

func newPlacer(b spec.Bundle) placer {
	p := placer{entries: map[string]spec.Entry{}}
	for _, e := range b.All() {
		key := cleanPath(e.Path)
		if _, taken := p.entries[key]; !taken && key != "" {
			p.entries[key] = e
		}
	}
	return p
}

var kindSections = map[spec.Kind]string{
	spec.KindRule:        SectionContext,
	spec.KindSkill:       SectionContext,
	spec.KindIgnore:      SectionContext,
	spec.KindAgent:       SectionControl,
	spec.KindHook:        SectionControl,
	spec.KindCommand:     SectionControl,
	spec.KindMCP:         SectionTools,
	spec.KindEnvironment: SectionTools,
	spec.KindReview:      SectionVerification,
}

// section places a finding by the section its check named, else by the
// spec at its path, else by its path alone. The config file shapes how
// sync runs, so it falls under control; instructions and anything else
// fall back to context, since they shape what the agent reads.
func (p placer) section(f Finding) string {
	if slices.ContainsFunc(sectionDefs, func(d sectionDef) bool { return d.id == f.Section }) {
		return f.Section
	}
	permission := securityLintCodes[f.Code] || strings.HasPrefix(f.Message, "permissions.") || strings.HasPrefix(f.Message, "protected path")
	path := cleanPath(f.Path)
	if e, ok := p.entries[path]; ok {
		if e.Kind != spec.KindSettings {
			return cmp.Or(kindSections[e.Kind], SectionContext)
		}
		if permission || (grantsAuthority(e) && !hasField(e, "model") && !hasField(e, "effort")) {
			return SectionAuthority
		}
		return SectionControl
	}
	for _, dir := range []string{config.SourceBaseDir + "/memory/", config.SourceBaseDir + "/local/memory/"} {
		if strings.HasPrefix(path, dir) {
			return SectionMemory
		}
	}
	switch {
	case permission:
		return SectionAuthority
	case slices.Contains([]string{config.ConfigFileName, config.LocalOverrideFileName, config.LegacyConfigFileName}, path):
		return SectionControl
	}
	return SectionContext
}

func cleanPath(p string) string {
	if p == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(p))
}

// plural counts n with the singular or plural noun.
func plural(n int, singular, pluralNoun string) string {
	if n == 1 {
		return "1 " + singular
	}
	return strconv.Itoa(n) + " " + pluralNoun
}
