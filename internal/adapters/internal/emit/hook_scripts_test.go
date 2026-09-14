package emit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSourceToolFromHookCommand(t *testing.T) {
	cases := []struct {
		cmd, wantTool string
		wantOK        bool
	}{
		{".claude/hooks/x.sh", "claude", true},
		{".codex/hooks/x.sh", "codex", true},
		{".gemini/hooks/x.sh", "gemini", true},
		{"gofmt", "", false},
		{".claude/agents/y.md", "", false},
		{"./hooks/x.sh", "", false},
		{"", "", false},
		// shell-expansion wrapped form — common in hooks.json (#264)
		{`"$(git rev-parse --show-toplevel)/.codex/hooks/protect-files.sh"`, "codex", true},
		{`"$(git rev-parse --show-toplevel)/.claude/hooks/x.sh"`, "claude", true},
	}
	for _, c := range cases {
		tool, ok := SourceToolFromHookCommand(c.cmd)
		if tool != c.wantTool || ok != c.wantOK {
			t.Errorf("SourceToolFromHookCommand(%q) = (%q, %v), want (%q, %v)", c.cmd, tool, ok, c.wantTool, c.wantOK)
		}
	}
}

func TestMaterializeHookScript_CopiesSourceToolStashIntoTargetHooks(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.MkdirAll(".agnostic-ai/scripts/claude", 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte("#!/usr/bin/env bash\necho protect\n")
	if err := os.WriteFile(".agnostic-ai/scripts/claude/protect-files.sh", body, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := NewSession().MaterializeHookScript(".codex/hooks/protect-files.sh", "codex", "claude", false); err != nil {
		t.Fatalf("materialize: %v", err)
	}

	out, err := os.ReadFile(filepath.Join(dir, ".codex/hooks/protect-files.sh"))
	if err != nil {
		t.Fatalf("expected emitted hook script under .codex/hooks/: %v", err)
	}
	if string(out) != string(body) {
		t.Errorf("body mismatch: %q vs %q", out, body)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dir, ".codex/hooks/protect-files.sh"))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o100 == 0 {
			t.Errorf("expected executable bit preserved, got %v", info.Mode().Perm())
		}
	}
}

func TestMaterializeHookScript_TargetVariantWinsOverSourceTool(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.MkdirAll(".agnostic-ai/scripts/codex", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(".agnostic-ai/scripts/claude", 0o755); err != nil {
		t.Fatal(err)
	}
	codexBody := []byte("codex-specific\n")
	claudeBody := []byte("claude-specific\n")
	if err := os.WriteFile(".agnostic-ai/scripts/codex/protect-files.sh", codexBody, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".agnostic-ai/scripts/claude/protect-files.sh", claudeBody, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := NewSession().MaterializeHookScript(".codex/hooks/protect-files.sh", "codex", "claude", false); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(filepath.Join(dir, ".codex/hooks/protect-files.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(codexBody) {
		t.Errorf("expected target-specific variant to win, got %q", out)
	}
}

func TestMaterializeHookScript_FallsBackToUnifiedScript(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.MkdirAll(".agnostic-ai/scripts", 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte("unified\n")
	if err := os.WriteFile(".agnostic-ai/scripts/protect-files.sh", body, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := NewSession().MaterializeHookScript(".codex/hooks/protect-files.sh", "codex", "claude", false); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(filepath.Join(dir, ".codex/hooks/protect-files.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(body) {
		t.Errorf("expected unified variant body, got %q", out)
	}
}

func TestMaterializeHookScript_NoStashIsNoOp(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := NewSession().MaterializeHookScript(".codex/hooks/missing.sh", "codex", "claude", false); err != nil {
		t.Errorf("missing stash should be a no-op, got error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex/hooks/missing.sh")); !os.IsNotExist(err) {
		t.Errorf("no body should be created when stash is empty, err=%v", err)
	}
}

func TestMaterializeHookScript_IgnoresNonHookCommand(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := NewSession().MaterializeHookScript("gofmt", "codex", "", false); err != nil {
		t.Errorf("expected no-op for non-hook command, got %v", err)
	}
}

// Regression for #264: codex hooks.json wraps the script path in a shell
// expansion (`"$(git rev-parse --show-toplevel)/.codex/hooks/x.sh"`).
// MaterializeHookScript must extract the basename and copy the stashed
// body anyway, otherwise the emitted hooks.json references files that
// never get materialized.
func TestMaterializeHookScript_HandlesShellExpansionWrappedPath(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.MkdirAll(".agnostic-ai/scripts/codex", 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte("#!/usr/bin/env bash\necho protect\n")
	if err := os.WriteFile(".agnostic-ai/scripts/codex/protect-files.sh", body, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := `"$(git rev-parse --show-toplevel)/.codex/hooks/protect-files.sh"`
	if err := NewSession().MaterializeHookScript(cmd, "codex", "codex", false); err != nil {
		t.Fatalf("materialize: %v", err)
	}

	out, err := os.ReadFile(filepath.Join(dir, ".codex/hooks/protect-files.sh"))
	if err != nil {
		t.Fatalf("expected emitted hook script under .codex/hooks/: %v", err)
	}
	if string(out) != string(body) {
		t.Errorf("body mismatch: %q vs %q", out, body)
	}
}

func stashHookScript(t *testing.T, body string) {
	t.Helper()
	if err := os.MkdirAll(".agnostic-ai/scripts/claude", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".agnostic-ai/scripts/claude/protect-files.sh", []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// A hook script listed under sync.unmanaged is user-owned: sync never
// overwrites it and reports the skip (#789).
func TestMaterializeHookScript_SkipsUnmanagedScript(t *testing.T) {
	t.Chdir(t.TempDir())
	stashHookScript(t, "#!/bin/sh\necho stash\n")
	const dst = ".codex/hooks/protect-files.sh"
	if err := os.MkdirAll(".codex/hooks", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("hand-edited\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	sess := NewSession()
	sess.SetUnmanaged([]string{dst})

	if err := sess.MaterializeHookScript(dst, "codex", "claude", false); err != nil {
		t.Fatalf("materialize: %v", err)
	}

	if got, _ := os.ReadFile(dst); string(got) != "hand-edited\n" {
		t.Errorf("user-owned script overwritten: %q", got)
	}
	if skips := sess.UnmanagedSkips(); len(skips) != 1 || skips[0] != dst {
		t.Errorf("UnmanagedSkips=%v, want [%s]", skips, dst)
	}
}

// The script write goes through detailed recording, so the sync ledger
// sees it with a content sum and can sweep it once the hook is gone.
func TestMaterializeHookScript_RecordsWriteWithSum(t *testing.T) {
	t.Chdir(t.TempDir())
	body := "#!/bin/sh\necho stash\n"
	stashHookScript(t, body)
	sess := NewSession()
	sess.StartDetailedRecording()

	if err := sess.MaterializeHookScript(".codex/hooks/protect-files.sh", "codex", "claude", false); err != nil {
		t.Fatalf("materialize: %v", err)
	}

	writes := sess.StopDetailedRecording()
	if len(writes) != 1 || writes[0].Path != ".codex/hooks/protect-files.sh" || writes[0].Sum != ContentSum(body) {
		t.Errorf("writes=%+v, want one create with the body sum", writes)
	}
}

// A failed sync rolls the script back like any other output.
func TestMaterializeHookScript_RollbackRestoresPriorScript(t *testing.T) {
	t.Chdir(t.TempDir())
	stashHookScript(t, "#!/bin/sh\necho new\n")
	const dst = ".codex/hooks/protect-files.sh"
	if err := os.MkdirAll(".codex/hooks", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	sess := NewSession()
	sess.StartTransaction()

	if err := sess.MaterializeHookScript(dst, "codex", "claude", false); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if err := sess.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	if got, _ := os.ReadFile(dst); string(got) != "old\n" {
		t.Errorf("rollback did not restore script: %q", got)
	}
}
