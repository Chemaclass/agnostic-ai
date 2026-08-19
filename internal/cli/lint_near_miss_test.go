package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// A near-miss of a key the tool owns is not an extension, it is a typo
// that silently disarms a setting. Nine phel-lang agents ran with full
// tool access because `allowed_tools:` parsed, emitted, and did
// nothing (#617).
func TestLintNearMissKeys_FlagsAllowedTools(t *testing.T) {
	entries := []spec.Entry{{
		Kind: spec.KindAgent, Name: "a", Path: "agents/a.md",
		Meta: map[string]any{"allowed_tools": []any{"Read"}}, Body: "b",
	}}

	findings := lintNearMissKeys(entries)

	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d: %v", len(findings), findings)
	}
	if !strings.Contains(findings[0].Message, "tools") {
		t.Errorf("message must name the key it meant: %q", findings[0].Message)
	}
	if findings[0].Severity != lintWarn {
		t.Errorf("expected warn so --strict gates it, got %v", findings[0].Severity)
	}
}

func TestLintNearMissKeys_FlagsEveryDocumentedNearMiss(t *testing.T) {
	for _, key := range []string{"allowed_tools", "allowedTools", "disallowed_tools", "max_turns", "model_name"} {
		entries := []spec.Entry{{
			Kind: spec.KindAgent, Name: "a", Path: "agents/a.md",
			Meta: map[string]any{key: "x"}, Body: "b",
		}}
		if got := lintNearMissKeys(entries); len(got) != 1 {
			t.Errorf("%s: expected a finding, got %v", key, got)
		}
	}
}

// The real key must stay silent, or the rule punishes correct specs.
func TestLintNearMissKeys_IgnoresCorrectKeys(t *testing.T) {
	entries := []spec.Entry{{
		Kind: spec.KindAgent, Name: "a", Path: "agents/a.md",
		Meta: map[string]any{"tools": []any{"Read"}, "model": "opus", "description": "d"}, Body: "b",
	}}

	if got := lintNearMissKeys(entries); len(got) != 0 {
		t.Errorf("correct keys must not be flagged, got %v", got)
	}
}

// Passthrough is the right default for genuine extensions. A key under
// x-<target> is deliberate, including target-native spellings like
// Junie's real disallowedTools.
func TestLintNearMissKeys_IgnoresTargetNamespacedKeys(t *testing.T) {
	entries := []spec.Entry{{
		Kind: spec.KindAgent, Name: "a", Path: "agents/a.md",
		Meta: map[string]any{"x-junie": map[string]any{"disallowedTools": []any{"Bash"}}}, Body: "b",
	}}

	if got := lintNearMissKeys(entries); len(got) != 0 {
		t.Errorf("x-<target> keys are deliberate, got %v", got)
	}
}

// An unrelated custom key is a genuine extension, not a near miss.
func TestLintNearMissKeys_IgnoresUnrelatedKeys(t *testing.T) {
	entries := []spec.Entry{{
		Kind: spec.KindAgent, Name: "a", Path: "agents/a.md",
		Meta: map[string]any{"owner": "platform-team"}, Body: "b",
	}}

	if got := lintNearMissKeys(entries); len(got) != 0 {
		t.Errorf("unknown keys pass through by design, got %v", got)
	}
}

// The rule has to be wired in, not merely defined.
func TestCollectLintFindings_IncludesNearMissKeys(t *testing.T) {
	b := spec.NewBundle([]spec.Entry{{
		Kind: spec.KindAgent, Name: "a", Path: "agents/a.md",
		Meta: map[string]any{"allowed_tools": []any{"Read"}}, Body: "b",
	}})

	var found bool
	for _, f := range collectLintFindings([]string{"claude"}, b) {
		if strings.Contains(f.Message, "allowed_tools") {
			found = true
		}
	}
	if !found {
		t.Error("lint must report the near-miss key; a rule nobody calls fixes nothing")
	}
}
