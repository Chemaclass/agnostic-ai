package integration

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestCopilotRoundTrip_SyncImportSyncIsByteEqual is the copilot
// audit's byte-stability gate from #331 acceptance criterion C:
//
//	sync copilot -> snapshot .github/instructions/* + .vscode/mcp.json
//	             -> wipe source specs
//	             -> import copilot
//	             -> wipe emit
//	             -> sync copilot
//	             -> assert byte-for-byte identical
//
// The fixture covers every supported kind that emits to per-file
// outputs. The project-root `.github/copilot-instructions.md` is
// sync-owned, so always-on rules without `globs` would land there
// and are excluded from this round-trip (same carve-out as the codex
// + claude audits). Rules in the fixture carry explicit `globs:` so
// they emit as per-file scoped instructions.
func TestCopilotRoundTrip_SyncImportSyncIsByteEqual(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	seedCopilotRoundTripFixture(t, dir)

	runCmd(t, "sync", "-t", "copilot")
	first := snapshotCopilotEmit(t, dir)
	if len(first) == 0 {
		t.Fatalf("first sync produced no copilot output")
	}

	for _, sub := range []string{"agents", "skills", "rules", "mcps", "settings"} {
		if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", sub)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "import", "copilot")

	for _, p := range []string{".github/instructions", ".github/copilot", ".vscode"} {
		if err := os.RemoveAll(filepath.Join(dir, p)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "sync", "-t", "copilot")
	second := snapshotCopilotEmit(t, dir)

	firstPaths := sortedKeys(first)
	secondPaths := sortedKeys(second)
	if !equalStringSlice(firstPaths, secondPaths) {
		t.Fatalf("emit path set changed across round-trip\nfirst:  %v\nsecond: %v",
			firstPaths, secondPaths)
	}
	for _, p := range firstPaths {
		if first[p] != second[p] {
			t.Errorf("byte mismatch at %s (first=%d bytes, second=%d bytes)\n%s",
				p, len(first[p]), len(second[p]), unifiedDiffLines(first[p], second[p]))
		}
	}
}

func seedCopilotRoundTripFixture(t *testing.T, dir string) {
	t.Helper()
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(`version: 1
sources:
  agents: .agnostic-ai/agents
  skills: .agnostic-ai/skills
  rules: .agnostic-ai/rules
  mcps: .agnostic-ai/mcps
  settings: .agnostic-ai/settings
targets:
  - copilot
gitignore:
  enabled: false
`), 0o644))

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/agents"), 0o755))
	for _, n := range []string{"alpha", "beta", "gamma"} {
		must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/agents", n+".md"),
			[]byte("---\nname: "+n+"\n---\n\n"+n+" body\n"), 0o644))
	}

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/skills"), 0o755))
	for _, n := range []string{"uno", "dos", "tres"} {
		must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/skills", n), 0o755))
		must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/skills", n, "SKILL.md"),
			[]byte("---\nname: "+n+"\n---\n\n"+n+" skill body\n"), 0o644))
	}

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/rules"), 0o755))
	for i, n := range []string{"r1", "r2", "r3"} {
		glob := []string{"**/*.go", "**/*.ts", "**/*.py"}[i]
		must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/rules", n+".md"),
			[]byte("---\nname: "+n+"\nglobs: \""+glob+"\"\n---\n\n"+n+" body\n"), 0o644))
	}

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/mcps"), 0o755))
	// cwd/envFile/dev/sandboxEnabled and oauth exercise WithVSCodeMCPExtras
	// (#692); importJSONMCPMap passes server fields through verbatim, so
	// this fixture also proves the import side needs no dedicated mapper.
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/mcps/stdio-server.yaml"),
		[]byte("name: stdio-server\ncommand: npx\nargs:\n  - \"-y\"\n  - \"@modelcontextprotocol/server-filesystem\"\n"+
			"cwd: \"${workspaceFolder}\"\nenvFile: \"${workspaceFolder}/.env\"\nsandboxEnabled: true\n"+
			"dev:\n  watch: \"src/**/*.ts\"\n  debug:\n    type: node\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/mcps/http-server.yaml"),
		[]byte("name: http-server\ntype: http\nurl: https://example.test/mcp\noauth:\n  clientId: example-client-id\n"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/settings"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/settings/defaults.yaml"),
		[]byte("model: gpt-5.4\n"), 0o644))
}

func snapshotCopilotEmit(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, r := range []string{".github/instructions", ".github/copilot/settings.json", ".vscode/mcp.json"} {
		full := filepath.Join(root, r)
		info, err := os.Stat(full)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("stat %s: %v", full, err)
		}
		if !info.IsDir() {
			data, err := os.ReadFile(full)
			if err != nil {
				t.Fatalf("read %s: %v", full, err)
			}
			out[r] = string(data)
			continue
		}
		err = filepath.WalkDir(full, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out[filepath.ToSlash(rel)] = string(data)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", full, err)
		}
	}
	return out
}

func TestCopilotRoundTrip_PreservesNativeSkillPriority(t *testing.T) {
	binary := builtBinary(t)
	dir := t.TempDir()
	files := map[string]string{
		"agnostic-ai.yaml":                 "version: 1\ntargets: [copilot]\n",
		".agents/skills/priority/SKILL.md": "---\nname: priority\ndescription: Shared winner\n---\nShared winner body.\n",
		".claude/skills/priority/SKILL.md": "---\nname: priority\ndescription: Claude loser\n---\nClaude loser body.\n",
	}
	for path, body := range files {
		path = filepath.Join(dir, path)
		must(t, os.MkdirAll(filepath.Dir(path), 0o755))
		must(t, os.WriteFile(path, []byte(body), 0o644))
	}
	for _, args := range [][]string{{"import", "copilot"}, {"sync"}, {"sync", "--check"}, {"lint"}} {
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	for _, path := range []string{".agnostic-ai/skills/priority/SKILL.md", ".agents/skills/priority/SKILL.md"} {
		raw, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "Shared winner body.") {
			t.Errorf("%s lost winning skill: %s", path, raw)
		}
	}
}
