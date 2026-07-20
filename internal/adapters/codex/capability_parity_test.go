package codex

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestEmit_CapabilityMatrixCoversEveryDeclaredKind enforces the
// invariant that the codex adapter actually emits something for every
// spec kind it declares in caps.Supports. A future refactor that drops
// support for, say, KindCommand would either need to remove the kind
// from Supports (forcing the warning channel) or fix the emit path.
//
// "Something" is intentionally loose: rules are routed through the
// entry-point (sync owns AGENTS.md) or the legacy rules-file opt-in;
// hooks land in .codex/hooks.json; MCPs in .codex/config.toml. The
// test inspects the set of relative paths actually written and maps
// each declared kind to at least one expected substring or path
// fragment.
func TestEmit_CapabilityMatrixCoversEveryDeclaredKind(t *testing.T) {
	dir := testutil.TempCwd(t)
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			// Opt into the legacy rules-file so KindRule has a visible
			// landing spot inside the adapter's own emit footprint, and
			// into commands-dir since project prompts are opt-in now.
			"codex": {RulesFile: "AGENTS-rules.md", CommandsDir: ".codex/prompts"},
		},
	}
	if err := New().Emit(kitSinkBundle(), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	paths := testutil.WalkRel(t, dir)

	type expect struct {
		kind     spec.Kind
		matchers []string // satisfied when any matcher is a substring of any emitted path
	}
	cases := []expect{
		{spec.KindAgent, []string{".codex/agents/alpha.toml", ".codex/agents/beta.toml", ".codex/agents/gamma.toml"}},
		{spec.KindSkill, []string{".agents/skills/uno/SKILL.md", ".agents/skills/dos/SKILL.md", ".agents/skills/tres/SKILL.md"}},
		{spec.KindRule, []string{"AGENTS-rules.md"}},
		{spec.KindHook, []string{".codex/hooks.json"}},
		{spec.KindMCP, []string{".codex/config.toml"}},
		{spec.KindCommand, []string{".codex/prompts/cmd-one.md", ".codex/prompts/cmd-two.md", ".codex/prompts/cmd-three.md"}},
	}

	for _, k := range caps.Supports {
		found := false
		for _, c := range cases {
			if c.kind != k {
				continue
			}
			for _, m := range c.matchers {
				if pathSetContains(paths, m) {
					found = true
					break
				}
			}
		}
		if !found {
			t.Errorf("declared kind %q in caps.Supports has no observable output (paths: %v)", k, paths)
		}
	}
}

// TestEmit_NoCapabilityWarningsForKitSinkBundle asserts that emitting
// every declared kind does not buffer any "unsupported" warning. If
// the adapter starts silently dropping a kind without removing it
// from Supports, the warning channel would stay empty (wrong) AND
// ReportUnsupported would too; this test catches the regression by
// checking the warning buffer.
func TestEmit_NoCapabilityWarningsForKitSinkBundle(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)

	if err := New().Emit(kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if got := emit.PendingCapabilityWarningsCount(); got != 0 {
		t.Errorf("expected no capability warnings for a kit-sink bundle, got %d", got)
	}
}

func pathSetContains(paths []string, needle string) bool {
	needle = filepath.ToSlash(needle)
	for _, p := range paths {
		if strings.Contains(p, needle) {
			return true
		}
	}
	return false
}
