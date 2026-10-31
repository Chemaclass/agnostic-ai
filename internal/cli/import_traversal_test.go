package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func TestImportAgentNamesCannotEscapeSourceDirectory(t *testing.T) {
	for _, tc := range []struct {
		name, dir, file, body string
		run                   func(string, string) (int, error)
	}{
		{"codex", ".codex/agents", "safe.toml", "name = \"../../../victim\"\ndeveloper_instructions = \"body\"\n", importCodexAgents},
		{"warp", ".warp/workflows", "safe.yaml", "name: ../../../victim\ncommand: body\n", importWarpWorkflows},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "project")
			dst := filepath.Join(root, ".agnostic-ai/agents")
			if err := os.MkdirAll(filepath.Join(root, tc.dir), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(dst, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, tc.dir, tc.file), []byte(tc.body), 0644); err != nil {
				t.Fatal(err)
			}
			victim := filepath.Join(parent, "victim.md")
			if err := os.WriteFile(victim, []byte("safe"), 0644); err != nil {
				t.Fatal(err)
			}
			if _, err := tc.run(root, dst); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(victim)
			if err != nil || string(got) != "safe" {
				t.Fatalf("victim changed: %q, %v", got, err)
			}
			got, err = os.ReadFile(filepath.Join(dst, "safe.md"))
			if err != nil || !strings.Contains(string(got), "body") {
				t.Fatalf("safe agent: %q, %v", got, err)
			}
		})
	}
}

func TestImportWriteFileRestrictsDestinations(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	source := filepath.Join(parent, "source")
	for _, dir := range []string{root, source} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	release, err := restrictImportWrites(root, config.Sources{Agents: source})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	for _, path := range []string{filepath.Join(root, "inside.md"), filepath.Join(source, "agent.md")} {
		if err := importWriteFile(path, []byte("allowed"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(parent, "outside.md")
	if err := importWriteFile(outside, []byte("blocked"), 0644); err == nil {
		t.Fatal("outside write succeeded")
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("outside file exists: %v", err)
	}
	link := filepath.Join(root, "outside-link")
	if err := os.Symlink(parent, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := importWriteFile(filepath.Join(link, "outside.md"), []byte("blocked"), 0644); err == nil {
		t.Fatal("write through outside link succeeded")
	}
}
