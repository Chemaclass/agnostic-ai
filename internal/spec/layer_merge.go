package spec

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// parentMarker is the body line an extending spec uses to place the
// body it extends.
const parentMarker = "::parent"

// extendEntry merges over, from a local layer, into base. Frontmatter
// merges key by key: maps merge recursively, scalars and lists replace,
// and null deletes. An empty body keeps base's; a body with a
// `::parent` line puts base's body there; any other body replaces it.
// A skill keeps base's assets unless over ships assets of its own. Path
// and Scope stay over's: they come from the file the author edits.
func extendEntry(base, over Entry) Entry {
	out := over
	shared := base
	out.Base, out.LocalMeta = &shared, over.Meta
	base = withSharedDefaults(base, over)
	out.Meta = mergeMeta(base.Meta, over.Meta)
	out.MetaKeys = mergeMetaKeys(base.MetaKeys, over.MetaKeys, out.Meta)
	out.NestedKeys = mergeNestedKeys(base.NestedKeys, over.NestedKeys, out.Meta)
	out.MetaStyles = mergeMetaStyles(base.MetaStyles, over.MetaStyles, out.Meta)
	out.Literals = mergeLiterals(base, over, out.Meta)
	if over.Kind == KindHook {
		out.HookAssetDirs = mergeHookAssetDirs(base, over, out.Meta)
	}
	switch {
	case strings.TrimSpace(over.Body) == "":
		out.Body = base.Body
		out.bodyLayer = base.BodySourceLayer()
		out.OpeningBlockKept = base.OpeningBlockKept
	default:
		out.Body = expandParent(over.Body, base.Body)
	}
	if out.Body != over.Body {
		out.BodyLine = 0
	}
	if over.Kind == KindRule {
		out.Scope = out.folderScope()
		if len(out.NativeRuleTargets()) > 0 {
			out.Scope = out.ruleFolder
		}
	}
	if over.Kind == KindSkill && !skillShipsAssets(over.Path) {
		out.AssetDir = base.SkillAssetDir()
		out.Assets = base.Assets
	}
	return out
}

// perToolDefaultFields are the settings fields whose per-tool map falls
// back to its default entry for the tools it leaves out.
var perToolDefaultFields = []string{"model", "effort"}

// withSharedDefaults reads a shared single model or effort in a settings
// spec as the default entry of a per-tool map, when the local value is
// one, so the local entries merge over it instead of replacing it.
func withSharedDefaults(base, over Entry) Entry {
	if over.Kind != KindSettings {
		return base
	}
	for _, key := range perToolDefaultFields {
		shared, set := base.Meta[key]
		_, sharedMap := shared.(map[string]any)
		if _, localMap := over.Meta[key].(map[string]any); !localMap || !set || shared == nil || sharedMap {
			continue
		}
		base.Meta = maps.Clone(base.Meta)
		base.Meta[key] = map[string]any{"default": shared}
		base.NestedKeys = maps.Clone(base.NestedKeys)
		if base.NestedKeys == nil {
			base.NestedKeys = map[string][]string{}
		}
		base.NestedKeys[key] = []string{"default"}
	}
	return base
}

func mergeMeta(base, over map[string]any) map[string]any {
	return mergeMap(base, over, true, false)
}

// mergeMap merges over into base. keepNull holds for the direct children
// of an x-<target> map, where null is the target resolver's delete
// marker, not a merge instruction.
func mergeMap(base, over map[string]any, top, keepNull bool) map[string]any {
	out := make(map[string]any, len(base)+len(over))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		if v == nil && !keepNull {
			delete(out, k)
			continue
		}
		if om, ok := v.(map[string]any); ok {
			if bm, ok := out[k].(map[string]any); ok {
				out[k] = mergeMap(bm, om, false, top && strings.HasPrefix(k, "x-"))
				continue
			}
		}
		out[k] = v
	}
	return out
}

// mergeMetaKeys keeps base's key order and appends over's new keys.
func mergeMetaKeys(base, over []string, meta map[string]any) []string {
	if base == nil && over == nil {
		return nil
	}
	var keys []string
	for _, k := range append(append([]string(nil), base...), over...) {
		if _, ok := meta[k]; ok && !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	return keys
}

func mergeMetaStyles(base, over map[string]yaml.Style, meta map[string]any) map[string]yaml.Style {
	if base == nil && over == nil {
		return nil
	}
	out := map[string]yaml.Style{}
	for _, styles := range []map[string]yaml.Style{base, over} {
		for k, v := range styles {
			if _, ok := meta[k]; ok {
				out[k] = v
			}
		}
	}
	return out
}

// expandParent replaces each `::parent` line in body with parent. An
// empty parent drops the line.
func expandParent(body, parent string) string {
	if !strings.Contains(body, parentMarker) {
		return body
	}
	parent = strings.TrimRight(parent, "\n")
	// A shared body may end inside an open ::target fence; close it so
	// the local lines after ::parent still reach every target.
	if strings.Contains(parent, targetFenceOpen) {
		parent += "\n::end"
	}
	lines := strings.Split(body, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimRight(line, " \t") != parentMarker {
			out = append(out, line)
			continue
		}
		if parent != "" {
			out = append(out, parent)
		}
	}
	return strings.Join(out, "\n")
}

// skillShipsAssets reports whether a folder skill holds a regular file besides
// its SKILL.md.
func skillShipsAssets(path string) bool {
	if filepath.Base(path) != "SKILL.md" {
		return false
	}
	root := filepath.Dir(path)
	found := false
	_ = WalkSourceRoot(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || found {
			return filepath.SkipDir
		}
		if d.Type().IsRegular() && p != path {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}
