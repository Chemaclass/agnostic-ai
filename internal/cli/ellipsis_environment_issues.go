package cli

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// ellipsisVariableIssues reports each `x-ellipsis.environment.variables`
// value that is a credential by its name or its shape, such as
// `DATABASE_URL: postgres://admin:pw@db/app`. The environment file is
// committed, so a secret belongs in Ellipsis: a variable with no value
// reads the stored secret of the same name. The message names the
// variable, never the value.
func ellipsisVariableIssues(settings []spec.Entry) []validationIssue {
	var out []validationIssue
	for _, e := range settings {
		x, _ := e.Meta["x-ellipsis"].(map[string]any)
		env, _ := x["environment"].(map[string]any)
		variables, _ := env["variables"].([]any)
		for _, item := range variables {
			v, _ := item.(map[string]any)
			name, _ := v["name"].(string)
			value, _ := v["value"].(string)
			if value == "" || (!mcpCredentialKey(name) && !mcpCredentialDetected(value)) {
				continue
			}
			out = append(out, validationIssue{
				Path:    e.Path,
				Field:   "x-ellipsis.environment.variables",
				Message: fmt.Sprintf("variable %s holds a credential value, which would be committed; omit value so Ellipsis reads the stored secret named %s", name, name),
			})
		}
	}
	return out
}
