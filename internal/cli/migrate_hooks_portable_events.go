package cli

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// hooksPortableEventsMigration rewrites a hook's native event: and
// matcher: as on: and match: when that form translates back to the same
// event and matcher on every target the hook reaches, so sync writes the
// same files after it.
var hooksPortableEventsMigration = specMigration{
	ID:      "hooks-portable-events",
	Group:   "hooks",
	Release: "0.79.0",
	Summary: "rewrite a hook's event: and matcher: as the portable on: and match:",
	Plan:    planHooksPortableEvents,
	Note:    portableHookTargetsNote,
}

// hookMigrationSpecs is what the hooks migration reads in a scope: the
// hooks, the layers they come from, and the targets that run them.
type hookMigrationSpecs struct {
	bundle  spec.Bundle
	layers  []spec.Layer
	targets []string
}

// loadHookMigrationSpecs loads the project, or the global layers and the
// targets a default sync --global writes hooks for.
func loadHookMigrationSpecs(s migrationScope) (hookMigrationSpecs, error) {
	if !s.global {
		cfg, b, err := s.loadProject()
		if err != nil {
			return hookMigrationSpecs{}, err
		}
		return projectHookMigrationSpecs(s.root, cfg, b)
	}
	b, layers, err := s.loadGlobalSpecs()
	if err != nil {
		return hookMigrationSpecs{}, err
	}
	targets, err := loadGlobalTargets(s.root, io.Discard)
	if err != nil {
		return hookMigrationSpecs{}, err
	}
	if targets == nil {
		targets = globalTargetNames()
	}
	return hookMigrationSpecs{bundle: b, layers: layers, targets: globalHookTargets(targets)}, nil
}

// projectHookMigrationSpecs is the project at root, loaded as cfg and b.
// An adapter outside the tree may run hooks, so it counts.
func projectHookMigrationSpecs(root string, cfg *config.Config, b spec.Bundle) (hookMigrationSpecs, error) {
	var targets []string
	for _, t := range cfg.Targets {
		if _, inTree := adapters.Get(t); inTree {
			if _, runs := targetsSupportingKind[spec.KindHook][t]; !runs {
				continue
			}
		}
		targets = append(targets, t)
	}
	layers, err := resolveLayers(root, cfg)
	if err != nil {
		return hookMigrationSpecs{}, err
	}
	return hookMigrationSpecs{bundle: withoutBuiltinEntries(b), layers: withoutBuiltinLayers(layers), targets: targets}, nil
}

// reach lists the targets h reaches that run hooks.
func (hs hookMigrationSpecs) reach(h spec.Entry) []string {
	var out []string
	for _, t := range hs.targets {
		if h.EmitsTo(t) {
			out = append(out, t)
		}
	}
	return out
}

// portableHookTargetsNote names the targets that run hooks but skip a
// portable one until their mapping lands.
func portableHookTargetsNote(s migrationScope) string {
	reach := andList(spec.PortableHookTargets())
	var missing []string
	if hs, err := loadHookMigrationSpecs(s); err == nil {
		for _, t := range hs.targets {
			if _, inTree := adapters.Get(t); inTree && !spec.TranslatesPortableHooks(t) {
				missing = append(missing, t)
			}
		}
	}
	switch len(missing) {
	case 0:
		return "portable hooks reach " + reach + " today; other targets skip them until their mapping lands"
	case 1:
		return "portable hooks reach " + reach + " today; " + missing[0] + " skips them until its mapping lands"
	}
	return "portable hooks reach " + reach + " today; " + andList(missing) + " skip them until their mapping lands"
}

// andList joins names as "a, b, and c".
func andList(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	}
	return strings.Join(names[:len(names)-1], ", ") + ", and " + names[len(names)-1]
}

func planHooksPortableEvents(s migrationScope) ([]migrationChange, []migrationSkip, error) {
	hs, err := loadHookMigrationSpecs(s)
	if err != nil {
		return nil, nil, err
	}
	planned, skips, err := planPortableHooks(s, hs)
	if err != nil {
		return nil, nil, err
	}
	var changes []migrationChange
	for _, p := range planned {
		changes = append(changes, p.change)
	}
	return changes, skips, nil
}

// plannedPortableHook is one hook spec hooks-portable-events rewrites.
type plannedPortableHook struct {
	hook   spec.Entry
	form   portableHookForm
	change migrationChange
}

// planPortableHooks plans the hooks-portable-events rewrites for scope s,
// loaded as hs. LINT034 reads the same plan, so lint suggests the
// portable form exactly where migrate writes it; that includes the spec
// roots check the registry repeats for every migration.
func planPortableHooks(s migrationScope, hs hookMigrationSpecs) ([]plannedPortableHook, []migrationSkip, error) {
	extended, err := s.extendedSpecNames(hs.layers, func(lb spec.Bundle) []spec.Entry { return lb.Hooks })
	if err != nil {
		return nil, nil, err
	}
	roots, packs := s.specRoots()
	var planned []plannedPortableHook
	var skips []migrationSkip
	for _, h := range hs.bundle.Hooks {
		event, ok := h.Meta["event"].(string)
		if !ok || event == "" {
			continue
		}
		skip := func(reason string) { skips = append(skips, migrationSkip{Path: h.Path, Reason: reason}) }
		if spec.IsPortableHook(h.Meta) {
			skips = append(skips, migrationSkip{Path: h.Path, Reason: "sets both the native and the portable form; keep one by hand", Actionable: true})
			continue
		}
		if pack, ok := strings.CutPrefix(h.Layer, layerNamePackPrefix); ok {
			skips = append(skips, packSkip(h.Path, pack))
			continue
		}
		if extended[h.Name] {
			skips = append(skips, migrationSkip{Path: h.Path, Reason: "a local/ spec extends this hook; rewrite both files by hand", Actionable: true})
			continue
		}
		form, reason := portableFormOf(h, hs.reach(h))
		if reason != "" {
			skip(reason)
			continue
		}
		if outside, ok := s.outsideSpecRoots(migrationChange{Path: h.Path}, roots, packs); ok {
			skips = append(skips, outside)
			continue
		}
		body, err := os.ReadFile(h.Path)
		if err != nil {
			return nil, nil, err
		}
		after, err := rewriteTopLevelYAMLKeys(string(body), form.rewrites())
		if err != nil {
			skip("cannot rewrite in place: " + err.Error())
			continue
		}
		planned = append(planned, plannedPortableHook{hook: h, form: form, change: migrationChange{Path: h.Path, Before: string(body), After: after}})
	}
	return planned, skips, nil
}

// portableHookForm is a native hook's event and matcher, with the on:
// and match: that give them back on every target the hook reaches.
type portableHookForm struct {
	event, matcher string
	hasMatcher     bool
	on, match      string
}

// portableFormOf returns the portable form of native hook h, which
// reaches targets, or why no portable form gives each of them the same
// event and matcher. The migration and import both apply it.
func portableFormOf(h spec.Entry, targets []string) (portableHookForm, string) {
	event, _ := h.Meta["event"].(string)
	rawMatcher, hasMatcher := h.Meta["matcher"]
	matcher, ok := rawMatcher.(string)
	if hasMatcher && !ok {
		return portableHookForm{}, "matcher: is not a string"
	}
	if len(targets) == 0 {
		return portableHookForm{}, "reaches no configured target that runs hooks"
	}
	on, match, blocker := spec.PortableHookForm(targets, event, matcher, hasMatcher)
	if blocker != "" {
		native := event
		if hasMatcher {
			native = fmt.Sprintf("%s with matcher %q", event, matcher)
		}
		return portableHookForm{}, fmt.Sprintf("no portable form gives %s on %s", native, blocker) + widerEditNote(blocker, matcher)
	}
	return portableHookForm{event: event, matcher: matcher, hasMatcher: hasMatcher, on: on, match: match}, ""
}

// rewrites renames event: and matcher: to on: and match: in place.
func (f portableHookForm) rewrites() []yamlKeyRewrite {
	out := []yamlKeyRewrite{{Key: "event", NewKey: "on", Value: f.on}}
	if f.hasMatcher {
		out = append(out, yamlKeyRewrite{Key: "matcher", NewKey: "match", Value: f.match})
	}
	return out
}

// extendedSpecNames names the specs of one kind, which kind picks from
// a layer, that a local/ spec merges into a lower layer's spec of the
// same name. Their fields come from two files.
func (s migrationScope) extendedSpecNames(layers []spec.Layer, kind func(spec.Bundle) []spec.Entry) (map[string]bool, error) {
	seen := map[string]bool{}
	extended := map[string]bool{}
	for _, layer := range layers {
		lb, err := s.loadLayer(layer)
		if err != nil {
			return nil, err
		}
		for _, h := range kind(lb) {
			if layer.Extends && seen[h.Name] {
				extended[h.Name] = true
			}
			seen[h.Name] = true
		}
	}
	return extended, nil
}

// hookMigrationTargets lists the configured targets h reaches that run
// hooks. An adapter outside the tree may run them, so it counts.
func hookMigrationTargets(cfg *config.Config, h spec.Entry) []string {
	var out []string
	for _, t := range cfg.Targets {
		if !h.EmitsTo(t) {
			continue
		}
		if _, inTree := adapters.Get(t); inTree {
			if _, runs := targetsSupportingKind[spec.KindHook][t]; !runs {
				continue
			}
		}
		out = append(out, t)
	}
	return out
}

// widerEditNote explains a matcher that names some of target's edit
// tools: match: edit would also run on the rest.
func widerEditNote(target, matcher string) string {
	edit, ok := spec.HookToolMatcher(target, "edit")
	if !ok || matcher == "" || matcher == edit {
		return ""
	}
	names := strings.Split(edit, "|")
	var extra []string
	for _, name := range strings.Split(matcher, "|") {
		if !slices.Contains(names, name) {
			return ""
		}
	}
	for _, name := range names {
		if !slices.Contains(strings.Split(matcher, "|"), name) {
			extra = append(extra, name)
		}
	}
	return fmt.Sprintf("; match: edit there also covers %s", strings.Join(extra, " and "))
}
