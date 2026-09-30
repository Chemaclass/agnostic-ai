package cli

import (
	"fmt"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// lintRuleFolderScope flags a rule whose folder names a project directory
// while its frontmatter `scope` points outside it (LINT020, warning). The
// frontmatter wins, so a reader of the tree would guess the wrong scope.
func lintRuleFolderScope(rules []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, r := range rules {
		scope, ok := r.Meta["scope"].(string)
		if !ok || r.Folder == "" {
			continue
		}
		scope, err := spec.NormalizeScope(scope)
		if err != nil {
			continue
		}
		if scope == r.Folder || strings.HasPrefix(scope, r.Folder+"/") {
			continue
		}
		out = append(out, lintFinding{
			Code: "LINT020", Severity: lintWarn, Path: r.Path,
			Message: fmt.Sprintf("the folder rules/%s/ names the %s/ directory, but `scope: %s` applies the rule to %s/: move the file out of that folder, or drop `scope` to scope it by folder", r.Folder, r.Folder, scope, scope),
		})
	}
	return out
}
