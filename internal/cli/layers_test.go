package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func TestResolveLayers_ProjectOnlyByDefault(t *testing.T) {
	t.Setenv(envUserGlobalRoot, "/nonexistent-agnostic-ai-test-path")
	t.Setenv("HOME", t.TempDir()) // empty home, no ~/.agnostic-ai

	root := t.TempDir()
	cfg := &config.Config{Sources: defaultLayerSources()}

	layers := resolveLayers(root, cfg)
	if len(layers) != 1 {
		t.Fatalf("expected 1 layer, got %d (%+v)", len(layers), layers)
	}
	if layers[0].Name != layerNameProject {
		t.Errorf("layer[0]=%q, want %q", layers[0].Name, layerNameProject)
	}
}

func TestResolveLayers_UserGlobalDetectedViaEnv(t *testing.T) {
	ug := t.TempDir()
	t.Setenv(envUserGlobalRoot, ug)
	t.Setenv("HOME", t.TempDir())

	root := t.TempDir()
	cfg := &config.Config{Sources: defaultLayerSources()}

	layers := resolveLayers(root, cfg)
	if len(layers) != 2 {
		t.Fatalf("expected 2 layers, got %d", len(layers))
	}
	if layers[0].Name != layerNameUserGlobal || layers[0].Root != ug {
		t.Errorf("layer[0]=%+v", layers[0])
	}
	if layers[1].Name != layerNameProject {
		t.Errorf("layer[1]=%+v", layers[1])
	}
}

func TestResolveLayers_ProjectUserDetected(t *testing.T) {
	t.Setenv(envUserGlobalRoot, "/nonexistent-agnostic-ai-test-path")
	t.Setenv("HOME", t.TempDir())

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, defaultProjectUser), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Sources: defaultLayerSources()}

	layers := resolveLayers(root, cfg)
	if len(layers) != 2 {
		t.Fatalf("expected 2 layers, got %d", len(layers))
	}
	if layers[1].Name != layerNameProjectUser {
		t.Errorf("layer[1]=%+v, want project-user", layers[1])
	}
	if layers[1].Root != filepath.Join(root, defaultProjectUser) {
		t.Errorf("project-user root mismatch: %q", layers[1].Root)
	}
}

func TestResolveLayers_AllThreePrecedenceOrder(t *testing.T) {
	ug := t.TempDir()
	t.Setenv(envUserGlobalRoot, ug)
	t.Setenv("HOME", t.TempDir())

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, defaultProjectUser), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Sources: defaultLayerSources()}

	layers := resolveLayers(root, cfg)
	if len(layers) != 3 {
		t.Fatalf("expected 3 layers, got %d", len(layers))
	}
	want := []string{layerNameUserGlobal, layerNameProject, layerNameProjectUser}
	for i, n := range want {
		if layers[i].Name != n {
			t.Errorf("layers[%d]=%q, want %q", i, layers[i].Name, n)
		}
	}
}
