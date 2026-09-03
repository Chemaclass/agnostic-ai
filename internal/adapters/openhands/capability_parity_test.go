package openhands

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
// invariant that the openhands adapter actually emits something for
// every spec kind it declares in caps.Supports, with one deliberate
// exception: KindRule has no per-adapter output at all. Rules reach
// OpenHands exclusively through the shared AGENTS.md entry-point that
// `sync` writes centrally (see openhands.go), a write this per-package
// test never observes because it calls Adapter.Emit directly.
// Declaring KindRule keeps the "unsupported" warning honest (rules do
// reach OpenHands) without this adapter ever touching a rules file
// itself.
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
		{spec.KindSkill, []string{".agents/skills/uno/SKILL.md", ".agents/skills/dos/SKILL.md", ".agents/skills/tres/SKILL.md"}},
		{spec.KindMCP, []string{"config.toml"}},
		{spec.KindEnvironment, []string{".openhands/setup.sh"}},
	}
	for _, k := range caps.Supports {
		if k == spec.KindRule {
			continue // delivered by sync's shared entry-point, not this adapter
		}
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
// every declared kind does not buffer any "unsupported" warning.
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
// every kind openhands does not declare in caps.Supports (Agent, Hook,
// Command, Settings). A future caps.Supports expansion needs to
// delete the matching row here and demonstrate the emit path that
// backs the new claim.
func TestEmit_UnsupportedKindsWarn(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "helper", Path: "agents/helper.md", Body: "helper body"},
		{Kind: spec.KindHook, Name: "fmt-go", Meta: map[string]any{"event": "PostToolUse", "command": "gofmt -w"}},
		{Kind: spec.KindCommand, Name: "cmd-one", Path: "commands/cmd-one.md", Body: "cmd body"},
		{Kind: spec.KindSettings, Name: "perms", Path: "settings/perms.yaml", Meta: map[string]any{"model": "opus"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{OnUnsupported: "warn"}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if got := emit.PendingCapabilityWarningsCount(); got != 4 {
		t.Errorf("expected 4 capability warnings (agent/hook/command/settings), got %d", got)
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
