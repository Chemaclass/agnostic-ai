package cli

import "github.com/chemaclass/agnostic-ai/internal/errs"

// ErrorText renders an error and its next step for the terminal.
func ErrorText(err error) string {
	text := err.Error()
	if errs.CodeOf(err) == errs.CodeRequiresUnmet {
		return text
	}
	if entry, ok := errs.Lookup(errs.CodeOf(err)); ok && entry.Fix != "" {
		text += "\n  fix: " + entry.Fix
	}
	return text
}
