package cli

import (
	"fmt"
	"os"
	"path/filepath"
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

// portableHookTargetsNote names the configured targets that run hooks but
// skip a portable one until their mapping lands.
func portableHookTargetsNote(root string) string {
	reach := andList(spec.PortableHookTargets())
	var missing []string
	if cfg, _, err := loadProject(root); err == nil {
		for _, t := range cfg.Targets {
			if _, runs := targetsSupportingKind[spec.KindHook][t]; runs && !spec.TranslatesPortableHooks(t) {
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

func planHooksPortableEvents(root string) ([]migrationChange, []migrationSkip, error) {
	cfg, b, err := loadProject(root)
	if err != nil {
		return nil, nil, err
	}
	extended, err := extendedHookNames(root, cfg)
	if err != nil {
		return nil, nil, err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, nil, err
	}
	var changes []migrationChange
	var skips []migrationSkip
	for _, h := range b.Hooks {
		event, ok := h.Meta["event"].(string)
		if !ok || event == "" {
			continue
		}
		skip := func(reason string) { skips = append(skips, migrationSkip{Path: h.Path, Reason: reason}) }
		if spec.IsPortableHook(h.Meta) {
			skips = append(skips, migrationSkip{Path: h.Path, Reason: "sets both the native and the portable form; keep one by hand", Actionable: true})
			continue
		}
		if pack, ok := strings.CutPrefix(h.Layer, "pack:"); ok {
			skip("comes from pack " + pack + "; its author migrates it")
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
		reach := hookMigrationTargets(cfg, h)
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
		if real, err := filepath.EvalSymlinks(h.Path); err != nil || !pathWithin(realRoot, real) {
			skip("resolves outside the project")
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
func extendedHookNames(root string, cfg *config.Config) (map[string]bool, error) {
	seen := map[string]bool{}
	extended := map[string]bool{}
	for _, layer := range resolveLayers(root, cfg) {
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
