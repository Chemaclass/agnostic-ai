package jules

import (
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// This package has no "coverage" half of the usual capability-parity
// pair (asserting every caps.Supports kind has an observable output):
// caps.Supports is exactly {KindRule}, and KindRule has zero
// adapter-level output by design (see jules.go and
// TestEmit_NeverWritesAnyFile in jules_test.go) — rules reach Jules
// solely through the shared AGENTS.md entry-point sync writes
// centrally. A "coverage" test would therefore skip its only row and
// assert nothing, so it is omitted; TestEmit_NoCapabilityWarningsForKitSinkBundle
// below covers the other half of the contract.

// kitSinkBundle returns a Bundle exercising every kind the jules
// adapter declares in caps.Supports (Rule only).
func kitSinkBundle() spec.Bundle {
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule 1 body"},
		{Kind: spec.KindRule, Name: "r2", Path: "rules/r2.md", Body: "rule 2 body"},
		{Kind: spec.KindRule, Name: "r3", Path: "rules/r3.md", Body: "rule 3 body"},
	}
	return spec.NewBundle(entries)
}

// TestEmit_NoCapabilityWarningsForKitSinkBundle asserts that a bundle
// containing only declared-supported kinds never buffers an
// "unsupported" warning.
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
// every kind jules does not declare in caps.Supports (Agent, Skill,
// Hook, MCP). A future caps.Supports expansion needs to delete the
// matching row here and demonstrate the emit path that backs the new
// claim — which for Jules would mean a genuinely new native file
// surface, since none exists today.
func TestEmit_UnsupportedKindsWarn(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "helper", Path: "agents/helper.md", Body: "helper body"},
		{Kind: spec.KindSkill, Name: "uno", Path: "skills/uno/SKILL.md", Body: "skill body"},
		{Kind: spec.KindHook, Name: "fmt-go", Meta: map[string]any{"event": "PostToolUse", "command": "gofmt -w"}},
		{Kind: spec.KindMCP, Name: "stdio-server", Meta: map[string]any{"command": "npx"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{OnUnsupported: "warn"}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if got := emit.PendingCapabilityWarningsCount(); got != 4 {
		t.Errorf("expected 4 capability warnings (agent/skill/hook/mcp), got %d", got)
	}
}
