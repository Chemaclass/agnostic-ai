package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
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

// The local layer lives inside the source dir, the same shape as the
// global ~/.agnostic-ai/local/. The old sibling folder is not read.
func TestResolveLayers_ReadsLocalInsideTheSourceDirOnly(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agnostic-ai.local"), 0o755); err != nil {
		t.Fatal(err)
	}
	if layers := resolveLayers(root, &config.Config{Sources: defaultLayerSources()}); len(layers) != 1 {
		t.Fatalf("old .agnostic-ai.local was loaded: %+v", layers)
	}
	if err := os.MkdirAll(filepath.Join(root, ".agnostic-ai", "local"), 0o755); err != nil {
		t.Fatal(err)
	}
	layers := resolveLayers(root, &config.Config{Sources: defaultLayerSources()})
	if len(layers) != 2 || layers[1].Root != filepath.Join(root, ".agnostic-ai", "local") {
		t.Fatalf("want .agnostic-ai/local as the project-user layer, got %+v", layers)
	}
}

// The local layer edits one field and extends the body of a shared spec,
// while the committed spec stays the source of truth.
func TestSync_ProjectLocalLayerOverridesAFieldAndExtendsTheBody(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "custom", "SKILL.md"),
		"---\nname: custom\ndescription: Custom skill\nmodel:\n  claude: sonnet\n---\nfoo for\n")
	writeFile(t, filepath.Join(dir, defaultProjectUser, "skills", "custom", "SKILL.md"),
		"---\nname: custom\nmodel:\n  claude: opus\n---\n::parent\nbar baz\n")

	execCLI(t, "sync")

	got := readFile(t, filepath.Join(dir, ".claude", "skills", "custom", "SKILL.md"))
	for _, want := range []string{"model: opus", "description: Custom skill", "foo for\nbar baz"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "sonnet") || strings.Contains(got, "::parent") {
		t.Errorf("shared model or marker leaked:\n%s", got)
	}
}
