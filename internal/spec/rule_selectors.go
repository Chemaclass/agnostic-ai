package spec

import (
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Native selectors use the folder for placement; explicit scope adds activation.
func (e Entry) NativeRuleTargets() []string {
	if e.Kind != KindRule {
		return nil
	}
	if _, explicit := e.Meta["scope"]; explicit {
		return nil
	}
	var targets []string
	for key, value := range e.Meta {
		if !strings.HasPrefix(key, "x-") {
			continue
		}
		native, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if _, explicit := native["scope"]; explicit {
			continue
		}
		target := strings.TrimPrefix(key, "x-")
		for _, selector := range nativeRuleSelectorKeys(target) {
			if value, exists := native[selector]; exists && value != nil {
				targets = append(targets, target)
				break
			}
		}
	}
	sort.Strings(targets)
	return targets
}

func nativeRuleSelectorKeys(target string) []string {
	switch target {
	case "continue":
		return []string{"globs", "regex"}
	case "claude", "cline", "openhands":
		return []string{"paths"}
	case "qoder":
		return []string{"paths", "glob"}
	case "cursor", "windsurf", "trae", "antigravity":
		return []string{"globs"}
	default:
		return nil
	}
}

// PathsAsGlobs reads a rule's `paths:`, the Claude Code form, as the
// portable `globs:` every adapter reads. Loading applies it; a caller
// that parses spec bytes itself applies it too. A malformed `paths` stays for
// validate to report. When both keys are set, `globs` stays and `paths`
// goes only if they name the same patterns; validate rejects the rest
// (see PathsConflictGlobs).
func (e Entry) PathsAsGlobs() Entry {
	paths, hasPaths := e.Meta["paths"]
	if e.Kind != KindRule || !hasPaths || !ValidGlobs(paths) {
		return e
	}
	globs, hasGlobs := e.Meta["globs"]
	if hasGlobs && !slices.Equal(GlobList(globs), GlobList(paths)) {
		return e
	}
	meta := make(map[string]any, len(e.Meta))
	for k, v := range e.Meta {
		meta[k] = v
	}
	delete(meta, "paths")
	keys := make([]string, 0, len(e.MetaKeys))
	for _, k := range e.MetaKeys {
		switch {
		case k != "paths":
			keys = append(keys, k)
		case !hasGlobs:
			keys = append(keys, "globs")
		}
	}
	if !hasGlobs {
		meta["globs"] = paths
		if style, ok := e.MetaStyles["paths"]; ok {
			styles := make(map[string]yaml.Style, len(e.MetaStyles))
			for k, v := range e.MetaStyles {
				styles[k] = v
			}
			delete(styles, "paths")
			styles["globs"] = style
			e.MetaStyles = styles
		}
	}
	e.Meta, e.MetaKeys = meta, keys
	return e
}

// PathsConflictGlobs reports whether a rule sets both `paths` and
// `globs` to different patterns, so no single scope can win.
func (e Entry) PathsConflictGlobs() bool {
	paths, hasPaths := e.Meta["paths"]
	globs, hasGlobs := e.Meta["globs"]
	return e.Kind == KindRule && hasPaths && hasGlobs && ValidGlobs(paths) &&
		!slices.Equal(GlobList(globs), GlobList(paths))
}

// LoadedKey names the key that holds a source frontmatter key after
// loading: `globs` for a rule's `paths` alias, key itself otherwise.
func (e Entry) LoadedKey(key string) string {
	if _, kept := e.Meta[key]; !kept && key == "paths" && e.Kind == KindRule {
		return "globs"
	}
	return key
}
