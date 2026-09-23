package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportFromGemini_ImportsDefaultModel(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, geminiSettings), `{"model":{"name":"gemini-2.5-pro","maxSessionTurns":15},"permissions":{"deny":["Bash(*)"]}}`)
	sources := rootSources()
	sources.Settings = "settings"
	if err := importFromGemini(root, sources); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "settings/gemini.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "model: gemini-2.5-pro") || strings.Contains(string(raw), "permissions") {
		t.Errorf("imported settings: %s", raw)
	}
	cfg := &config.Config{Sources: sources}
	bundle, err := spec.LoadBundle(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	output := testutil.TempCwd(t)
	adapter, _ := adapters.Get("gemini")
	if err := adapter.Emit(adapters.NewSession(), bundle, cfg, false); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(filepath.Join(output, geminiSettings))
	if err != nil {
		t.Fatal(err)
	}
	var native map[string]any
	if err := json.Unmarshal(raw, &native); err != nil {
		t.Fatal(err)
	}
	model, _ := native["model"].(map[string]any)
	if model["name"] != "gemini-2.5-pro" {
		t.Errorf("round-trip model = %#v", model)
	}
}
