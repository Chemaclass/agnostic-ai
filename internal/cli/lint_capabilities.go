package cli

import (
	"fmt"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func lintNeutralCapabilities(b spec.Bundle) []lintFinding {
	var out []lintFinding
	for _, e := range b.All() {
		field := ""
		switch e.Kind {
		case spec.KindAgent:
			field = "tools"
			if _, set := e.Meta["can"]; set {
				field = "can"
			}
		case spec.KindSkill:
			field = "allowed-tools"
		}
		if field != "" {
			names := toStringSlice(e.Meta[field])
			if text, ok := e.Meta[field].(string); ok {
				names = append(names, spec.SplitToolList(text)...)
			}
			for _, name := range names {
				if neutral, ok := spec.NeutralCapability(name); ok && neutral != name {
					out = append(out, lintFinding{Code: "LINT037", Severity: lintWarn, Path: e.Path, Message: fmt.Sprintf("%s: use %s instead of %s", field, neutral, name)})
				}
			}
		}
		if e.Kind == spec.KindSettings {
			perms, _ := e.Meta["permissions"].(map[string]any)
			for _, list := range spec.PermissionLists {
				for _, rule := range toStringSlice(perms[list]) {
					if neutral, ok := spec.NeutralPermission(rule); ok && neutral != rule {
						out = append(out, lintFinding{Code: "LINT037", Severity: lintWarn, Path: e.Path, Message: fmt.Sprintf("permissions.%s: use %s instead of %s", list, neutral, rule)})
					}
				}
			}
		}
	}
	return out
}

func filterLintToFiles(findings []lintFinding, files []string) []lintFinding {
	var out []lintFinding
	for _, finding := range findings {
		path, _ := filepath.Abs(finding.Path)
		for _, file := range files {
			absolute, _ := filepath.Abs(file)
			if path == absolute {
				out = append(out, finding)
				break
			}
		}
	}
	return out
}
