package integration

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A second `import all` right after `sync` reads only files sync wrote,
// so it must leave the sources as they are (#1349).
func TestImportAll_AgainAfterSyncLeavesSourcesUnchanged(t *testing.T) {
	cases := map[string]map[string]string{
		"no tool config": {},
		// With .codex/ detected, codex imports the nested AGENTS.md as a
		// scoped rule that claude writes under .claude/rules/services/api/.
		"scoped rule": {".codex/config.toml": ""},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			files := map[string]string{
				"agnostic-ai.yaml":       "version: 1\ntargets: [claude, codex]\n",
				"AGENTS.md":              "# Project\n\nRoot guidance.\n",
				"services/api/AGENTS.md": "# API\n\nUse integer minor units.\n",
			}
			for k, v := range extra {
				files[k] = v
			}
			for rel, body := range files {
				must(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755))
				must(t, os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644))
			}

			runCmd(t, "import", "all")
			runCmd(t, "sync")
			before := sourceSnapshot(t, filepath.Join(dir, ".agnostic-ai"))

			runCmd(t, "import", "all")

			after := sourceSnapshot(t, filepath.Join(dir, ".agnostic-ai"))
			for rel, body := range after {
				if prior, ok := before[rel]; !ok {
					t.Errorf("second import added %s:\n%s", rel, body)
				} else if prior != body {
					t.Errorf("second import changed %s\nbefore:\n%s\nafter:\n%s", rel, prior, body)
				}
			}
			for rel := range before {
				if _, ok := after[rel]; !ok {
					t.Errorf("second import removed %s", rel)
				}
			}
			runCmd(t, "sync", "--check")
		})
	}
}

// sourceSnapshot maps every source file under dir, except the sync
// state cache, to its content.
func sourceSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() == ".sync-state" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
