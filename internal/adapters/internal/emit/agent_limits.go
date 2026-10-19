package emit

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// AgentLimit is a portable agent field, such as `maxTurns`, that a
// target writes under its own key, such as Gemini's `max_turns`.
type AgentLimit struct {
	Portable, Native string
	// TopLevelNative says the target also reads Native at the top level
	// of a spec, as Gemini does `max_turns`.
	TopLevelNative bool
}

// The portable agent fields an AgentLimit carries.
const (
	AgentMaxTurns   = "maxTurns"
	AgentBackground = "background"
)

// Value returns what target writes under Native for agent a, first
// match wins: `x-<target>.<Native>` as written, a valid
// `x-<target>.<Portable>`, the top-level Native when the target reads
// it, then a valid top-level Portable. An override of the wrong type is
// skipped with a note, so the shared value still applies.
func (l AgentLimit) Value(a spec.Entry, target string) (any, bool) {
	custom, _ := a.Meta[XPrefix+target].(map[string]any)
	if v, set := custom[l.Native]; set && v != nil {
		return v, true
	}
	if v, set := custom[l.Portable]; set {
		if valid, ok := l.valid(v); ok {
			return valid, true
		}
		if v != nil {
			NoteProject(fmt.Sprintf("%s: %s%s.%s on agent %s is %s, so sync wrote the shared %s",
				target, XPrefix, target, l.Portable, a.Name, l.expected(), l.Portable))
		} else {
			return nil, false
		}
	}
	if l.TopLevelNative {
		if v, set := a.Meta[l.Native]; set && v != nil {
			return v, true
		}
	}
	return l.valid(a.Meta[l.Portable])
}

func (l AgentLimit) valid(v any) (any, bool) {
	if l.Portable == AgentBackground {
		return spec.AgentBackground(v)
	}
	return spec.AgentMaxTurns(v)
}

func (l AgentLimit) expected() string {
	if l.Portable == AgentBackground {
		return "not true or false"
	}
	return "not a whole number of 1 or more"
}
