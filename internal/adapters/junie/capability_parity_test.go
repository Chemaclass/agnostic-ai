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
// Rules are only observable in .junie/AGENTS.md content (#552): they
// inline there, not as a distinct per-kind path, so this checks inBody
// rather than a matcher path for that one kind. A path check alone
// would pass even if the content were never inlined, since
// .junie/AGENTS.md always exists: exactly the failure mode #552 shipped
// with a green suite. Agents no longer share that risk (#604): they
// emit to their own native `.junie/agents/<name>.md` file, so a
// matcher path is the right check and the surer one: .junie/AGENTS.md
// no longer carries agent content to inspect at all.
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
		{kind: spec.KindAgent, matchers: []string{".junie/agents/alpha.md", ".junie/agents/beta.md", ".junie/agents/gamma.md"}},
		{kind: spec.KindSkill, matchers: []string{".junie/skills/uno/SKILL.md", ".junie/skills/dos/SKILL.md", ".junie/skills/tres/SKILL.md"}},
		{kind: spec.KindMCP, matchers: []string{".junie/mcp/mcp.json"}},
		{kind: spec.KindCommand, matchers: []string{".junie/commands/cmd-one.md", ".junie/commands/cmd-two.md", ".junie/commands/cmd-three.md"}},
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
// every kind junie does not declare in caps.Supports (Hook only, since
// #605 moved Command into caps.Supports).
func TestEmit_UnsupportedKindsWarn(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "fmt-go", Meta: map[string]any{"event": "PostToolUse", "command": "gofmt -w"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{OnUnsupported: "warn"}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if got := emit.PendingCapabilityWarningsCount(); got != 1 {
		t.Errorf("expected 1 capability warning (hook), got %d", got)
	}
}

// TestEmit_CommandNoLongerWarns confirms a Command spec targeting
// junie reaches its native `.junie/commands/<name>.md` file instead of
// tripping ReportUnsupported (#605).
func TestEmit_CommandNoLongerWarns(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)

	entries := []spec.Entry{
		{Kind: spec.KindCommand, Name: "cmd-one", Path: "commands/cmd-one.md", Body: "cmd body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{OnUnsupported: "warn"}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if got := emit.PendingCapabilityWarningsCount(); got != 0 {
		t.Errorf("expected no capability warnings once Command is supported, got %d", got)
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
