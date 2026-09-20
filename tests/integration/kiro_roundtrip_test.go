package integration

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestKiroRoundTrip_HooksSurviveSyncImportSync is the gate for #952.
// Before it, `.kiro/hooks/*.json` was emit-only: a sync wrote the files
// and `import kiro` walked straight past them, so a repo synced to Kiro
// could not read back its own hooks.
//
// The multi-command spec is the load-bearing case. The emit side splits
// one spec's `command:` list into one `hooks[]` entry per command,
// suffixing the names `-2` and `-3`. Without the importer's grouping
// rule, the round-trip returns three specs and the second sync writes
// three files where the first wrote one.
func TestKiroRoundTrip_HooksSurviveSyncImportSync(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	seedKiroHookRoundTripFixture(t, dir)

	runCmd(t, "sync", "-t", "kiro")
	first := snapshotKiroEmit(t, dir)
	if len(first) == 0 {
		t.Fatalf("first sync produced no kiro output")
	}
	multi, ok := first[".kiro/hooks/multi.json"]
	if !ok {
		t.Fatalf("first sync wrote no multi-command hook file: %v", sortedKeys(first))
	}
	if !strings.Contains(multi, `"multi-3"`) {
		t.Fatalf("multi-command hook did not split into suffixed entries:\n%s", multi)
	}

	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai")); err != nil {
		t.Fatal(err)
	}

	runCmd(t, "import", "kiro")

	hooks, err := os.ReadDir(filepath.Join(dir, ".agnostic-ai", "hooks"))
	if err != nil {
		t.Fatalf("import wrote no hook specs: %v", err)
	}
	if len(hooks) != 5 {
		var names []string
		for _, h := range hooks {
			names = append(names, h.Name())
		}
		t.Fatalf("imported %d hook specs, want 5: %v", len(hooks), names)
	}

	for _, p := range []string{".kiro", ".kiroignore", "AGENTS.md"} {
		if err := os.RemoveAll(filepath.Join(dir, p)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "sync", "-t", "kiro")
	second := snapshotKiroEmit(t, dir)

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

// seedKiroHookRoundTripFixture writes one spec per hook shape the kiro
// adapter emits: a plain command, a command list, a native agent
// action, a Stop-hook confirmation block, and a disabled hook whose
// explicit `timeout: 0` disables the vendor's 60-second default.
func seedKiroHookRoundTripFixture(t *testing.T, dir string) {
	t.Helper()
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(`version: 1
sources:
  hooks: .agnostic-ai/hooks
targets:
  - kiro
gitignore:
  enabled: false
`), 0o644))

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/hooks"), 0o755))
	write := func(name, body string) {
		must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/hooks", name+".yaml"), []byte(body), 0o644))
	}
	write("lint", `name: lint
event: PostFileSave
matcher: "\\.(ts|tsx)$"
description: Lint edited TypeScript.
command: npx eslint --fix
timeout: 30
`)
	write("multi", `name: multi
event: PostToolUse
matcher: Edit
command:
  - gofmt -l .
  - go vet ./...
  - golangci-lint run
`)
	write("review", `name: review
event: Stop
x-kiro:
  action:
    type: agent
    prompt: Summarize what changed in this session.
`)
	write("submit", `name: submit
event: Stop
command: ./submit.sh
x-kiro:
  confirm:
    question: Submit this session's results?
    confirmCommand: ./confirm-options.sh
    options:
      - id: submit
        label: Yes, submit
        run: true
      - id: dismiss
        label: Not this time
        run: false
`)
	write("guard", `name: guard
event: PreToolUse
matcher: Bash
command: ./guard.sh {{filePath}}
timeout: 0
disabled: true
`)
}

// snapshotKiroEmit reads every file the kiro adapter writes.
func snapshotKiroEmit(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, rel := range []string{".kiro", ".kiroignore", "AGENTS.md"} {
		full := filepath.Join(root, rel)
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
			out[rel] = string(data)
			continue
		}
		err = filepath.WalkDir(full, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			relPath, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out[filepath.ToSlash(relPath)] = string(data)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", full, err)
		}
	}
	return out
}
