package cli

import "github.com/chemaclass/agnostic-ai/internal/errs"

// ErrorText renders err for the terminal. A coded error gets its registry
// fix on a second line, so the message says what to do next without a
// trip to `agnostic-ai explain <code>`.
func ErrorText(err error) string {
	text := err.Error()
	if entry, ok := errs.Lookup(errs.CodeOf(err)); ok && entry.Fix != "" {
		text += "\n  fix: " + entry.Fix
	}
	return text
}
