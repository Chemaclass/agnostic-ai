package spec

import (
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
