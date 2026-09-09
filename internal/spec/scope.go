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
	}
	scope = path.Clean(scope)
	if scope == "." {
		return "", fmt.Errorf("invalid scope %q: omit scope for project-wide rules", scope)
	}
	return scope, nil
}

// RuleScope keeps layout-derived scope ahead of target-resolved frontmatter.
func RuleScope(e Entry) (string, error) {
	scope := e.Scope
	if scope == "" {
		if raw, ok := e.Meta["scope"]; ok {
			var valid bool
			scope, valid = raw.(string)
			if !valid {
				return "", fmt.Errorf("%s: scope must be a directory string", e.Path)
			}
		}
	}
	result, err := NormalizeScope(scope)
	if err != nil {
		return "", fmt.Errorf("%s: %w", e.Path, err)
	}
	return result, nil
}
