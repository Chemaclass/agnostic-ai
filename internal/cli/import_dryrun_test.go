package cli

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestImport_DryRunListsPathsNotContents asserts the new planning shape
// for `import --dry-run`: one line per file path that would be written,
// no file bodies. Closes the issue where dry-run dumped every body.
func TestImport_DryRunListsPathsNotContents(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "## sliced\n\nbody\n")
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"),
		"version: 1\nsources:\n  rules: rules\n  agents: agents\n  skills: skills\n  hooks: hooks\n  mcps: mcps\n  commands: commands\ntargets: [claude]\n")

	stdout := captureStdout(t, func() {
		root := NewRootCmd("test")
		root.SetArgs([]string{"import", "claude", "--dry-run"})
		if err := root.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(stdout, "would write ") {
		t.Errorf("expected 'would write <path>' lines, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "dry-run:") {
		t.Errorf("expected dry-run summary line, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "--- ") {
		t.Errorf("dry-run should not dump file contents anymore, got:\n%s", stdout)
	}
	// The rule body must not leak into stdout.
	if strings.Contains(stdout, "body") {
		t.Errorf("file content leaked into dry-run output:\n%s", stdout)
	}
	// No file should actually be written.
	if _, err := os.Stat(filepath.Join(dir, "rules", "sliced.md")); !os.IsNotExist(err) {
		t.Errorf("dry-run wrote a file: %v", err)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()
	fn()
	_ = w.Close()
	<-done
	return buf.String()
}

// snapshotProject maps every file and directory under root to its bytes
// (directories map to a marker), so a before and after comparison
// catches a written file, a created directory, and a changed mode.
func snapshotProject(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			out[rel] = "<dir " + info.Mode().String() + ">"
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = info.Mode().String() + "\n" + string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return out
}

func assertProjectUnchanged(t *testing.T, before, after map[string]string) {
	t.Helper()
	for p, v := range after {
		if old, ok := before[p]; !ok {
			t.Errorf("dry-run created %s", p)
		} else if old != v {
			t.Errorf("dry-run changed %s", p)
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			t.Errorf("dry-run removed %s", p)
		}
	}
}

// A permissions spec used to bypass the dry-run gate and land on disk.
func TestImport_DryRunDoesNotWritePermissionsSpec(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [windsurf]\n")
	writeFile(t, filepath.Join(dir, ".devin", "config.json"), `{"permissions":{"allow":["Read(src/**)"]}}`)
	before := snapshotProject(t, dir)

	silence(t)
	if _, err := runCLI(t, "import", "windsurf", "--dry-run"); err != nil {
		t.Fatalf("import: %v", err)
	}

	assertProjectUnchanged(t, before, snapshotProject(t, dir))
}

// Every importer write must pass through importWriteFile, or dry-run
// writes it to disk and the diff preview cannot attribute it.
func TestImporters_WriteOnlyThroughImportWriteFile(t *testing.T) {
	files, err := filepath.Glob("import*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "import_write.go" || f == "import_preview.go" {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, call := range []string{"os.WriteFile(", "os.MkdirAll(", "os.Create(", "os.OpenFile("} {
			if strings.Contains(string(data), call) {
				t.Errorf("%s calls %s; use importWriteFile or importMkdirAll", f, call)
			}
		}
	}
}

// A later import stage reads a file an earlier stage only planned: codex
// quotes the frontmatter of the SKILL.md it just copied. Plain dry-run
// must plan it like a real import, keep path-only output, and write
// nothing (#1046).
func TestImport_DryRunPlansCodexSkillWithoutWriting(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
	writeFile(t, filepath.Join(dir, ".agents", "skills", "deploy", "SKILL.md"),
		"---\nname: deploy\ndescription: Ship it #fast\n---\n\nbody\n")
	before := snapshotProject(t, dir)

	stdout := captureStdout(t, func() {
		if _, err := runCLI(t, "import", "codex", "--dry-run"); err != nil {
			t.Errorf("import: %v", err)
		}
	})

	want := "  would write " + filepath.Join(".agnostic-ai", "skills", "deploy", "SKILL.md") + "\n"
	if !strings.Contains(stdout, want) {
		t.Errorf("missing %q in:\n%s", want, stdout)
	}
	if !strings.HasSuffix(stdout, "dry-run: 1 file(s) would be written\n") {
		t.Errorf("unexpected summary:\n%s", stdout)
	}
	assertProjectUnchanged(t, before, snapshotProject(t, dir))
}

// During a dry-run a write that resolves outside the copy is recorded
// but never reaches disk, so no path shape can lead it into the project.
func TestImportWriteFile_SkipsPathsOutsideSandbox(t *testing.T) {
	testutil.TempCwd(t)
	sandbox, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "SKILL.md")
	importSandbox = sandbox
	t.Cleanup(func() { importSandbox = "" })

	for _, p := range []string{outside, filepath.Join("..", "escape.md")} {
		if err := importWriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s reached disk outside the sandbox: %v", p, err)
		}
	}
	if err := importWriteFile("inside.md", []byte("x"), 0o644); err != nil {
		t.Fatalf("write inside: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sandbox, "inside.md")); err != nil {
		t.Errorf("write inside the sandbox did not land: %v", err)
	}
}
