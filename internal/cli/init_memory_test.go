package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestInit_MemoryUsesRepoModeOnlyForIgnoredGitProjects(t *testing.T) {
	for _, tc := range []struct {
		name      string
		git       bool
		gitignore bool
		memory    bool
		wantRepo  bool
	}{
		{name: "memory in Git", git: true, gitignore: true, memory: true, wantRepo: true},
		{name: "memory outside Git", gitignore: true, memory: true},
		{name: "memory with outputs committed", git: true, memory: true},
		{name: "no memory requested", git: true, gitignore: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			silence(t)
			if tc.git {
				gitInit(t, dir)
			}
			args := []string{"init", "--gitignore=off"}
			if tc.gitignore {
				args[1] = "--gitignore=on"
			}
			if tc.memory {
				args = append(args, "--memory")
			}
			cmd := NewRootCmd("test")
			cmd.SetIn(strings.NewReader("claude\n"))
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("init: %v", err)
			}
			cfg, err := config.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			if slices.Contains(cfg.Builtins, "memory") != tc.memory {
				t.Errorf("builtins = %v, want memory enabled=%t", cfg.Builtins, tc.memory)
			}
			if cfg.RepoPersonalMemory() != tc.wantRepo {
				t.Errorf("personal mode = %q, want repo=%t", cfg.Memory.Personal, tc.wantRepo)
			}
			shared, err := os.ReadFile(filepath.Join(dir, config.ConfigFileName))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(shared), "personal:") {
				t.Error("init wrote a personal choice to the shared config")
			}
			if !tc.wantRepo {
				if _, err := os.Stat(filepath.Join(dir, config.LocalOverrideFileName)); !os.IsNotExist(err) {
					t.Errorf("init created local config: %v", err)
				}
			}
		})
	}
}

func TestInit_MemoryPreservesExistingLocalConfig(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	gitInit(t, dir)
	path := filepath.Join(dir, config.LocalOverrideFileName)
	before := "# Keep this choice.\nmemory:\n  personal: checkout\n"
	writeFile(t, path, before)
	cmd := NewRootCmd("test")
	cmd.SetIn(strings.NewReader("claude\n"))
	cmd.SetArgs([]string{"init", "--memory"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != before {
		t.Errorf("local config changed: %s", after)
	}
}

func TestInit_MemoryDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	gitInit(t, dir)
	cmd := NewRootCmd("test")
	cmd.SetIn(strings.NewReader("claude\n"))
	cmd.SetArgs([]string{"init", "--memory", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{config.ConfigFileName, config.LocalOverrideFileName, ".gitignore"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("dry run wrote %s: %v", name, err)
		}
	}
}
