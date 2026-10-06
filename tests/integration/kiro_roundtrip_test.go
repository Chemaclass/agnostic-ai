package integration

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestKiroRoundTrip_PromptsPreserveNativeTemplates(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	name := "agnostic-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = filepath.Join(packageDir, "..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	for _, layout := range []string{"default", "configured", "absolute-source", "absolute-native", "target-dir", "per-kind-precedence"} {
		t.Run(layout, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			source := ".agnostic-ai/commands"
			native := ".kiro/prompts"
			outputs := ""
			if layout != "default" {
				source = "portable/commands"
				native = ".kiro/custom-prompts"
				outputs = "outputs:\n  kiro:\n    commands-dir: " + native + "\n"
			}
			if layout == "absolute-source" {
				source = filepath.Join(t.TempDir(), "commands")
			}
			if layout == "absolute-native" {
				native = filepath.Join(t.TempDir(), "prompts")
				outputs = "outputs:\n  kiro:\n    commands-dir: " + filepath.ToSlash(native) + "\n"
			}
			if layout == "target-dir" {
				native = ".kiro-alt/prompts"
				outputs = "outputs:\n  kiro:\n    dir: .kiro-alt\n"
			}
			if layout == "per-kind-precedence" {
				outputs = "outputs:\n  kiro:\n    dir: .kiro-alt\n    commands-dir: " + native + "\n"
			}
			must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\ntargets: [kiro]\nsources:\n  commands: "+filepath.ToSlash(source)+"\n"+outputs+"gitignore:\n  enabled: false\n"), 0o644))
			const body = "\n\n# Review\n\nReview ${1} with ${10}; full input: $ARGUMENTS or ${@}.\n"
			nativeDir := native
			if !filepath.IsAbs(nativeDir) {
				nativeDir = filepath.Join(dir, nativeDir)
			}
			prompt := filepath.Join(nativeDir, "review.md")
			must(t, os.MkdirAll(filepath.Dir(prompt), 0o755))
			must(t, os.WriteFile(prompt, []byte(body), 0o644))
			for _, candidate := range []string{".kiro/prompts", ".kiro-alt/prompts"} {
				if candidate == native {
					continue
				}
				ignored := filepath.Join(dir, candidate, "ignored.md")
				must(t, os.MkdirAll(filepath.Dir(ignored), 0o755))
				must(t, os.WriteFile(ignored, []byte("This directory is not selected.\n"), 0o644))
			}
			run := func(args ...string) string {
				t.Helper()
				cmd := exec.Command(binary, args...)
				cmd.Dir = dir
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("%v: %v\n%s", args, err, out)
				}
				return string(out)
			}
			sourceDir := source
			if !filepath.IsAbs(sourceDir) {
				sourceDir = filepath.Join(dir, sourceDir)
			}
			imported := filepath.Join(sourceDir, "review.md")
			for _, flags := range [][]string{{"--dry-run"}, {"--dry-run", "--diff"}} {
				out := run(append([]string{"import", "kiro"}, flags...)...)
				if !strings.Contains(filepath.ToSlash(out), filepath.ToSlash(source+"/review.md")) {
					t.Errorf("preview did not propose the configured command destination:\n%s", out)
				}
				if _, err := os.Stat(sourceDir); !os.IsNotExist(err) {
					t.Errorf("preview created the commands source directory: %v", err)
				}
				got, err := os.ReadFile(prompt)
				if err != nil {
					t.Fatalf("read native prompt after preview: %v", err)
				}
				if string(got) != body {
					t.Errorf("preview changed native prompt:\n%s", got)
				}
			}
			run("import", "kiro")
			got, err := os.ReadFile(imported)
			if err != nil {
				t.Fatalf("read imported prompt: %v", err)
			}
			if string(got) != body {
				t.Errorf("native templates changed on import:\n%s", got)
			}
			if _, err := os.Stat(filepath.Join(sourceDir, "ignored.md")); !os.IsNotExist(err) {
				t.Errorf("import read the unconfigured native directory: %v", err)
			}
			must(t, os.Remove(prompt))
			run("sync", "-t", "kiro")
			first, err := os.ReadFile(prompt)
			if err != nil {
				t.Fatalf("sync did not recreate prompt: %v", err)
			}
			if emitted, ok := strings.CutPrefix(string(first), header.Line(header.FormatMarkdown)+"\n"); !ok || emitted != body {
				t.Errorf("emitted prompt body changed: got %q, want %q", emitted, body)
			}
			for _, template := range []string{"${1}", "${10}", "$ARGUMENTS", "${@}"} {
				if !strings.Contains(string(first), template) {
					t.Errorf("emitted prompt lost %s:\n%s", template, first)
				}
			}
			run("sync", "--check", "-t", "kiro")
			run("lint")
			must(t, os.Remove(imported))
			run("import", "kiro")
			got, err = os.ReadFile(imported)
			if err != nil {
				t.Fatalf("read reimported prompt: %v", err)
			}
			if string(got) != body {
				t.Errorf("reimport changed content or retained generated metadata:\n%s", got)
			}
			must(t, os.Remove(prompt))
			run("sync", "-t", "kiro")
			second, err := os.ReadFile(prompt)
			if err != nil {
				t.Fatal(err)
			}
			if string(first) != string(second) {
				t.Errorf("prompt emit is not a fixed point:\n%s", unifiedDiffLines(string(first), string(second)))
			}
			run("sync", "--check", "-t", "kiro")
			run("lint")
		})
	}
}

func TestKiroRoundTrip_PromptsPreserveLeadingYAML(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	name := "agnostic-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = filepath.Join(packageDir, "..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	must(t, os.WriteFile("agnostic-ai.yaml", []byte("version: 1\ntargets: [kiro]\ngitignore:\n  enabled: false\n"), 0o644))
	const body = "---\nname: other\n---\nReview $ARGUMENTS.\n"
	native := filepath.Join(dir, ".kiro/prompts/review.md")
	must(t, os.MkdirAll(filepath.Dir(native), 0o755))
	must(t, os.WriteFile(native, []byte(body), 0o644))
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	run("import", "kiro")
	sourceDir := filepath.Join(dir, ".agnostic-ai/commands")
	if _, err := os.Stat(filepath.Join(sourceDir, "review.md")); err != nil {
		t.Fatalf("missing imported review.md: %v", err)
	}
	must(t, os.WriteFile(filepath.Join(sourceDir, "source-review.md"), []byte("---\nname: source-review\n---\n\n"+body), 0o644))
	must(t, os.Remove(native))
	run("sync", "-t", "kiro")
	first := map[string]string{}
	for _, command := range []string{"review", "source-review"} {
		path := filepath.Join(dir, ".kiro/prompts", command+".md")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("sync lost filename-based prompt %s: %v", command, err)
		}
		if got, ok := strings.CutPrefix(string(data), header.Line(header.FormatMarkdown)+"\n"); !ok || got != body {
			t.Errorf("%s lost literal leading YAML:\ngot:\n%s\nwant:\n%s", command, got, body)
		}
		first[command] = string(data)
		must(t, os.Remove(filepath.Join(sourceDir, command+".md")))
	}
	if _, err := os.Stat(filepath.Join(dir, ".kiro/prompts/other.md")); !os.IsNotExist(err) {
		t.Errorf("literal YAML created an unintended other.md prompt: %v", err)
	}
	run("import", "kiro")
	for _, command := range []string{"review", "source-review"} {
		must(t, os.Remove(filepath.Join(dir, ".kiro/prompts", command+".md")))
	}
	run("sync", "-t", "kiro")
	for _, command := range []string{"review", "source-review"} {
		data, err := os.ReadFile(filepath.Join(dir, ".kiro/prompts", command+".md"))
		if err != nil {
			t.Fatalf("reimport lost filename-based prompt %s: %v", command, err)
		}
		if string(data) != first[command] {
			t.Errorf("%s changed across reimport/sync:\n%s", command, unifiedDiffLines(first[command], string(data)))
		}
	}
	run("sync", "--check", "-t", "kiro")
	run("lint")
}

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
