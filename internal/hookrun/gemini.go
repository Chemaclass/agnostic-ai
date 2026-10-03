package hookrun

import (
	"encoding/json"
	"slices"
	"strings"
)

// geminiAdvisoryEvents ignore decision and continue (docs/hooks/reference.md).
var geminiAdvisoryEvents = []string{"SessionStart", "SessionEnd", "Notification", "PreCompress"}

// decideGemini follows gemini-cli c6bccb7 hookRunner.ts: the reply is
// trimmed stdout, or stderr when stdout is empty. JSON is read for
// decision and continue whatever the exit code. Plain text with an exit
// other than 0 and 1 denies; with no text at all, there is no decision,
// so a failing exit is only an error.
func decideGemini(event string, r Result) Decision {
	switch {
	case r.TimedOut:
		return Timeout
	case r.StartErr != nil:
		return Error
	}
	reply, isJSON := geminiReply(r)
	text := geminiText(r)
	blocks := isJSON && (reply.Decision == "deny" || reply.Decision == "block" || (reply.Continue != nil && !*reply.Continue))
	if !isJSON && text != "" && r.Exit != 0 && r.Exit != 1 {
		blocks = true
	}
	switch {
	case blocks && !slices.Contains(geminiAdvisoryEvents, event):
		return Block
	case r.Exit != 0:
		return Error
	}
	return Allow
}

// geminiAddsContext reports a JSON reply's additionalContext. Plain
// stdout on exit 0 becomes a systemMessage, which only the user sees.
func geminiAddsContext(r Result) bool {
	reply, ok := geminiReply(r)
	return ok && !r.TimedOut && reply.HookSpecificOutput.AdditionalContext != ""
}

func geminiText(r Result) string {
	if text := strings.TrimSpace(r.Stdout); text != "" {
		return text
	}
	return strings.TrimSpace(r.Stderr)
}

func geminiReply(r Result) (hookReply, bool) {
	var reply hookReply
	text := geminiText(r)
	if !strings.HasPrefix(text, "{") || json.Unmarshal([]byte(text), &reply) != nil {
		return reply, false
	}
	return reply, true
}

// ExpandCommand applies a target's own substitutions to a command before
// its shell sees it. Gemini CLI replaces $GEMINI_PROJECT_DIR,
// $GEMINI_CWD, $GEMINI_SESSION_ID, and $CLAUDE_PROJECT_DIR with the
// quoted value (hookRunner.ts expandCommand); `${...}` forms are left to
// the shell, which finds the same values in the env.
func ExpandCommand(target, goos, command, root string) string {
	if target != "gemini" {
		return command
	}
	quote := posixQuote
	if goos == "windows" {
		quote = powershellQuote
	}
	cwd, session := quote(root), quote(SessionID)
	return strings.NewReplacer(
		"$GEMINI_PROJECT_DIR", cwd,
		"$GEMINI_CWD", cwd,
		"$GEMINI_SESSION_ID", session,
		"$CLAUDE_PROJECT_DIR", cwd,
	).Replace(command)
}

func posixQuote(s string) string {
	if plainWord(s, "_-./:@%+=,") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// powershellQuote mirrors escapeShellArg in shell-utils.ts.
func powershellQuote(s string) string {
	if plainWord(s, "-_.") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// plainWord reports whether s is non-empty and holds only ASCII letters,
// digits, and the runes in extra.
func plainWord(s, extra string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		alnum := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
		if !alnum && !strings.ContainsRune(extra, r) {
			return false
		}
	}
	return true
}
