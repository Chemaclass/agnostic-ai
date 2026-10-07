package qoder

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
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

func additionalDirectoriesSpec(dirs ...any) spec.Bundle {
	return spec.NewBundle([]spec.Entry{{Kind: spec.KindSettings, Name: "defaults", Meta: map[string]any{
		"x-qoder": map[string]any{"permissions": map[string]any{"additionalDirectories": dirs}},
	}}})
}

func sortedAdditionalDirectories(t *testing.T) []string {
	t.Helper()
	got := additionalDirectories(t)
	slices.Sort(got)
	return got
}

func TestEmit_AdditionalDirectoriesKeepNativeEntriesWithoutRepoMemory(t *testing.T) {
	testutil.TempCwd(t)
	if err := os.MkdirAll(filepath.Dir(defaultMCPFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaultMCPFile, []byte(`{"permissions":{"additionalDirectories":["native"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	b := additionalDirectoriesSpec("spec")
	for range 3 {
		if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
			t.Fatal(err)
		}
		if got := sortedAdditionalDirectories(t); !slices.Equal(got, []string{"native", "spec"}) {
			t.Fatalf("additionalDirectories = %v, want [native spec]", got)
		}
	}
}

func TestEmit_LeavingRepoMemoryDropsOnlyTheStore(t *testing.T) {
	for name, leave := range map[string]func(*config.Config){
		"checkout mode":   func(c *config.Config) { c.Memory.Personal = config.PersonalMemoryCheckout },
		"gitignore off":   func(c *config.Config) { c.Gitignore.Enabled = false },
		"memory built-in": func(c *config.Config) { c.Builtins = nil },
	} {
		t.Run(name, func(t *testing.T) {
			testutil.TempCwd(t)
			t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
			if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
				t.Fatalf("git init: %v\n%s", err, out)
			}
			if err := os.MkdirAll(filepath.Dir(defaultMCPFile), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(defaultMCPFile, []byte(`{"permissions":{"additionalDirectories":["native"]}}`), 0o644); err != nil {
				t.Fatal(err)
			}
			prior := emit.PriorMergedKeys
			t.Cleanup(func() { emit.PriorMergedKeys = prior })
			var claims []emit.MergedKey
			emit.PriorMergedKeys = func(string) []emit.MergedKey { return claims }
			sync := func(cfg *config.Config) {
				t.Helper()
				sess := emit.NewSession()
				sess.StartDetailedRecording()
				if err := New().Emit(sess, additionalDirectoriesSpec("spec"), cfg, false); err != nil {
					t.Fatal(err)
				}
				for _, w := range sess.StopDetailedRecording() {
					if w.Path == defaultMCPFile {
						claims = w.Keys
					}
				}
			}

			cfg := repoMemoryConfig(true)
			dir, err := emit.PersonalMemoryDirFor(cfg, defaultMCPFile, target)
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"native", "spec", filepath.ToSlash(dir)}
			slices.Sort(want)
			sync(cfg)
			if got := sortedAdditionalDirectories(t); !slices.Equal(got, want) {
				t.Fatalf("repo mode: %v, want %v", got, want)
			}

			leave(cfg)
			for range 2 {
				sync(cfg)
				if got := sortedAdditionalDirectories(t); !slices.Equal(got, []string{"native", "spec"}) {
					t.Fatalf("after leaving: %v, want [native spec]", got)
				}
			}
		})
	}
}

func TestEmit_LeavingRepoMemoryLeavesNoStoreEntry(t *testing.T) {
	testutil.TempCwd(t)
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	prior := emit.PriorMergedKeys
	t.Cleanup(func() { emit.PriorMergedKeys = prior })
	var claims []emit.MergedKey
	emit.PriorMergedKeys = func(string) []emit.MergedKey { return claims }
	sync := func(cfg *config.Config) {
		t.Helper()
		sess := emit.NewSession()
		sess.StartDetailedRecording()
		if err := New().Emit(sess, spec.NewBundle(nil), cfg, false); err != nil {
			t.Fatal(err)
		}
		for _, w := range sess.StopDetailedRecording() {
			if w.Path == defaultMCPFile {
				claims = w.Keys
			}
		}
	}
	cfg := repoMemoryConfig(true)
	sync(cfg)
	if len(additionalDirectories(t)) != 1 {
		t.Fatalf("repo mode: %v", additionalDirectories(t))
	}
	cfg.Memory.Personal = config.PersonalMemoryCheckout
	sync(cfg)
	if got := additionalDirectories(t); len(got) != 0 {
		t.Errorf("store entry stayed: %v", got)
	}
	if raw, _ := os.ReadFile(defaultMCPFile); strings.Contains(string(raw), "additionalDirectories") {
		t.Errorf("key stayed: %s", raw)
	}
}
