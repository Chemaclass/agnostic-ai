package qoder

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func repoMemoryConfig(gitignore bool) *config.Config {
	return &config.Config{
		Builtins:  []string{emit.MemoryBuiltin},
		Memory:    config.MemoryConfig{Personal: config.PersonalMemoryRepo},
		Gitignore: config.Gitignore{Enabled: gitignore},
	}
}

func additionalDirectories(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(defaultMCPFile)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Permissions struct {
			AdditionalDirectories []string
			DefaultMode           string
		}
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Permissions.AdditionalDirectories
}

func TestEmit_RepoMemoryStoreJoinsAdditionalDirectories(t *testing.T) {
	testutil.TempCwd(t)
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := os.MkdirAll(filepath.Dir(defaultMCPFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaultMCPFile, []byte(`{"permissions":{"additionalDirectories":["native"],"defaultMode":"ask"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := repoMemoryConfig(true)
	dir, err := emit.PersonalMemoryDirFor(cfg, defaultMCPFile, target)
	if err != nil {
		t.Fatal(err)
	}
	dir = filepath.ToSlash(dir)
	for range 2 {
		if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
			t.Fatal(err)
		}
		got := additionalDirectories(t)
		slices.Sort(got)
		want := []string{"native", dir}
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("additionalDirectories = %v, want %v", got, want)
		}
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("store not created: %v", err)
	}
}

func TestEmit_RepoMemoryStoreStaysOutOfTrackedSettings(t *testing.T) {
	testutil.TempCwd(t)
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), repoMemoryConfig(false), false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(defaultMCPFile); !os.IsNotExist(err) {
		t.Errorf("settings written without gitignore: %v", err)
	}
}
