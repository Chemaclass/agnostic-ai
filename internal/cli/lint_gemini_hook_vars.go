package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// geminiHookVariables are replaced in a hook command before its shell runs,
// whatever the quoting around them. The match is the bare `$NAME`, so the
// `${NAME}` form is left alone:
// https://github.com/google-gemini/gemini-cli/blob/fb972b2f87fe7d5b06d37eac711490162d98de2c/packages/core/src/hooks/hookRunner.ts#L526-L531
var geminiHookVariables = []string{
	"$GEMINI_PROJECT_DIR",
	"$GEMINI_CWD",
	"$GEMINI_PLANS_DIR",
	"$GEMINI_SESSION_ID",
	"$CLAUDE_PROJECT_DIR",
}

// lintGeminiHookVariables flags a Gemini hook command that wraps one of
// those variables in single quotes, where Gemini's own single-quoted
// value closes the quoting.
func lintGeminiHookVariables(cfg *config.Config, targets []string, b spec.Bundle) []lintFinding {
	if !slices.Contains(targets, "gemini") {
		return nil
	}
	var out []lintFinding
	for _, hook := range b.For("gemini").Hooks {
		handlers, err := adapters.HookHandlers(cfg, "gemini", hook)
		if err != nil {
			continue
		}
		var found []string
		for _, h := range handlers {
			for _, v := range singleQuotedGeminiVariables(h.Command) {
				if !slices.Contains(found, v) {
					found = append(found, v)
				}
			}
		}
		if len(found) == 0 {
			continue
		}
		out = append(out, lintFinding{
			Code:     "LINT030",
			Severity: lintWarn,
			Path:     hook.Path,
			Message: fmt.Sprintf("Hook %q holds %s inside single quotes; Gemini replaces it with a single-quoted value anyway, which closes the quoting. Drop the single quotes or write it in double quotes",
				hook.Name, strings.Join(found, ", ")),
		})
	}
	return out
}

// singleQuotedGeminiVariables returns each Gemini-replaced variable that
// appears between single quotes, tracking shell quoting: a quote inside
// double quotes or after a backslash does not open a span.
func singleQuotedGeminiVariables(command string) []string {
	var out []string
	inSingle, inDouble := false, false
	for i := 0; i < len(command); i++ {
		c := command[i]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
				continue
			}
			if c != '$' {
				continue
			}
			for _, v := range geminiHookVariables {
				if strings.HasPrefix(command[i:], v) && !slices.Contains(out, v) {
					out = append(out, v)
					break
				}
			}
		case c == '\\':
			i++
		case c == '"':
			inDouble = !inDouble
		case c == '\'' && !inDouble:
			inSingle = true
		}
	}
	return out
}
