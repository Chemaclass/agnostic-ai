package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// toolRefSyntaxes are the reference forms one tool reads that a spec
// does not. In a spec `url` or `args` sync copies them as text, so only
// the tools that own the form expand them.
var toolRefSyntaxes = []spec.EnvRefSyntax{spec.EnvRefDollarEnv, spec.EnvRefBraceEnv, spec.EnvRefSecrets}

// lintMCPToolRefs flags a tool's own reference form at the top level of
// the MCP `url` or `args` that sync writes for the server's transport,
// when an enabled target reads it as text. Older imports wrote these. An
// `x-<target>` block is that tool's own text and is not checked.
func lintMCPToolRefs(targets []string, mcps []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, e := range mcps {
		field := mcpLaunchField(e.Meta)
		var refs, portable, literal []string
		for _, value := range mcpLaunchStrings(e.Meta[field]) {
			for _, syntax := range toolRefSyntaxes {
				for _, ref := range syntax.LaunchRefs(value) {
					if slices.Contains(refs, ref) {
						continue
					}
					var readers []string
					for _, t := range targets {
						if adapters.MCPLaunchRefSyntax(t, field) == syntax {
							readers = append(readers, t)
						} else if !slices.Contains(literal, t) {
							literal = append(literal, t)
						}
					}
					if len(readers) == len(targets) {
						continue
					}
					refs = append(refs, ref)
					portable = append(portable, syntax.ReadLaunch(ref, spec.EnvRefReading{}))
				}
			}
		}
		if len(refs) == 0 {
			continue
		}
		// Only the references are quoted: the value around them may hold
		// a secret, and lint output lands in CI logs.
		out = append(out, lintFinding{
			Code:     "LINT028",
			Severity: lintWarn,
			Path:     e.Path,
			Message: fmt.Sprintf("MCP server %q has %s in `%s`, which sync copies as text to %s. Write %s so sync writes each tool's own form",
				e.Name, strings.Join(refs, ", "), field, strings.Join(literal, ", "), strings.Join(portable, ", ")),
		})
	}
	return out
}

// mcpLaunchField is the field sync writes for the server's transport:
// `args` for stdio, `url` for a remote server.
func mcpLaunchField(meta map[string]any) string {
	switch transport, _ := meta["type"].(string); transport {
	case "stdio", "local":
		return "args"
	case "":
		if _, hasCommand := meta["command"]; hasCommand || meta["url"] == nil {
			return "args"
		}
	}
	return "url"
}

// mcpLaunchStrings returns a url, or each string argument.
func mcpLaunchStrings(v any) []string {
	switch v := v.(type) {
	case string:
		return []string{v}
	case []any:
		var out []string
		for _, arg := range v {
			if s, ok := arg.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	}
	return nil
}
