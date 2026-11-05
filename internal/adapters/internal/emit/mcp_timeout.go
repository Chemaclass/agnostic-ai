package emit

import (
	"fmt"
	"math"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// MCPTimeoutSeconds returns the MCP timeout for a target that reads
// seconds. `x-<target>.timeout` is already in the target's unit and wins.
// The portable `timeout` is milliseconds. fractional marks a target that
// accepts fractions of a second, which gets the exact value; otherwise
// it is rounded up so a short limit never becomes 0, which such targets
// read as their default. A value the target cannot read writes no key
// and prints a note.
func MCPTimeoutSeconds(target string, e spec.Entry, fractional bool) (any, bool) {
	if custom, ok := e.Meta[XPrefix+target].(map[string]any); ok {
		if raw, set := custom["timeout"]; set {
			if seconds, isFloat := raw.(float64); fractional && isFloat && seconds > 0 {
				return seconds, true
			}
			seconds, whole := wholeNumber(raw)
			if !whole {
				NoteProject(fmt.Sprintf("%s: MCP %s x-%s.timeout %v is not a number of seconds %s reads, so no timeout is written", target, e.Name, target, raw, target))
			}
			return seconds, whole
		}
	}
	raw, set := e.Meta["timeout"]
	if !set || raw == nil {
		return 0, false
	}
	ms, whole := wholeNumber(raw)
	if !whole {
		NoteProject(fmt.Sprintf("%s: MCP %s timeout %v is not a whole number of milliseconds, so no timeout is written", target, e.Name, raw))
		return 0, false
	}
	if ms <= 0 {
		return 0, false
	}
	if fractional && ms%1000 != 0 {
		return float64(ms) / 1000, true
	}
	seconds := (ms + 999) / 1000
	if seconds*1000 != ms {
		NoteProject(fmt.Sprintf("%s: MCP %s timeout %d ms is written as %d s, since %s reads whole seconds", target, e.Name, ms, seconds, target))
	}
	return seconds, true
}

func wholeNumber(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case uint64:
		return int(n), true
	case float64:
		if n == math.Trunc(n) && !math.IsInf(n, 0) {
			return int(n), true
		}
	}
	return 0, false
}
