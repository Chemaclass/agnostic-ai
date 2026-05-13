package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func runCompletion(t *testing.T, args ...string) string {
	t.Helper()
	root := NewRootCmd("test")
	root.SetArgs(append([]string{"__complete"}, args...))
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(&bytes.Buffer{})
	_ = root.Execute()
	return buf.String()
}

func TestTargetCompletion_FallsBackToDefaults_OutsideProject(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	out := runCompletion(t, "sync", "--target", "")
	for _, tgt := range config.DefaultTargets() {
		if !strings.Contains(out, tgt) {
			t.Errorf("expected default target %q in completion output:\n%s", tgt, out)
		}
	}
}

func TestTargetCompletion_ReadsConfigTargets(t *testing.T) {
	dir := t.TempDir()
	yaml := "version: 1\ntargets:\n  - claude\n  - cursor\n"
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
	out := runCompletion(t, "sync", "--target", "")
	if !strings.Contains(out, "claude") {
		t.Errorf("expected 'claude' in completion output:\n%s", out)
	}
	if !strings.Contains(out, "cursor") {
		t.Errorf("expected 'cursor' in completion output:\n%s", out)
	}
	if strings.Contains(out, "codex") {
		t.Errorf("'codex' should not appear when config restricts targets:\n%s", out)
	}
}

func TestTargetCompletion_DoctorAndRevert(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	for _, sub := range []string{"doctor", "revert"} {
		out := runCompletion(t, sub, "--target", "")
		if !strings.Contains(out, "claude") {
			t.Errorf("%s --target completion: expected 'claude', got:\n%s", sub, out)
		}
	}
}
