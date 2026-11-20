package approvals_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/approvals"
)

func TestSpecSources_ReadsMovedFolders(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\nsources:\n  hooks: config/hooks\n  agents: specs/agents\n  skills: config/skills\n")
	writeFile(t, filepath.Join(root, "agnostic-ai.local.yaml"), "sources:\n  settings: /abs/settings\n")
	writeFile(t, filepath.Join(root, "agnostic.config.yaml"), "version: 1\nsources:\n  mcps: old/mcps\n")

	got, err := approvals.SpecSources(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{filepath.Join(root, "config/hooks"), filepath.Join(root, "specs/agents"), filepath.Join(root, "old/mcps"), "/abs/settings"} {
		if !slices.Contains(got, dir) {
			t.Errorf("spec folders %v miss %s", got, dir)
		}
	}
	if slices.Contains(got, filepath.Join(root, "config/skills")) {
		t.Errorf("spec folders %v hold the skills folder", got)
	}
}

func TestSpecSources_BrokenConfigIsAnError(t *testing.T) {
	for name, body := range map[string]string{
		"not yaml":       "sources: [",
		"source a list":  "sources:\n  hooks: [a, b]\n",
		"sources a list": "sources: [a]\n",
		"source is root": "sources:\n  hooks: .\n",
		"source above":   "sources:\n  agents: ../x/..\n",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), body)
			if _, err := approvals.SpecSources(root); err == nil {
				t.Error("want an error, so the gate asks")
			}
		})
	}
	if got, err := approvals.SpecSources(t.TempDir()); err != nil || len(got) != 0 {
		t.Errorf("no config = %v, %v; want nothing", got, err)
	}
}
