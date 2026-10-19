package cli

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

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

// mcpReferenceDefault matches the default of a `${NAME:-default}`
// reference.
var mcpReferenceDefault = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*:-([^}]*)\}`)

// agentFileCredentialIssues reports credentials an agent file would
// carry beyond what mcpLiteralIssues checks: in `url` and `args`, in an
// `x-<target>` block's `env` and `headers`, and in a reference's
// default. An agent file is committed with every copy of the agent.
// The message names the server and field, never the value.
func agentFileCredentialIssues(mcps []spec.Entry) []validationIssue {
	var out []validationIssue
	for _, e := range mcps {
		blocks := []map[string]any{e.Meta}
		for _, key := range slices.Sorted(maps.Keys(e.Meta)) {
			if x, ok := e.Meta[key].(map[string]any); ok && strings.HasPrefix(key, "x-") {
				blocks = append(blocks, x)
			}
		}
		var fields []string
		for i, meta := range blocks {
			for _, field := range agentFileCredentialFields(meta, i > 0) {
				if !slices.Contains(fields, field) {
					fields = append(fields, field)
				}
			}
		}
		for _, field := range fields {
			out = append(out, validationIssue{Path: e.Path, Field: field,
				Message: fmt.Sprintf("MCP server %q: %s holds a credential, and sync would write it into an agent file; use a ${NAME} reference with no default", e.Name, field)})
		}
	}
	return out
}

// agentFileCredentialFields lists the fields of one MCP block that hold
// a credential. inBlock marks an `x-<target>` block, where a value
// cannot be marked `!literal`.
func agentFileCredentialFields(meta map[string]any, inBlock bool) []string {
	var out []string
	if u, _ := meta["url"].(string); u != "" && mcpCredentialDetected(u) {
		out = append(out, "url")
	}
	if args := emitStrings(meta["args"]); len(args) > 0 && mcpCredentialDetected(strings.Join(args, " ")) {
		out = append(out, "args")
	}
	for _, field := range mcpSecretFields {
		values, _ := meta[field].(map[string]any)
		for _, key := range slices.Sorted(maps.Keys(values)) {
			value, _ := values[key].(string)
			if value == "" {
				continue
			}
			literal := inBlock && !mcpReferenceValue(field, value) && mcpCredentialSetting(key, value)
			if literal || referenceDefaultCredential(key, value) {
				out = append(out, field+" "+key)
			}
		}
	}
	return out
}

func referenceDefaultCredential(key, value string) bool {
	for _, m := range mcpReferenceDefault.FindAllStringSubmatch(value, -1) {
		if m[1] != "" && (mcpCredentialKey(key) || mcpCredentialDetected(m[1])) {
			return true
		}
	}
	return false
}

func emitStrings(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
