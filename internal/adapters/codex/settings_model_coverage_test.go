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

func TestEmit_SettingsModelCoverageFollowsOverrides(t *testing.T) {
	for _, tc := range []struct {
		name, model, overlay, err string
		dryRun                    bool
	}{
		{name: "output", model: "test-codex-model"},
		{name: "overlay", overlay: "model = \"overlay-model\"\n"},
		{name: "overlay dry run", overlay: "model = \"overlay-model\"\n", dryRun: true},
		{name: "no override", err: "Claude model"},
		{name: "profile only", overlay: "[profiles.work]\nmodel = \"profile-model\"\n", err: "Claude model"},
		{name: "broken overlay", overlay: "model =\n", err: "parse " + configOverlayPath},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.TempCwd(t)
			if tc.overlay != "" {
				if err := os.MkdirAll(filepath.Dir(configOverlayPath), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(configOverlayPath, []byte(tc.overlay), 0644); err != nil {
					t.Fatal(err)
				}
			}
			cfg := &config.Config{OnUnsupported: "error", Outputs: map[string]config.Output{target: {Config: &config.CodexConfig{Model: tc.model}}}}
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindSettings, Name: "shared", Path: ".agnostic-ai/settings/shared.yaml", Meta: map[string]any{"model": "sonnet"}}})
			emit.ResetCoverageNotes()
			t.Cleanup(emit.ResetCoverageNotes)
			sess := emit.NewSession()
			if tc.dryRun {
				sess.StartCapture()
			}
			err := New().Emit(sess, b, cfg, tc.dryRun)
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Errorf("expected %q error, got %v", tc.err, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			body := emittedConfig(t, sess, tc.dryRun)
			if strings.Contains(body, "sonnet") {
				t.Errorf("shared model reached output: %s", body)
			}
			if tc.model != "" && !strings.Contains(body, tc.model) {
				t.Errorf("missing output model: %s", body)
			}
			if tc.overlay != "" && !strings.Contains(body, tc.overlay) {
				t.Errorf("missing overlay model: %s", body)
			}
			b.Agents = []spec.Entry{{Kind: spec.KindAgent, Name: "reviewer", Path: ".agnostic-ai/agents/reviewer.md", Meta: map[string]any{"model": "sonnet"}}}
			err = New().Emit(emit.NewSession(), b, cfg, tc.dryRun)
			if err == nil || !strings.Contains(err.Error(), "reviewer.md") {
				t.Errorf("agent model must remain rejected, got %v", err)
			}
		})
	}
}

func emittedConfig(t *testing.T, sess *emit.Session, dryRun bool) string {
	t.Helper()
	if !dryRun {
		return readFile(t, defaultConfigFile)
	}
	for _, f := range sess.StopCapture() {
		if f.Path == defaultConfigFile {
			return f.Content
		}
	}
	t.Fatalf("dry run previewed no %s", defaultConfigFile)
	return ""
}
