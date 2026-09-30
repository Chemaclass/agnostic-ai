package spec

import (
	"fmt"
	"path"
	"strings"
)

// NormalizeScope validates the portable project-relative directory syntax.
// Validate before trimming separators: /outside must never become outside.
func NormalizeScope(scope string) (string, error) {
	if scope == "" {
		return "", nil
	}
	if strings.TrimSpace(scope) != scope || strings.ContainsAny(scope, "\\:*?[]{}!,()\r\n\x00") || strings.HasPrefix(scope, "/") {
		return "", fmt.Errorf("invalid scope %q: use a project-relative directory", scope)
	}
	for _, part := range strings.Split(scope, "/") {
		if part == ".." {
			return "", fmt.Errorf("invalid scope %q: parent traversal is not allowed", scope)
		}
		// Import never walks into node_modules, so output there would not
		// round-trip.
		if part == "node_modules" {
			return "", fmt.Errorf("invalid scope %q: node_modules holds installed packages, not project files", scope)
		}
	}
	scope = path.Clean(scope)
	if scope == "." {
		return "", fmt.Errorf("invalid scope %q: omit scope for project-wide rules", scope)
	}
	return scope, nil
}

// RuleScope returns the rule's directory scope. A target-resolved
// frontmatter `scope` wins over the layout scope.
func RuleScope(e Entry) (string, error) {
	scope := e.Scope
	if raw, ok := e.Meta["scope"]; ok {
		var valid bool
		scope, valid = raw.(string)
		if !valid {
			return "", fmt.Errorf("%s: scope must be a directory string", e.Path)
		}
	}
	result, err := NormalizeScope(scope)
	if err != nil {
		return "", fmt.Errorf("%s: %w", e.Path, err)
	}
	return result, nil
}
