package ellipsis

import (
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// reviewPrompt joins the review specs into the reviewer's prompt: root
// guidance first, then one section per scope in sorted order. A scoped
// section says when it applies, so directory-specific checks never turn
// into checks on every change. "" when no review can reach the file.
func reviewPrompt(reviews []spec.Entry) string {
	byScope := map[string][]string{}
	for _, r := range reviews {
		scope := cleanScope(r.EffectiveScope())
		if emit.ScopeEscapesRoot(scope) {
			continue
		}
		if body := strings.Trim(r.Body, "\n"); strings.TrimSpace(body) != "" {
			byScope[scope] = append(byScope[scope], body)
		}
	}
	var parts []string
	if root := byScope[""]; len(root) > 0 {
		parts = append(parts, strings.Join(root, "\n\n"))
	}
	scopes := make([]string, 0, len(byScope))
	for scope := range byScope {
		if scope != "" {
			scopes = append(scopes, scope)
		}
	}
	sort.Strings(scopes)
	for _, scope := range scopes {
		parts = append(parts, scopedSection(scope, byScope[scope]))
	}
	return strings.Join(parts, "\n\n")
}

// cleanScope puts `./services/api/` and `services/api` in one section.
func cleanScope(scope string) string {
	scope = filepath.ToSlash(scope)
	if scope == "" {
		return ""
	}
	if scope = path.Clean(scope); scope == "." {
		return ""
	}
	return scope
}

func scopedSection(scope string, bodies []string) string {
	dir := scope + "/"
	return "## Checks for changes under " + dir + "\n\n" +
		"Apply the checks in this section only when the pull request changes a file under `" + dir + "`. Skip them otherwise.\n\n" +
		strings.Join(bodies, "\n\n")
}
