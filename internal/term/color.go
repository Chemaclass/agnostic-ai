// Package term offers minimal terminal helpers shared across the binary:
// ANSI color detection and symbol wrappers. No third-party deps.
package term

import (
	"io"
	"os"
)

// ANSI color codes used by Tick / Bang / Cross helpers.
const (
	Green  = "\x1b[32m"
	Yellow = "\x1b[33m"
	Red    = "\x1b[31m"
	Reset  = "\x1b[0m"
)

// ColorEnabled reports whether ANSI sequences should be emitted to w.
// True only when w refers to a character device (tty) and NO_COLOR is
// unset. https://no-color.org governs the env opt-out.
func ColorEnabled(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// Colorize wraps s in code when w is a color-capable terminal.
func Colorize(w io.Writer, s, code string) string {
	if !ColorEnabled(w) {
		return s
	}
	return code + s + Reset
}

// Tick returns "✓" colored green on tty, plain otherwise.
func Tick(w io.Writer) string { return Colorize(w, "✓", Green) }

// Bang returns "!" colored yellow on tty, plain otherwise.
func Bang(w io.Writer) string { return Colorize(w, "!", Yellow) }

// Cross returns "✗" colored red on tty, plain otherwise.
func Cross(w io.Writer) string { return Colorize(w, "✗", Red) }
