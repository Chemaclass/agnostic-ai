package cli

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// toolRefSyntaxes are the reference forms one tool reads that a spec
// does not. In a spec `url` or `args` sync copies them as text to every
// tool, so only the tool that owns the form expands them.
var toolRefSyntaxes = []spec.EnvRefSyntax{spec.EnvRefDollarEnv, spec.EnvRefBraceEnv, spec.EnvRefSecrets}

// lintMCPToolRefs flags a tool's own reference form at the top level of
// an MCP `url` or `args` value, which older imports wrote. An
// `x-<target>` block is that tool's own text and is not checked.
func lintMCPToolRefs(mcps []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, e := range mcps {
		for _, field := range []string{"url", "args"} {
			for _, value := range mcpLaunchStrings(e.Meta[field]) {
				portable := value
				for _, syntax := range toolRefSyntaxes {
					portable = syntax.ReadLaunch(portable, spec.EnvRefReading{})
				}
				if portable == value {
					continue
				}
				out = append(out, lintFinding{
					Code:     "LINT028",
					Severity: lintWarn,
					Path:     e.Path,
					Message: fmt.Sprintf("MCP server %q has %q in `%s`, a reference only one tool reads; the others get it as text. Write %q so sync writes each tool's own form",
						e.Name, value, field, portable),
				})
				break
			}
		}
	}
	return out
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
	}
	return nil
}
