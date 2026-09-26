package spec

import (
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
// A skill keeps base's assets unless over ships assets of its own; Path
// stays over's, since it names the file the author edits.
func extendEntry(base, over Entry) Entry {
	out := over
	out.Meta = mergeMeta(base.Meta, over.Meta)
	out.MetaKeys = mergeMetaKeys(base.MetaKeys, over.MetaKeys, out.Meta)
	out.MetaStyles = mergeMetaStyles(base.MetaStyles, over.MetaStyles, out.Meta)
	switch {
	case strings.TrimSpace(over.Body) == "":
		out.Body = base.Body
	default:
		out.Body = expandParent(over.Body, base.Body)
	}
	if over.Scope == "" {
		out.Scope = base.Scope
	}
	if over.Kind == KindSkill && !skillShipsAssets(over.Path) {
		out.AssetDir = base.SkillAssetDir()
	}
	return out
}

func mergeMeta(base, over map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(over))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		if v == nil {
			delete(out, k)
			continue
		}
		if om, ok := v.(map[string]any); ok {
			if bm, ok := out[k].(map[string]any); ok {
				out[k] = mergeMeta(bm, om)
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

// skillShipsAssets reports whether a folder skill holds any file besides
// its SKILL.md.
func skillShipsAssets(path string) bool {
	if filepath.Base(path) != "SKILL.md" {
		return false
	}
	root := filepath.Dir(path)
	found := false
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || found {
			return filepath.SkipDir
		}
		if !d.IsDir() && p != path {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}
