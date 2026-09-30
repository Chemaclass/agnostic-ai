package claude

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

func writeSettingsOverlay(t *testing.T, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(settingsOverlayPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsOverlayPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func dryRunSettings(t *testing.T, b spec.Bundle) (string, error) {
	t.Helper()
	sess := emit.NewSession()
	sess.StartCapture()
	err := New().Emit(sess, b, &config.Config{}, true)
	for _, f := range sess.StopCapture() {
		if f.Path == filepath.Join(".claude", "settings.json") {
			return f.Content, err
		}
	}
	return "", err
}

func TestEmit_DryRunReadsSettingsOverlay(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	writeSettingsOverlay(t, `{"enabledPlugins": {"plugin-a": true}, "statusLine": {"type": "command", "command": "echo status"}}`)
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindHook, Name: "h1", Meta: map[string]any{
		"event": "PostToolUse", "matcher": "Edit", "command": "echo hi",
	}}})

	preview, err := dryRunSettings(t, b)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("sync: %v", err)
	}
	written, err := os.ReadFile(filepath.Join(".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if preview != string(written) {
		t.Errorf("dry run diverges from sync\nDRY RUN:\n%s\nSYNC:\n%s", preview, written)
	}
}

func TestEmit_DryRunRejectsMalformedSettingsOverlay(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	writeSettingsOverlay(t, `{"statusLine": `)
	b := spec.NewBundle(nil)

	_, dryErr := dryRunSettings(t, b)
	syncErr := New().Emit(emit.NewSession(), b, &config.Config{}, false)
	want := "parse " + settingsOverlayPath
	for name, err := range map[string]error{"dry run": dryErr, "sync": syncErr} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: want error containing %q, got %v", name, want, err)
		}
	}
	if dryErr != nil && syncErr != nil && dryErr.Error() != syncErr.Error() {
		t.Errorf("dry run error %q differs from sync error %q", dryErr, syncErr)
	}
}
