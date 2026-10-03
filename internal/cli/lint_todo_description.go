package cli

import (
	"fmt"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// lintTodoDescriptions flags a spec whose description still starts with the
// TODO placeholder `new` writes (LINT031, warn). Sync copies that text to
// every target, where it shows in tool pickers and decides skill triggers.
func lintTodoDescriptions(entries []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, e := range entries {
		desc, _ := e.Meta["description"].(string)
		if !strings.HasPrefix(strings.TrimSpace(desc), "TODO") {
			continue
		}
		out = append(out, lintFinding{
			Code:     "LINT031",
			Severity: lintWarn,
			Path:     e.Path,
			Message:  fmt.Sprintf("%s %q keeps the placeholder `description` from `new`; sync copies it to every target. Replace it with what the spec is for", e.Kind, e.Name),
		})
	}
	return out
}
