package cli

import (
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// declaresSupport reports whether the named target's adapter accepts the
// given kind, determined by behavior rather than by reading the adapter's
// unexported caps value: emit a bundle holding only that kind with
// `on-unsupported: error` and see whether the run rejects it.
//
// Any other error means the adapter accepted the kind and then failed for
// an unrelated reason (missing config, unwritable path), which still
// counts as "declares support".
func declaresSupport(t *testing.T, target string, kind spec.Kind) bool {
	t.Helper()
	testutil.TempCwd(t)

	a, err := adapters.Resolve(target)
	if err != nil {
		t.Fatalf("resolve %s: %v", target, err)
	}
	b := spec.NewBundle([]spec.Entry{sampleEntryForKind(kind)})
	cfg := &config.Config{OnUnsupported: "error"}

	adapters.ResetCapabilityWarnings()
	t.Cleanup(adapters.ResetCapabilityWarnings)

	err = adapters.EmitWithProvenance(adapters.NewSession(), a, b, cfg, true)
	return errs.CodeOf(err) != errs.CodeUnsupportedKind
}

// sampleEntryForKind builds the minimal spec entry an adapter needs to
// route the kind far enough to accept or reject it.
func sampleEntryForKind(kind spec.Kind) spec.Entry {
	e := spec.Entry{Kind: kind, Name: "probe", Body: "probe body"}
	switch kind {
	case spec.KindSkill:
		e.Path = "skills/probe/SKILL.md"
	case spec.KindHook:
		e.Meta = map[string]any{"event": "PostToolUse", "command": "true"}
	case spec.KindMCP:
		e.Meta = map[string]any{"command": "probe-server"}
	case spec.KindCommand:
		e.Path = "commands/probe.md"
	}
	return e
}

// TestNativeCapabilities_MatchDeclaredAdapterSupport enforces that the
// hand-maintained targetsSupportingKind map stays in step with what the
// adapters declare.
//
// The map drives two user-visible behaviors: the orphan-kind validator
// (specs of a kind no enabled target supports are reported as dead
// weight) and incremental `sync --watch` (a spec edit only re-syncs the
// targets mapped to its kind). Nothing enforced the relationship, so
// every adapter that gained a capability had to remember to update a
// second list in another package.
//
// It drifted three separate times during the 2026-08-01 target audit:
// antigravity was missing for skills, factory/qoder/openhands for MCP,
// and augment for agents and skills. Each one silently told users their
// specs were dead weight and skipped those targets on watch re-sync.
func TestNativeCapabilities_MatchDeclaredAdapterSupport(t *testing.T) {
	for _, target := range adapters.Names() {
		for _, kind := range spec.AllKinds {
			if !declaresSupport(t, target, kind) {
				continue
			}
			if _, ok := targetsSupportingKind[kind][target]; !ok {
				t.Errorf("adapter %q declares support for %q but targetsSupportingKind[%q] omits it; "+
					"add it there or the orphan-kind validator will call those specs dead weight "+
					"and sync --watch will skip the target", target, kind, kind)
			}
		}
	}
}

// TestNativeCapabilities_NameOnlyRegisteredTargets catches the reverse
// drift: an entry left behind after a target is renamed or dropped, which
// would keep the validator quiet about a kind nothing emits.
func TestNativeCapabilities_NameOnlyRegisteredTargets(t *testing.T) {
	registered := make(map[string]bool, len(adapters.Names()))
	for _, name := range adapters.Names() {
		registered[name] = true
	}
	for kind, targets := range targetsSupportingKind {
		for target := range targets {
			if !registered[target] {
				t.Errorf("targetsSupportingKind[%q] lists %q, which is not a registered target", kind, target)
			}
		}
	}
}

// TestNativeCapabilities_ListNoKindAnAdapterDropped closes the other half
// of the parity invariant.
//
// TestNativeCapabilities_MatchDeclaredAdapterSupport catches a target that
// gains a capability without being added here. It cannot catch the
// reverse: a target that stops supporting a kind and is left in the map.
//
// That gap is not hypothetical. When amp's `Command` support was removed
// (Amp has no file-based command surface; commands register through its
// plugin API), `targetsSupportingKind[spec.KindCommand]` still listed amp.
// Nothing failed. The orphan-kind validator would have kept telling users
// their command specs were fine for amp, and `sync --watch` would have
// kept re-syncing amp on a command edit that produces nothing. A human
// caught it while reading the diff.
//
// Together the two tests pin the map to exactly what the adapters declare,
// in both directions, so neither adding nor removing a capability can
// leave it stale.
func TestNativeCapabilities_ListNoKindAnAdapterDropped(t *testing.T) {
	for kind, targets := range targetsSupportingKind {
		for target := range targets {
			if _, ok := adapters.Get(target); !ok {
				continue // unregistered targets are the other test's job
			}
			if !declaresSupport(t, target, kind) {
				t.Errorf("targetsSupportingKind[%q] lists %q, but that adapter no longer declares the kind; "+
					"remove it or the orphan-kind validator will call those specs supported "+
					"and sync --watch will re-sync a target that emits nothing for them", kind, target)
			}
		}
	}
}
