package adapters

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_NamespacedCommandNeverPutsAColonInAPath(t *testing.T) {
	for _, target := range Names() {
		t.Run(target, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			cfg := &config.Config{Targets: []string{target}}
			bundle := spec.NewBundle([]spec.Entry{{
				Kind: spec.KindCommand, Name: "git:commit", Path: ".agnostic-ai/commands/git-commit.md",
				Meta:     map[string]any{"name": "git:commit", "description": "Write a commit message"},
				MetaKeys: []string{"name", "description"}, Body: "Write a commit message.",
			}})
			adapter, _ := Get(target)
			if err := EmitWithProvenance(NewSession(), adapter, bundle, cfg, false); err != nil {
				t.Fatalf("emit: %v", err)
			}
			err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				rel, _ := filepath.Rel(dir, path)
				if strings.Contains(rel, ":") {
					t.Errorf("path holds a colon: %s", rel)
				}
				if filepath.Base(path) == "SKILL.md" {
					if body, _ := os.ReadFile(path); strings.Contains(string(body), "name: git:commit") {
						t.Errorf("%s names the skill git:commit", rel)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEmit_NamespacedCommandKeepsGeminiSubfolder(t *testing.T) {
	dir := testutil.TempCwd(t)
	cfg := &config.Config{Targets: []string{"gemini"}}
	bundle := spec.NewBundle([]spec.Entry{{
		Kind: spec.KindCommand, Name: "git:commit", Path: ".agnostic-ai/commands/git-commit.md",
		Meta: map[string]any{"name": "git:commit"}, MetaKeys: []string{"name"}, Body: "Write a commit message.",
	}})
	adapter, _ := Get("gemini")
	if err := EmitWithProvenance(NewSession(), adapter, bundle, cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gemini", "commands", "git", "commit.toml")); err != nil {
		t.Errorf("gemini lost its namespace folder: %v", err)
	}
}

func TestFlatCommandNames_TakesAFreeNameWhenTheDashFormIsTaken(t *testing.T) {
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindCommand, Name: "git-commit", Meta: map[string]any{}},
		{Kind: spec.KindCommand, Name: "git:commit", Meta: map[string]any{"name": "git:commit"}},
	})
	got := FlatCommandNames(b, "claude").Commands
	if got[0].Name != "git-commit" || got[1].Name != "git-commit-2" || got[1].Meta["name"] != "git-commit-2" {
		t.Errorf("names = %q, %q (meta %v)", got[0].Name, got[1].Name, got[1].Meta["name"])
	}
	if b.Commands[1].Meta["name"] != "git:commit" {
		t.Errorf("the source bundle changed: %v", b.Commands[1].Meta)
	}
	if kept := FlatCommandNames(b, "gemini").Commands[1].Name; kept != "git:commit" {
		t.Errorf("gemini name = %q, want git:commit", kept)
	}
}
