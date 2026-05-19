package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/chemaclass/agnostic-ai/internal/term"
)

const (
	levelQuiet   = -1
	levelDefault = 0
	levelVerbose = 1
)

var (
	verbosity           = levelDefault
	logOut    io.Writer = os.Stdout
)

// Prints a one line command summary. Can be suppressed using --quiet flag
func summaryf(format string, a ...any) {
	if verbosity < levelDefault {
		return
	}
	_, _ = fmt.Fprintf(logOut, format, a...)
}

// Prints detail on target. Needs -v flag
func verbosef(format string, a ...any) {
	if verbosity < levelVerbose {
		return
	}
	_, _ = fmt.Fprintf(logOut, format, a...)
}

// Status symbols for the active log sink.
func tick() string  { return term.Tick(logOut) }
func cross() string { return term.Cross(logOut) }
