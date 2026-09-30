package codex

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

func writeExecPoliciesOverlay(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(execPoliciesOverlayPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(execPoliciesOverlayPath, []byte("- pattern: [git, status]\n  decision: allow\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func dryRunExecPolicies(t *testing.T) (string, error) {
	t.Helper()
	sess := emit.NewSession()
	sess.StartCapture()
	err := New().Emit(sess, spec.NewBundle(nil), &config.Config{}, true)
	for _, f := range sess.StopCapture() {
		if f.Path == defaultExecPoliciesFile {
			return f.Content, err
		}
	}
	return "", err
}

func TestEmit_DryRunReadsExecPoliciesHeaderOverlay(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	writeExecPoliciesOverlay(t)
	if err := os.WriteFile(execPoliciesHeaderOverlayPath, []byte("Team command policy.\nReview before widening.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	preview, err := dryRunExecPolicies(t)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatalf("sync: %v", err)
	}
	written := readFile(t, defaultExecPoliciesFile)
	if !strings.Contains(written, "# Team command policy.\n# Review before widening.\n") {
		t.Fatalf("sync dropped the captured header:\n%s", written)
	}
	if preview != written {
		t.Errorf("dry run diverges from sync\nDRY RUN:\n%s\nSYNC:\n%s", preview, written)
	}
}

func TestEmit_DryRunRejectsUnreadableExecPoliciesHeaderOverlay(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	writeExecPoliciesOverlay(t)
	if err := os.Mkdir(execPoliciesHeaderOverlayPath, 0o755); err != nil {
		t.Fatal(err)
	}

	_, dryErr := dryRunExecPolicies(t)
	syncErr := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{}, false)
	for name, err := range map[string]error{"dry run": dryErr, "sync": syncErr} {
		if err == nil || !strings.Contains(err.Error(), execPoliciesHeaderOverlayPath) {
			t.Errorf("%s: want error naming %q, got %v", name, execPoliciesHeaderOverlayPath, err)
		}
	}
	if dryErr != nil && syncErr != nil && dryErr.Error() != syncErr.Error() {
		t.Errorf("dry run error %q differs from sync error %q", dryErr, syncErr)
	}
}
