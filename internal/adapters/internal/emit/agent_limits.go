package emit

import (
	"fmt"
	"maps"

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
func (l AgentLimit) Value(a spec.Entry, target string) (value any, ok bool) {
	custom, _ := a.Meta[XPrefix+target].(map[string]any)
	if v, set := custom[l.Native]; set && v != nil && l.Native != l.Portable {
		return v, true
	}
	if v, set := custom[l.Portable]; set {
		if v == nil {
			return nil, false
		}
		if valid, ok := l.valid(v); ok {
			return valid, true
		}
		defer func() { l.noteInvalid(a, target, ok) }()
	}
	if l.TopLevelNative {
		if v, set := a.Meta[l.Native]; set && v != nil {
			return v, true
		}
	}
	return l.valid(a.Meta[l.Portable])
}

// WithValidAgentLimits returns a with each `x-<target>` override of
// limits that has the wrong type left out, with a note, so a target
// that writes the portable key as is gets the shared value instead.
func WithValidAgentLimits(a spec.Entry, target string, limits ...string) spec.Entry {
	custom, _ := a.Meta[XPrefix+target].(map[string]any)
	var drop []string
	for _, field := range limits {
		if v, set := custom[field]; set && v != nil {
			if _, ok := (AgentLimit{Portable: field}).valid(v); !ok {
				drop = append(drop, field)
			}
		}
	}
	if len(drop) == 0 {
		return a
	}
	meta := maps.Clone(a.Meta)
	custom = maps.Clone(custom)
	for _, field := range drop {
		delete(custom, field)
	}
	meta[XPrefix+target] = custom
	out := a
	out.Meta = meta
	for _, field := range drop {
		l := AgentLimit{Portable: field, Native: field}
		_, shared := l.valid(a.Meta[field])
		l.noteInvalid(a, target, shared)
	}
	return out
}

func (l AgentLimit) noteInvalid(a spec.Entry, target string, shared bool) {
	result := "the shared " + l.Portable
	if !shared {
		result = "no turn limit"
		if l.Portable == AgentBackground {
			result = "no background flag"
		}
	}
	NoteProject(fmt.Sprintf("%s: %s%s.%s on agent %s is %s, so sync wrote %s",
		target, XPrefix, target, l.Portable, a.Name, l.expected(), result))
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
