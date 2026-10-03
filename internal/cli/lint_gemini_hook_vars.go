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
			for _, v := range quotedGeminiVariables(h.Command) {
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
			Message: fmt.Sprintf("Hook %q holds %s inside quotes or a command substitution; Gemini replaces it with a shell-escaped value before the shell runs, which breaks the quoting. Write \"${NAME}\" (Gemini leaves the braced form to the shell) or drop the quotes",
				hook.Name, strings.Join(found, ", ")),
		})
	}
	return out
}

// quotedGeminiVariables returns each Gemini-replaced variable written bare
// inside single or double quotes. Gemini inserts a shell-escaped value
// before the shell parses the command, so the value's own apostrophes
// show up literally in double quotes and close the quotes in single ones.
// Unquoted words and `${NAME}` are fine. A `#` at the start of a word
// outside quotes begins a comment, so its apostrophes open nothing. Quotes
// inside a command substitution start a new context this scan does not
// track, so every bare variable after a `$(` or backquote counts.
func quotedGeminiVariables(command string) []string {
	var out []string
	inSingle, inDouble, inSubstitution := false, false, false
	for i := 0; i < len(command); i++ {
		c := command[i]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
			}
		case c == '\\':
			i++
		case c == '`' || c == '$' && i+1 < len(command) && command[i+1] == '(':
			inSubstitution = true
		case c == '"':
			inDouble = !inDouble
		case c == '\'' && !inDouble:
			inSingle = true
		case c == '#' && !inDouble && (i == 0 || strings.ContainsRune(" \t\n;&|()", rune(command[i-1]))):
			for i < len(command) && command[i] != '\n' {
				i++
			}
		}
		if c != '$' || (!inSingle && !inDouble && !inSubstitution) {
			continue
		}
		for _, v := range geminiHookVariables {
			if strings.HasPrefix(command[i:], v) && !slices.Contains(out, v) {
				out = append(out, v)
				break
			}
		}
	}
	return out
}
