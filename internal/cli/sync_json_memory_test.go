package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

// memorySeedProject sets up a repo-mode memory project whose checkout
// store holds one fact to seed, plus an overlay file no target reads.
// It returns the personal store sync seeds.
func memorySeedProject(t *testing.T) string {
	t.Helper()
	repoMemoryProject(t, true)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n")
	root, _ := os.Getwd()
	writeStore(t, filepath.Join(root, ".agnostic-ai", "local", "memory"), map[string]string{"tabs.md": tabsFact})
	writeFile(t, filepath.Join(".agnostic-ai", "overlays", "gemini.settings.json"), `{"ui": {"theme": "Dracula"}}`)
	return memoryPaths(t)["personal"]
}

// treeFiles lists the files under dir, leaving out Git and the command
// lock only a CLI run takes.
func treeFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if !d.IsDir() && d.Name() != projectLockName {
			rel, _ := filepath.Rel(dir, path)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(out)
	return out
}

func TestSyncJSON_PreparesAndSeedsMemoryLikePlainSync(t *testing.T) {
	leftBehind := map[string][2][]string{}
	for _, json := range []bool{false, true} {
		name := map[bool]string{false: "sync", true: "sync --json"}[json]
		t.Run(name, func(t *testing.T) {
			store := memorySeedProject(t)
			captureLogOut(t)
			notes := captureNotes(t)
			if json {
				out := runSyncJSONDrops(t)
				var messages []string
				for _, n := range out.Notes {
					messages = append(messages, n.Target+": "+n.Message)
				}
				joined := strings.Join(messages, "\n")
				for _, want := range []string{".agnostic-ai/overlays/gemini.settings.json: sync does not read", "copied 1 fact(s)"} {
					if !strings.Contains(joined, want) {
						t.Errorf("JSON notes lack %q:\n%s", want, joined)
					}
				}
				if notes.Len() != 0 {
					t.Errorf("sync --json printed notes outside the JSON:\n%s", notes.String())
				}
			} else if err := runSyncOnce(".", nil, false, false, "", 1); err != nil {
				t.Fatal(err)
			}
			if seeds := adapters.TakeMemorySeeds(); len(seeds) != 0 {
				t.Errorf("memory seed queue = %+v, want empty", seeds)
			}
			if got := readText(t, filepath.Join(store, "tabs.md")); got != tabsFact {
				t.Errorf("seeded fact = %q", got)
			}
			root, _ := os.Getwd()
			leftBehind[name] = [2][]string{treeFiles(t, root), treeFiles(t, store)}
		})
	}
	plain, viaJSON := leftBehind["sync"], leftBehind["sync --json"]
	if !slices.Equal(plain[0], viaJSON[0]) {
		t.Errorf("project files differ:\nsync:        %v\nsync --json: %v", plain[0], viaJSON[0])
	}
	if !slices.Equal(plain[1], viaJSON[1]) {
		t.Errorf("personal store files differ:\nsync:        %v\nsync --json: %v", plain[1], viaJSON[1])
	}
}
