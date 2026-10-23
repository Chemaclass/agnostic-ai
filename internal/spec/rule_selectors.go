package spec

import (
	"slices"
	"sort"
	"strings"
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
	case "claude", "cline", "openhands", "codebuddy":
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
// that parses spec bytes itself applies it too. `paths` stays, so Claude
// and Cline write it back as the user wrote it. A malformed `paths`, an
// empty one (Cline's never-active rule), or one beside `globs` is left
// alone; validate rejects `paths` and `globs` that differ.
func (e Entry) PathsAsGlobs() Entry {
	paths, hasPaths := e.Meta["paths"]
	if _, hasGlobs := e.Meta["globs"]; e.Kind != KindRule || !hasPaths || hasGlobs || len(GlobList(paths)) == 0 {
		return e
	}
	meta := make(map[string]any, len(e.Meta)+1)
	for k, v := range e.Meta {
		meta[k] = v
	}
	meta["globs"] = paths
	e.Meta, e.globsFromPaths = meta, true
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

// LoadedKeys names the keys that hold a source frontmatter key after
// loading: a rule's `paths` and the `globs` read from it, or key alone.
func (e Entry) LoadedKeys(key string) []string {
	if key == "paths" && e.globsFromPaths {
		return []string{"paths", "globs"}
	}
	return []string{key}
}
