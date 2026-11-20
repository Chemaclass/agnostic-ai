package cli

import (
	"errors"
	"os"

	"github.com/chemaclass/agnostic-ai/internal/config"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// ErrorText renders an error and its next step for the terminal.
func ErrorText(err error) string {
	text := err.Error()
	if errs.CodeOf(err) == errs.CodeRequiresUnmet {
		return text
	}
	if dir, ok := projectAboveMissingConfig(err); ok {
		return text + "\n  fix: this directory is inside the agnostic-ai project at " + dir + "; run the command from that folder."
	}
	if entry, ok := errs.Lookup(errs.CodeOf(err)); ok && entry.Fix != "" {
		text += "\n  fix: " + entry.Fix
	}
	return text
}

// projectAboveMissingConfig returns the enclosing project when err says
// the working directory has no config, so the hint points there and not
// at init, which would start a second project. A missing config in any
// other folder, such as a --against tree, keeps the registry fix.
func projectAboveMissingConfig(err error) (string, bool) {
	var missing *config.MissingError
	if errs.CodeOf(err) != errs.CodeConfigMissing || !errors.As(err, &missing) {
		return "", false
	}
	cwd, werr := os.Getwd()
	if werr != nil || !sameDir(missing.Root, cwd) {
		return "", false
	}
	return enclosingProject(cwd)
}
