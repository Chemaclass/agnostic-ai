package spec

import "strings"

// MCPToolPrefix marks a permission rule naming one MCP server tool.
const MCPToolPrefix = "mcp__"

// SplitPermissionRule parses the `Scope(argument)` form a Settings spec
// writes its permission lists in, documented under Settings in
// spec-format.md. A bare tool name is a whole-tool rule and not one of
// these, so it reports false rather than an empty argument.
//
// The scope ends at the first `(` and the argument runs to the trailing
// `)`, so a nested paren stays inside the argument.
//
// This lives here, rather than in the adapters' shared emit package,
// because the importers under internal/cli parse the same grammar and
// cannot reach internal/adapters/internal/emit. Four byte-identical
// copies existed before this one.
func SplitPermissionRule(rule string) (scope, arg string, ok bool) {
	scope, rest, found := strings.Cut(rule, "(")
	if !found || scope == "" || !strings.HasSuffix(rest, ")") {
		return "", "", false
	}
	arg = strings.TrimSuffix(rest, ")")
	if arg == "" {
		return "", "", false
	}
	return scope, arg, true
}

// SplitMCPPermissionRule parses the `mcp__<server>__<tool>` form into
// its two halves. Only the first separator after the prefix divides
// them, so a tool name carrying its own `__` survives intact.
//
// Callers compose the vendor's own spelling from the result: Kilo Code
// writes `<server>_<tool>` and Augment writes `<tool>_<server>`, each
// with its own extra rules. Only the parse is shared.
func SplitMCPPermissionRule(rule string) (server, tool string, ok bool) {
	rest, found := strings.CutPrefix(rule, MCPToolPrefix)
	if !found {
		return "", "", false
	}
	server, tool, found = strings.Cut(rest, "__")
	if !found || server == "" || tool == "" {
		return "", "", false
	}
	return server, tool, true
}
