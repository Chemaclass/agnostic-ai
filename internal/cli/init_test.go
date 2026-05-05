package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestScaffold_DefaultBaseDir(t *testing.T) {
	dir := t.TempDir()
	if err := scaffold(dir, ""); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"agents", "skills", "rules", "hooks", "mcps"} {
		if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", d)); err != nil {
			t.Errorf("missing .agnostic-ai/%s", d)
		}
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic.config.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(cfg), "agents: .agnostic-ai/agents") {
		t.Errorf("config missing nested agents path:\n%s", cfg)
	}
}

func TestScaffold_CustomBaseDir(t *testing.T) {
	dir := t.TempDir()
	if err := scaffold(dir, "specs"); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"agents", "skills", "rules", "hooks", "mcps"} {
		if _, err := os.Stat(filepath.Join(dir, "specs", d)); err != nil {
			t.Errorf("missing specs/%s", d)
		}
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic.config.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(cfg), "agents: specs/agents") {
		t.Errorf("config missing custom path:\n%s", cfg)
	}
}

func TestScaffold_BaseDirDot_WritesAtRoot(t *testing.T) {
	dir := t.TempDir()
	if err := scaffold(dir, "."); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"agents", "skills", "rules", "hooks", "mcps"} {
		if _, err := os.Stat(filepath.Join(dir, d)); err != nil {
			t.Errorf("missing %s at root", d)
		}
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic.config.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(cfg), "agents: agents\n") {
		t.Errorf("config should use bare paths when base is '.':\n%s", cfg)
	}
}

func TestScaffold_NestedBaseDir(t *testing.T) {
	dir := t.TempDir()
	if err := scaffold(dir, filepath.Join("config", "ai")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config", "ai", "agents")); err != nil {
		t.Errorf("missing config/ai/agents")
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic.config.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(cfg), "agents: config/ai/agents") {
		t.Errorf("config missing nested base path:\n%s", cfg)
	}
}

func TestInitCmd_PositionalDirArg(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"init", "specs"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "specs", "agents")); err != nil {
		t.Errorf("expected specs/agents/ from positional dir arg, got %v", err)
	}
}

func TestInitCmd_DefaultsToAgnosticAi(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"init"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "agents")); err != nil {
		t.Errorf("expected default .agnostic-ai/agents/, got %v", err)
	}
}

func TestInitCmd_RejectsExtraArgs(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"init", "specs", "extra"})
	if err := root.Execute(); err == nil {
		t.Error("expected error for too many positional args")
	}
}

func TestScaffold_RefusesIfConfigExists(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agnostic.config.yaml"),
		[]byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := scaffold(dir, ""); err == nil {
		t.Error("expected error when config already exists")
	}
}
