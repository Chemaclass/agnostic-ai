package cli

import (
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// claudeNativeAgentFields are the agent frontmatter fields Claude Code's
// subagent page documents. Claude Code reads a top-level key as written
// and "ignores a field it doesn't recognize without reporting an error",
// so a misspelling of one of these does nothing.
var claudeNativeAgentFields = []string{
	"name", "description", "tools", "disallowedTools", "model", "permissionMode",
	"maxTurns", "autoCompactWindow", "skills", "mcpServers", "hooks", "memory",
	"background", "omitClaudeMd", "effort", "isolation", "color", "initialPrompt",
	"experimental",
}

// claudeAgentNearMiss returns the Claude Code agent field key spells
// with other case, `_`, or `-`, such as `max_turns` for `maxTurns`.
func claudeAgentNearMiss(kind spec.Kind, key string, targets []string) (string, bool) {
	if kind != spec.KindAgent || !slices.Contains(targets, "claude") || slices.Contains(claudeNativeAgentFields, key) {
		return "", false
	}
	folded := foldFieldName(key)
	for _, field := range claudeNativeAgentFields {
		if foldFieldName(field) == folded {
			return field, true
		}
	}
	return "", false
}

func foldFieldName(s string) string {
	return strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(s))
}

// lintAgentDescriptions flags an agent with no description (LINT048,
// warn). Tools read it to decide when to hand the agent a task, and
// Claude Code skips an agent file without one. An agent with no body
// either is LINT001's.
func lintAgentDescriptions(entries []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, e := range entries {
		if e.Kind != spec.KindAgent || strings.TrimSpace(e.Body) == "" {
			continue
		}
		if desc, _ := e.Meta["description"].(string); strings.TrimSpace(desc) != "" {
			continue
		}
		out = append(out, lintFinding{
			Code:     "LINT048",
			Severity: lintWarn,
			Path:     e.Path,
			Message:  "agent has no description, which tools read to decide when to hand it a task; Claude Code gets the name instead",
		})
	}
	return out
}
