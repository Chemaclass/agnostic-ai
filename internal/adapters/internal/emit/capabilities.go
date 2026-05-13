package emit

import (
	"errors"
	"fmt"

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
func ReportUnsupported(c Capabilities, b spec.Bundle, mode string) error {
	if mode == "" {
		mode = OnUnsupportedWarn
	}
	for _, k := range spec.AllKinds {
		if c.supports(k) || !b.Has(k) {
			continue
		}
		msg := fmt.Sprintf("  ! %s: %s not supported, skipped", c.Target, k)
		switch mode {
		case OnUnsupportedError:
			return errors.New(msg)
		case OnUnsupportedSilent:
			continue
		default: // warn
			_, _ = fmt.Fprintln(Warner, msg)
		}
	}
	return nil
}
