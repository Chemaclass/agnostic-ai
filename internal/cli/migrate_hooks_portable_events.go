package cli

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
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
	if s.global {
		if err := requireGlobalVersion(s.root, nil); err != nil {
			return hookMigrationSpecs{}, err
		}
		layers := globalLayers(s.root)
		b, err := spec.LoadLayered(layers)
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
	cfg, b, err := loadProject(s.root)
	if err != nil {
		return hookMigrationSpecs{}, err
	}
	var targets []string
	for _, t := range cfg.Targets {
		// An adapter outside the tree may run hooks, so it counts.
		if _, inTree := adapters.Get(t); inTree {
			if _, runs := targetsSupportingKind[spec.KindHook][t]; !runs {
				continue
			}
		}
		targets = append(targets, t)
	}
	return hookMigrationSpecs{bundle: b, layers: resolveLayers(s.root, cfg), targets: targets}, nil
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
	extended, err := extendedHookNames(hs.layers)
	if err != nil {
		return nil, nil, err
	}
	var changes []migrationChange
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
		rawMatcher, hasMatcher := h.Meta["matcher"]
		matcher, ok := rawMatcher.(string)
		if hasMatcher && !ok {
			skip("matcher: is not a string")
			continue
		}
		var reach []string
		for _, t := range hs.targets {
			if h.EmitsTo(t) {
				reach = append(reach, t)
			}
		}
		if len(reach) == 0 {
			skip("reaches no configured target that runs hooks")
			continue
		}
		on, match, blocker := spec.PortableHookForm(reach, event, matcher, hasMatcher)
		if blocker != "" {
			native := event
			if hasMatcher {
				native = fmt.Sprintf("%s with matcher %q", event, matcher)
			}
			skip(fmt.Sprintf("no portable form gives %s on %s", native, blocker) + widerEditNote(blocker, matcher))
			continue
		}
		body, err := os.ReadFile(h.Path)
		if err != nil {
			return nil, nil, err
		}
		rewrites := []yamlKeyRewrite{{Key: "event", NewKey: "on", Value: on}}
		if hasMatcher {
			rewrites = append(rewrites, yamlKeyRewrite{Key: "matcher", NewKey: "match", Value: match})
		}
		after, err := rewriteTopLevelYAMLKeys(string(body), rewrites)
		if err != nil {
			skip("cannot rewrite in place: " + err.Error())
			continue
		}
		changes = append(changes, migrationChange{Path: h.Path, Before: string(body), After: after})
	}
	return changes, skips, nil
}

// extendedHookNames names the hooks a local/ spec merges into a lower
// layer's spec of the same name. Their fields come from two files.
func extendedHookNames(layers []spec.Layer) (map[string]bool, error) {
	seen := map[string]bool{}
	extended := map[string]bool{}
	for _, layer := range layers {
		lb, err := spec.LoadLayered([]spec.Layer{layer})
		if err != nil {
			return nil, err
		}
		for _, h := range lb.Hooks {
			if layer.Extends && seen[h.Name] {
				extended[h.Name] = true
			}
			seen[h.Name] = true
		}
	}
	return extended, nil
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
