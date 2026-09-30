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
		name, model, overlay string
		rejected             bool
	}{
		{name: "output", model: "test-codex-model"},
		{name: "overlay", overlay: "model = \"overlay-model\"\n"},
		{name: "no override", rejected: true},
		{name: "profile only", overlay: "[profiles.work]\nmodel = \"profile-model\"\n", rejected: true},
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
			err := New().Emit(emit.NewSession(), b, cfg, false)
			if tc.rejected {
				if err == nil || !strings.Contains(err.Error(), "Claude model") {
					t.Errorf("expected model error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			body := readFile(t, defaultConfigFile)
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
			err = New().Emit(emit.NewSession(), b, cfg, false)
			if err == nil || !strings.Contains(err.Error(), "reviewer.md") {
				t.Errorf("agent model must remain rejected, got %v", err)
			}
		})
	}
}
