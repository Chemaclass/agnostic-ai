package cli

import (
	"fmt"
	"maps"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// mcpSecretFields are the MCP fields whose values are references unless
// the spec marks one `!literal`.
var mcpSecretFields = []string{"env", "headers"}

// mcpUnmarked is an `env` or `headers` value that is neither a
// reference nor marked `!literal`.
type mcpUnmarked struct {
	field, key, value string
}

// mcpReferenceValue reports whether value is only `${NAME}` or
// `${NAME:-default}` references or only `$${NAME}` text, after `Bearer `
// or, in a header, another one-word scheme such as `token ${GITHUB_TOKEN}`.
func mcpReferenceValue(field, value string) bool {
	if field == "headers" {
		value = mcpSchemeWord.ReplaceAllString(value, "")
	}
	return spec.OnlyEnvRefs(value) || spec.OnlyEscapedEnvRefs(value)
}

// mcpUnmarkedLiterals lists e's `env` and `headers` string values that
// are neither references nor marked `!literal`, by field and then key.
// An empty value holds nothing, and a number or boolean is no secret, so
// neither counts.
func mcpUnmarkedLiterals(e spec.Entry) []mcpUnmarked {
	var out []mcpUnmarked
	for _, field := range mcpSecretFields {
		values, _ := e.Meta[field].(map[string]any)
		for _, key := range slices.Sorted(maps.Keys(values)) {
			value, ok := values[key].(string)
			if !ok || value == "" || e.MarkedLiteral(field, key) || mcpReferenceValue(field, value) {
				continue
			}
			out = append(out, mcpUnmarked{field: field, key: key, value: value})
		}
	}
	return out
}

// mcpLiteralIssues reports each MCP `env` and `headers` value that is
// neither a reference nor marked `!literal`. The message names the
// server, field, and key, never the value, since it lands in CI logs.
func mcpLiteralIssues(mcps []spec.Entry) []validationIssue {
	var out []validationIssue
	for _, e := range mcps {
		for _, l := range mcpUnmarkedLiterals(e) {
			out = append(out, validationIssue{
				Path:  e.Path,
				Field: l.field,
				Message: fmt.Sprintf("MCP server %q: %s %s is a literal value; write a ${NAME} reference, or mark a plain setting `!literal`. `agnostic-ai migrate --only secrets` does both",
					e.Name, l.field, l.key),
			})
		}
	}
	return out
}

// lintMCPLiterals reports mcpLiteralIssues as LINT035 errors.
func lintMCPLiterals(mcps []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, issue := range mcpLiteralIssues(mcps) {
		out = append(out, lintFinding{Code: "LINT035", Severity: lintError, Path: issue.Path, Message: issue.Message})
	}
	return out
}

// mcpCredentialSetting reports whether import's detector reads an `env`
// or header value as a credential: a value under a credential name, or
// one that holds a credential by its shape.
func mcpCredentialSetting(key, value string) bool {
	return mcpCredentialKey(key) && mcpSecretValue(mcpDetectorText(value), false, mcpWeakCredential) || mcpCredentialDetected(value)
}

// mcpMarkedLiteral is a value import writes as `!literal value`.
type mcpMarkedLiteral string

func (v mcpMarkedLiteral) MarshalYAML() (any, error) {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: spec.LiteralTag, Value: string(v)}, nil
}

// markMCPLiterals returns server with each `env` and `headers` value
// that is neither a reference nor empty marked `!literal`, so the spec
// import writes keeps its plain settings without a lint warning. A value
// under a credential name stays unmarked, so lint asks the user about
// it. The maps in server stay as they are.
func markMCPLiterals(server map[string]any) map[string]any {
	out := maps.Clone(server)
	for _, field := range mcpSecretFields {
		values, ok := server[field].(map[string]any)
		if !ok {
			continue
		}
		marked := maps.Clone(values)
		for key, raw := range values {
			if value, ok := raw.(string); ok && value != "" && !mcpCredentialKey(key) && !mcpReferenceValue(field, value) {
				marked[key] = mcpMarkedLiteral(value)
			}
		}
		out[field] = marked
	}
	return out
}
