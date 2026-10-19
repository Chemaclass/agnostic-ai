package cli

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/preview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// mcpReferenceDefault matches the default of a `${NAME:-default}`
// reference.
var mcpReferenceDefault = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*:-([^}]*)\}`)

// mcpNativeReference is a value that is only one tool's own reference
// form, such as `${env:NAME}` or `{env:NAME}`, which an `x-<target>`
// block may hold.
var mcpNativeReference = regexp.MustCompile(`^(\$\{[^}]+\}|\{env:[^}]+\})+$`)

// mcpCredentialFields lists the places in e, outside what LINT035
// checks, where the import detector finds a credential: `url`, `args`,
// an `x-<target>` block's `env` and `headers`, and the default of a
// reference. A `${NAME}` reference and a `!literal` plain setting pass.
// Each place reads as a field path, such as `x-cursor.env GITHUB_TOKEN`.
func mcpCredentialFields(e spec.Entry) []string {
	out := mcpBlockCredentialFields(e.Meta, "", false)
	for _, key := range slices.Sorted(maps.Keys(e.Meta)) {
		if block, ok := e.Meta[key].(map[string]any); ok && strings.HasPrefix(key, "x-") {
			out = append(out, mcpBlockCredentialFields(block, key+".", true)...)
		}
	}
	return out
}

// mcpBlockCredentialFields checks one MCP block. inBlock marks an
// `x-<target>` block, where no value can be marked `!literal`, so its
// `env` and `headers` values count only when they read as credentials.
func mcpBlockCredentialFields(meta map[string]any, prefix string, inBlock bool) []string {
	var out []string
	if u, _ := meta["url"].(string); u != "" && mcpCredentialDetected(u) {
		out = append(out, prefix+"url")
	}
	if args := mcpArgStrings(meta["args"]); len(args) > 0 && mcpCredentialDetected(strings.Join(args, " ")) {
		out = append(out, prefix+"args")
	}
	for _, field := range mcpSecretFields {
		values, _ := meta[field].(map[string]any)
		for _, key := range slices.Sorted(maps.Keys(values)) {
			value, _ := values[key].(string)
			if value == "" {
				continue
			}
			if inBlock && mcpBlockLiteral(field, key, value) || mcpCredentialDefault(key, value) {
				out = append(out, prefix+field+" "+key)
			}
		}
	}
	return out
}

func mcpBlockLiteral(field, key, value string) bool {
	if mcpReferenceValue(field, value) {
		return false
	}
	if field == "headers" {
		value = mcpSchemeWord.ReplaceAllString(value, "")
	}
	return !mcpNativeReference.MatchString(value) && mcpCredentialUnder(key, value)
}

// mcpCredentialDefault reports whether a reference in value falls back
// to a credential.
func mcpCredentialDefault(key, value string) bool {
	for _, m := range mcpReferenceDefault.FindAllStringSubmatch(value, -1) {
		if m[1] != "" && mcpCredentialUnder(key, m[1]) {
			return true
		}
	}
	return false
}

func mcpArgStrings(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// lintMCPCredentials reports mcpCredentialFields as LINT049 warnings.
// The message names the server and field, never the value.
func lintMCPCredentials(mcps []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, e := range mcps {
		for _, field := range mcpCredentialFields(e) {
			out = append(out, lintFinding{Code: "LINT049", Severity: lintWarn, Path: e.Path,
				Message: fmt.Sprintf("MCP server %q: %s holds what looks like a credential, which sync writes into each tool's MCP file; use a ${NAME} reference with no default", e.Name, field)})
		}
	}
	return out
}

// mcpCredentialUnder reports whether value under key reads as a
// credential: by its shape, or under a credential name unless it is a
// short placeholder such as `none` under a name that ends in `key`.
func mcpCredentialUnder(key, value string) bool {
	if mcpCredentialDetected(value) {
		return true
	}
	strength := mcpNameStrength(key, true)
	return strength != preview.NoCredential && mcpSecretValue(mcpDetectorText(value), true, strength)
}
