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

// Prints a line that must survive --quiet, such as a kept hand edit or
// orphan: routine output otherwise, but on stderr under --quiet, the way
// errors print, so a hook running --quiet still learns about it.
func keptf(format string, a ...any) {
	if verbosity < levelDefault {
		_, _ = fmt.Fprintf(os.Stderr, format, a...)
		return
	}
	_, _ = fmt.Fprintf(logOut, format, a...)
}

// Status symbols for the active log sink.
func tick() string  { return term.Tick(logOut) }
func cross() string { return term.Cross(logOut) }
func bang() string  { return term.Bang(logOut) }
