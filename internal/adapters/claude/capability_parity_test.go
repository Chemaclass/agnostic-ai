package claude

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
// invariant that the claude adapter actually emits something for
// every spec kind it declares in caps.Supports. A future refactor
// that drops support for, say, KindHook would either need to
// remove the kind from Supports (forcing the warning channel) or
// fix the emit path.
func TestEmit_CapabilityMatrixCoversEveryDeclaredKind(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := New().Emit(kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	paths := testutil.WalkRel(t, dir)
	type expect struct {
		kind     spec.Kind
		matchers []string
	}
	cases := []expect{
		{spec.KindAgent, []string{".claude/agents/alpha.md", ".claude/agents/beta.md", ".claude/agents/gamma.md"}},
		{spec.KindSkill, []string{".claude/skills/uno/SKILL.md", ".claude/skills/dos/SKILL.md", ".claude/skills/tres/SKILL.md"}},
		{spec.KindRule, []string{".claude/rules/r1.md", ".claude/rules/r2.md", ".claude/rules/r3.md"}},
		{spec.KindHook, []string{".claude/settings.json"}},
		{spec.KindMCP, []string{".mcp.json"}},
		{spec.KindCommand, []string{".claude/commands/cmd-one.md", ".claude/commands/cmd-two.md", ".claude/commands/cmd-three.md"}},
		{spec.KindSettings, []string{".claude/settings.json"}},
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

// TestEmit_NoCapabilityWarningsForKitSinkBundle asserts that
// emitting every declared kind does not buffer any "unsupported"
// warning. The claude adapter declares every spec kind, so the
// expected warning count is exactly zero.
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
