package windsurf

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_CapabilityMatrixCoversEveryDeclaredKind(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	paths := testutil.WalkRel(t, dir)
	type expect struct {
		kind     spec.Kind
		matchers []string
	}
	cases := []expect{
		{spec.KindRule, []string{".devin/rules/r1.md", ".devin/rules/r2.md", ".devin/rules/r3.md", "backend/.devin/rules/r4.md"}},
		{spec.KindAgent, []string{".devin/rules/agent-alpha.md", ".devin/rules/agent-beta.md", ".devin/rules/agent-gamma.md", "backend/.devin/rules/agent-delta.md"}},
		{spec.KindSkill, []string{".agents/skills/uno/SKILL.md", ".agents/skills/dos/SKILL.md", ".agents/skills/tres/SKILL.md"}},
		{spec.KindIgnore, []string{".devinignore"}},
		{spec.KindMCP, []string{".devin/mcp_config.json"}},
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

func TestEmit_NoCapabilityWarningsForKitSinkBundle(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)

	if err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if got := emit.PendingCapabilityWarningsCount(); got != 0 {
		t.Errorf("expected no capability warnings, got %d", got)
	}
}

// TestEmit_UnsupportedKindsWarn asserts ReportUnsupported fires for
// every kind windsurf does not declare in caps.Supports (Hook,
// Command). MCP (`.devin/mcp_config.json`) is now native, so it must
// not warn: a future caps.Supports expansion needs to delete the
// matching row here and demonstrate the emit path that backs it.
func TestEmit_UnsupportedKindsWarn(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "fmt-go", Meta: map[string]any{"event": "PostToolUse", "command": "gofmt -w"}},
		{Kind: spec.KindCommand, Name: "cmd-one", Path: "commands/cmd-one.md", Body: "cmd body"},
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
