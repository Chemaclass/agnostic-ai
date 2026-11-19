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
