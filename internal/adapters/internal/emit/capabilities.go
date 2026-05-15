package emit

import (
	"fmt"
	"sync"

	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Capabilities declares which spec kinds an adapter supports natively.
type Capabilities struct {
	// Target is the adapter name used in user-facing messages.
	Target string
	// Supports lists the spec kinds this adapter emits natively.
	Supports []spec.Kind
}

// supports reports whether the adapter declares native support for k.
func (c Capabilities) supports(k spec.Kind) bool {
	for _, s := range c.Supports {
		if s == k {
			return true
		}
	}
	return false
}

// Unsupported policies. Mirrors agnostic-ai.yaml `on-unsupported`.
const (
	OnUnsupportedWarn   = "warn"
	OnUnsupportedError  = "error"
	OnUnsupportedSilent = "silent"
)

// ReportUnsupported logs or returns an error for any spec kind in b that
// the adapter does not support natively.
//
// mode controls behavior; an empty mode falls back to OnUnsupportedWarn.
// Warnings are emitted at most once per (target, kind) tuple per process,
// include the number of specs that would have been skipped, and end with
// a one-line suppression hint pointing at `on-unsupported: silent`. The
// dedup map is reset by ResetCapabilityWarnings — `sync --watch` and the
// test suite call this between runs.
func ReportUnsupported(c Capabilities, b spec.Bundle, mode string) error {
	if mode == "" {
		mode = OnUnsupportedWarn
	}
	for _, k := range spec.AllKinds {
		if c.supports(k) || !b.Has(k) {
			continue
		}
		count := countKind(b, k)
		msg := fmt.Sprintf("  ! %s: %d %s skipped (target does not support %s)",
			c.Target, count, pluralizeKind(k, count), k)
		switch mode {
		case OnUnsupportedError:
			return errs.Coded(errs.CodeUnsupportedKind, "%s", msg)
		case OnUnsupportedSilent:
			continue
		default: // warn
			if alreadyWarned(c.Target, k) {
				continue
			}
			_, _ = fmt.Fprintln(Warner, msg)
		}
	}
	if mode == OnUnsupportedWarn {
		printSuppressionHintOnce()
	}
	return nil
}

// capabilityWarnState dedupes per-process and prints the suppression hint
// at most once per process. sync --watch and tests reset via
// ResetCapabilityWarnings between runs.
var capabilityWarnState struct {
	mu       sync.Mutex
	seen     map[string]bool
	hintDone bool
}

func alreadyWarned(target string, k spec.Kind) bool {
	key := target + "\x00" + string(k)
	capabilityWarnState.mu.Lock()
	defer capabilityWarnState.mu.Unlock()
	if capabilityWarnState.seen == nil {
		capabilityWarnState.seen = map[string]bool{}
	}
	if capabilityWarnState.seen[key] {
		return true
	}
	capabilityWarnState.seen[key] = true
	return false
}

func printSuppressionHintOnce() {
	capabilityWarnState.mu.Lock()
	defer capabilityWarnState.mu.Unlock()
	if capabilityWarnState.hintDone || len(capabilityWarnState.seen) == 0 {
		return
	}
	capabilityWarnState.hintDone = true
	_, _ = fmt.Fprintln(Warner, "    fix: set `on-unsupported: silent` in agnostic-ai.yaml to hide these")
}

// ResetCapabilityWarnings clears the per-process dedup state. Used by
// sync --watch between runs and by tests that assert on warning output.
func ResetCapabilityWarnings() {
	capabilityWarnState.mu.Lock()
	capabilityWarnState.seen = nil
	capabilityWarnState.hintDone = false
	capabilityWarnState.mu.Unlock()
}

func countKind(b spec.Bundle, k spec.Kind) int {
	switch k {
	case spec.KindAgent:
		return len(b.Agents)
	case spec.KindSkill:
		return len(b.Skills)
	case spec.KindRule:
		return len(b.Rules)
	case spec.KindHook:
		return len(b.Hooks)
	case spec.KindMCP:
		return len(b.MCPs)
	case spec.KindCommand:
		return len(b.Commands)
	}
	return 0
}

func pluralizeKind(k spec.Kind, n int) string {
	if n == 1 {
		return string(k)
	}
	return string(k) + "s"
}
