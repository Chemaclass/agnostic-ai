package antigravity

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
// invariant that the antigravity adapter actually emits something
// for every spec kind it declares in caps.Supports. A future
// refactor that drops support for, say, KindAgent would either need
// to remove the kind from Supports (forcing the warning channel) or
// fix the emit path.
func TestEmit_CapabilityMatrixCoversEveryDeclaredKind(t *testing.T) {
	dir := testutil.TempCwd(t)
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"antigravity": {RulesFile: ".agent/AGENTS-rules.md"},
		},
	}
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	paths := testutil.WalkRel(t, dir)
	type expect struct {
		kind     spec.Kind
		matchers []string
	}
	cases := []expect{
		{spec.KindRule, []string{".agents/rules/r1.md", ".agents/rules/r2.md", ".agents/rules/r3.md"}},
		{spec.KindAgent, []string{".agents/agents/alpha.md", ".agents/agents/beta.md", ".agents/agents/gamma.md"}},
		{spec.KindSkill, []string{".agents/skills/s1/SKILL.md", ".agents/skills/s2/SKILL.md", ".agents/skills/s3/SKILL.md"}},
		{spec.KindMCP, []string{".agents/mcp_config.json"}},
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
// warning. If the adapter starts silently dropping a kind without
// removing it from Supports, the warning channel would stay empty
// (wrong) AND ReportUnsupported would too; this test catches the
// regression by checking the warning buffer.
func TestEmit_NoCapabilityWarningsForKitSinkBundle(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)

	if err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if got := emit.PendingCapabilityWarningsCount(); got != 0 {
		t.Errorf("expected no capability warnings for a kit-sink bundle, got %d", got)
	}
}

// TestEmit_UnsupportedKindsWarn asserts ReportUnsupported fires for
// every kind antigravity does not declare in caps.Supports (Hook,
// Command). Skill (`.agents/skills/<name>/SKILL.md`) and MCP
// (`.agents/mcp_config.json`) are both native, so neither must warn. A
// future caps.Supports expansion needs to delete the matching row here
// and demonstrate the emit path that backs it.
func TestEmit_UnsupportedKindsWarn(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)

	entries := []spec.Entry{
		{Kind: spec.KindSkill, Name: "uno", Path: "skills/uno/SKILL.md", Body: "skill body"},
		{Kind: spec.KindHook, Name: "fmt-go", Meta: map[string]any{"event": "PostToolUse", "command": "gofmt -w"}},
		{Kind: spec.KindCommand, Name: "cmd-one", Path: "commands/cmd-one.md", Body: "cmd body"},
		{Kind: spec.KindMCP, Name: "stdio-server", Meta: map[string]any{"command": "npx"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{OnUnsupported: "warn"}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if got := emit.PendingCapabilityWarningsCount(); got != 2 {
		t.Errorf("expected 2 capability warnings (hook/command), got %d", got)
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
