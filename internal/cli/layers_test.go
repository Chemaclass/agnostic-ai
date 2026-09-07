package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func TestResolveLayers_ProjectOnlyByDefault(t *testing.T) {
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

func TestResolveLayers_DoesNotLoadGlobalHome(t *testing.T) {
	globalHome := t.TempDir()
	t.Setenv(envUserGlobalRoot, globalHome)
	if err := os.MkdirAll(filepath.Join(globalHome, "rules"), 0o755); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	layers := resolveLayers(root, &config.Config{Sources: defaultLayerSources()})
	if len(layers) != 1 || layers[0].Name != layerNameProject {
		t.Fatalf("global home joined project layers: %+v", layers)
	}
}

func TestResolveLayers_ProjectUserDetected(t *testing.T) {
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

func TestResolveLayers_ProjectAndProjectUserPrecedenceOrder(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, defaultProjectUser), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Sources: defaultLayerSources()}

	layers := resolveLayers(root, cfg)
	if len(layers) != 2 {
		t.Fatalf("expected 2 layers, got %d", len(layers))
	}
	want := []string{layerNameProject, layerNameProjectUser}
	for i, n := range want {
		if layers[i].Name != n {
			t.Errorf("layers[%d]=%q, want %q", i, layers[i].Name, n)
		}
	}
}
