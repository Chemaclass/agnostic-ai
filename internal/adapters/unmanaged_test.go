package adapters

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmitWithProvenance_SkipsUnmanagedPaths(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindAgent, Name: "hand", Body: "hand agent"},
		{Kind: spec.KindAgent, Name: "gen", Body: "generated agent"},
	})
	cfg := &config.Config{Targets: []string{"claude"}}
	cfg.Sync.Unmanaged = []string{".claude/agents/hand.md"}
	a, _ := Get("claude")
	sess := NewSession()
	sess.StartCapture()

	if err := EmitWithProvenance(sess, a, b, cfg, false); err != nil {
		t.Fatal(err)
	}

	paths := map[string]bool{}
	for _, f := range sess.StopCapture() {
		paths[filepath.ToSlash(f.Path)] = true
	}
	if paths[".claude/agents/hand.md"] {
		t.Error("user-owned agent must not be captured")
	}
	if !paths[".claude/agents/gen.md"] {
		t.Errorf("managed agent missing from capture: %v", paths)
	}
}

// sync never writes a user-owned shared AGENTS.md, so readers that would
// disagree about its content are not a conflict.
func TestValidateScopedRules_UnmanagedSharedPathSkipsReaderConflicts(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "payments", Body: "new",
		Meta: map[string]any{"scope": "payments", "target": "codex"}}})
	cfg := &config.Config{Targets: []string{"codex", "amp"}}
	cfg.Sync.Unmanaged = []string{"payments/AGENTS.md"}

	if err := ValidateScopedRules(cfg, b, cfg.Targets); err != nil {
		t.Fatalf("user-owned shared path must not raise a reader conflict: %v", err)
	}
}

func TestValidateScopedRules_IgnoresUnmanagedDestination(t *testing.T) {
	for _, tc := range []struct{ target, destination string }{
		{"codex", "payments/AGENTS.md"},
		{"claude", ".claude/rules/payments/payments.md"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			if err := os.MkdirAll(filepath.Dir(tc.destination), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(tc.destination, []byte("keep my instructions"), 0o644); err != nil {
				t.Fatal(err)
			}
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "payments", Meta: map[string]any{"scope": "payments"}, Body: "new"}})
			cfg := &config.Config{Targets: []string{tc.target}}
			cfg.Sync.Unmanaged = []string{tc.destination}

			if err := ValidateScopedRules(cfg, b, cfg.Targets); err != nil {
				t.Fatalf("user-owned destination must not fail preflight: %v", err)
			}
		})
	}
}
