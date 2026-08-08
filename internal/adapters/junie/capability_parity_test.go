package junie

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestEmit_CapabilityMatrixCoversEveryDeclaredKind enforces the
// invariant that the junie adapter actually emits something for
// every spec kind it declares in caps.Supports. A future refactor
// that drops support for, say, KindSkill would either need to
// remove the kind from Supports (forcing the warning channel) or
// fix the emit path.
//
// Rules and agents are only observable in .junie/AGENTS.md content
// (#552): both inline there, not as a distinct per-kind path, so this
// checks inBody rather than a matcher path for those two kinds. A path
// check alone would pass even if the content were never inlined, since
// .junie/AGENTS.md always exists: exactly the failure mode #552 shipped
// with a green suite.
func TestEmit_CapabilityMatrixCoversEveryDeclaredKind(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	paths := testutil.WalkRel(t, dir)
	entryBody, _ := os.ReadFile(filepath.Join(dir, ".junie/AGENTS.md"))
	body := string(entryBody)

	type expect struct {
		kind     spec.Kind
		matchers []string
		inBody   []string
	}
	cases := []expect{
		{kind: spec.KindRule, inBody: []string{"### r1", "### r2", "### r3"}},
		{kind: spec.KindAgent, inBody: []string{"### alpha", "### beta", "### gamma"}},
		{kind: spec.KindSkill, matchers: []string{".junie/skills/uno/SKILL.md", ".junie/skills/dos/SKILL.md", ".junie/skills/tres/SKILL.md"}},
		{kind: spec.KindMCP, matchers: []string{".junie/mcp/mcp.json"}},
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
			for _, s := range c.inBody {
				if strings.Contains(body, s) {
					found = true
					break
				}
			}
		}
		if !found {
			t.Errorf("declared kind %q in caps.Supports has no observable output (paths: %v, .junie/AGENTS.md: %s)", k, paths, body)
		}
	}
}

// TestEmit_NoCapabilityWarningsForKitSinkBundle asserts that
// emitting every declared kind does not buffer any "unsupported"
// warning.
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
// every kind junie does not declare in caps.Supports (Hook, Command).
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
