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

// lintGeminiHookVariables flags a Gemini hook command that writes one of
// those variables bare inside quotes, where Gemini's escaped value breaks
// the quoting.
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
			for _, v := range bareGeminiVariables(h.Command) {
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
			Message: fmt.Sprintf("Hook %q holds a bare %s; Gemini replaces it as text before the shell runs, and a project path holding shell syntax or another such name can run as code. Write \"${NAME}\" (Gemini leaves the braced form to the shell)",
				hook.Name, strings.Join(found, ", ")),
		})
	}
	return out
}

// bareGeminiVariables returns each Gemini-replaced variable written bare
// in command. Gemini inserts a shell-escaped value before the shell parses
// the command, and replaces the names one after another, so a project path
// that holds another name is replaced again inside the inserted quotes.
// No bare form is safe; `${NAME}` is never replaced and always is.
func bareGeminiVariables(command string) []string {
	var out []string
	for _, v := range geminiHookVariables {
		if strings.Contains(command, v) {
			out = append(out, v)
		}
	}
	return out
}
