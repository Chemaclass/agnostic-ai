package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSync_DetectsAGENTSMdCollision(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "codex,amp"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected collision error when codex and amp share AGENTS.md")
	}
	if !strings.Contains(err.Error(), "output collision") {
		t.Errorf("expected 'output collision' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "AGENTS.md") {
		t.Errorf("expected colliding path in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "amp") || !strings.Contains(err.Error(), "codex") {
		t.Errorf("expected both target names in error, got: %v", err)
	}
}

func TestSync_NoCollisionForDisjointTargets(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude,codex"})
	if err := root.Execute(); err != nil {
		t.Errorf("claude+codex should not collide: %v", err)
	}
}

func TestSync_Check_DetectsCollision(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "codex,warp", "--check"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected collision error from sync --check")
	}
	if !strings.Contains(err.Error(), "output collision") {
		t.Errorf("expected 'output collision' in error, got: %v", err)
	}
}
