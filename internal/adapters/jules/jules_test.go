package jules

import (
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// This package deliberately has no header_coverage_test.go and no
// kitsink_golden_test.go: Jules has no native file surface at all, so
// Emit never writes a file (dry-run or real), leaving no provenance
// header to check and no output tree to snapshot. TestEmit_NeverWritesAnyFile
// below is this adapter's kit-sink coverage: it runs the same
// every-supported-kind bundle every other adapter's golden test runs
// and asserts the one true invariant here — an empty directory.

func TestName(t *testing.T) {
	if got := New().Name(); got != "jules" {
		t.Errorf("Name() = %q, want %q", got, "jules")
	}
}

// TestEmit_NeverWritesAnyFile asserts the adapter's whole contract:
// regardless of bundle shape, dry-run, or config, Emit writes nothing.
// Rules reach Jules only through the shared AGENTS.md entry-point sync
// writes centrally.
func TestEmit_NeverWritesAnyFile(t *testing.T) {
	everyKind := kitSinkBundle()
	everyKind.Agents = []spec.Entry{{Kind: spec.KindAgent, Name: "alpha", Path: "agents/alpha.md", Body: "alpha body"}}
	everyKind.Skills = []spec.Entry{{Kind: spec.KindSkill, Name: "uno", Path: "skills/uno/SKILL.md", Body: "uno skill body"}}
	everyKind.MCPs = []spec.Entry{{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}}}
	everyKind.Hooks = []spec.Entry{{Kind: spec.KindHook, Name: "fmt-go", Meta: map[string]any{"event": "PostToolUse", "command": "gofmt -w"}}}

	for _, tc := range []struct {
		name   string
		bundle spec.Bundle
		dryRun bool
	}{
		{name: "empty bundle", bundle: spec.NewBundle(nil)},
		{name: "rule-only kit-sink bundle", bundle: kitSinkBundle()},
		{name: "every declared and undeclared kind", bundle: everyKind},
		{name: "dry-run", bundle: kitSinkBundle(), dryRun: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			cfg := &config.Config{OnUnsupported: emit.OnUnsupportedSilent}
			if err := New().Emit(emit.NewSession(), tc.bundle, cfg, tc.dryRun); err != nil {
				t.Fatalf("emit: %v", err)
			}
			paths := testutil.WalkRel(t, dir)
			if len(paths) != 0 {
				t.Errorf("expected no files written, got %v", paths)
			}
		})
	}
}
