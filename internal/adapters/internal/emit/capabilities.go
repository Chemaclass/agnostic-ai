package emit

import (
	"fmt"
	"strings"
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

// ReportUnsupported records unsupported (target, kind) pairs for later
// flushing, or returns an error immediately when mode is "error".
//
// Warnings are buffered so a sync over many adapters can group them by
// kind in a single line. Call FlushCapabilityWarnings at the end of a
// sync run to render the buffered output and clear state.
func ReportUnsupported(c Capabilities, b spec.Bundle, mode string) error {
	if mode == "" {
		mode = OnUnsupportedWarn
	}
	for _, k := range spec.AllKinds {
		if c.supports(k) || !b.Has(k) {
			continue
		}
		count := countKind(b, k)
		switch mode {
		case OnUnsupportedError:
			msg := fmt.Sprintf("  ! %s: %d %s skipped (target does not support %s)",
				c.Target, count, pluralizeKind(k, count), k)
			return errs.Coded(errs.CodeUnsupportedKind, "%s", msg)
		case OnUnsupportedSilent:
			continue
		default: // warn
			capabilityWarnState.mu.Lock()
			capabilityWarnState.pending = append(capabilityWarnState.pending, pendingWarn{
				target: c.Target, kind: k, count: count,
			})
			capabilityWarnState.mu.Unlock()
		}
	}
	return nil
}

type pendingWarn struct {
	target string
	kind   spec.Kind
	count  int
}

var capabilityWarnState struct {
	mu      sync.Mutex
	pending []pendingWarn
}

// FlushCapabilityWarnings prints one line per (kind, count) group across
// all buffered targets and clears the buffer. Safe to call when empty.
func FlushCapabilityWarnings() {
	capabilityWarnState.mu.Lock()
	defer capabilityWarnState.mu.Unlock()
	if len(capabilityWarnState.pending) == 0 {
		return
	}
	type key struct {
		k spec.Kind
		n int
	}
	order := []key{}
	groups := map[key][]string{}
	seen := map[string]bool{} // target+kind dedup within one flush
	for _, p := range capabilityWarnState.pending {
		dedupKey := p.target + "\x00" + string(p.kind)
		if seen[dedupKey] {
			continue
		}
		seen[dedupKey] = true
		k := key{p.kind, p.count}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], p.target)
	}
	for _, k := range order {
		targets := groups[k]
		_, _ = fmt.Fprintf(Warner, "  ! %d %s unsupported by %s\n",
			k.n, pluralizeKind(k.k, k.n), strings.Join(targets, ", "))
	}
	_, _ = fmt.Fprintln(Warner, "    fix: set `on-unsupported: silent` in agnostic-ai.yaml to hide these")
	capabilityWarnState.pending = nil
}

// ResetCapabilityWarnings clears buffered warnings without printing.
// Used by tests and by `sync --watch` between runs.
func ResetCapabilityWarnings() {
	capabilityWarnState.mu.Lock()
	capabilityWarnState.pending = nil
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
