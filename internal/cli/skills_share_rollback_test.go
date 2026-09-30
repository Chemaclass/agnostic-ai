package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Turning shared-skills off makes sync unlink a folder before any target
// writes the real copy there.
func TestSync_FailedGitignoreWriteRestoresUnlinkedSkillFolder(t *testing.T) {
	const ignored = "gitignore:\n  enabled: true\n"
	for _, tc := range []struct {
		name    string
		args    []string
		relinks bool
	}{
		{"text", []string{"sync"}, true},
		// The JSON path does not roll back writes, so the copy cursor
		// wrote in place of the link stays.
		{"json", []string{"sync", "--json"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := setupSharedSkillsFixture(t, sharedSkillsCfg+ignored)
			testutil.Chdir(t, dir)
			silence(t)
			if err := runSync(t); err != nil {
				t.Fatal(err)
			}
			link, err := os.Readlink(cursorGreet)
			if err != nil {
				t.Skipf("shared-skills kept a real copy: %v", err)
			}
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex, cursor]\n"+ignored)
			if err := os.Remove(".gitignore"); err != nil {
				t.Fatal(err)
			}
			// A directory fails the write on every OS, even for root.
			if err := os.Mkdir(".gitignore", 0o755); err != nil {
				t.Fatal(err)
			}

			if out, err := runCLI(t, tc.args...); err == nil {
				t.Fatalf("sync succeeded with .gitignore a directory:\n%s", out)
			}

			if got, err := os.Readlink(cursorGreet); tc.relinks && (err != nil || got != link) {
				t.Errorf("link not restored: %q, %v; want %q", got, err, link)
			}
			if _, err := os.Stat(filepath.Join(cursorGreet, "SKILL.md")); err != nil {
				t.Errorf("cursor lost its skill: %v", err)
			}
		})
	}
}

// A kept link to the wrong folder gives way to the copy cursor writes,
// and the copy to a link to the canonical folder. Undoing the copy
// through that link would delete the canonical files.
func TestSync_FailedGitignoreWriteRestoresRepointedLink(t *testing.T) {
	const canonical = ".agents/skills/greet/SKILL.md"
	for _, tc := range []struct {
		name, targets string
		edit          bool
	}{
		{"unchanged skill", "codex, cursor", false},
		{"unchanged skill, cursor first", "cursor, codex", false},
		{"edited skill", "codex, cursor", true},
		{"edited skill, cursor first", "cursor, codex", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const ignored = "gitignore:\n  enabled: true\n"
			dir := setupSharedSkillsFixture(t, "version: 1\ntargets: ["+tc.targets+"]\nsync:\n  shared-skills: true\n"+ignored)
			testutil.Chdir(t, dir)
			silence(t)
			if err := runSync(t); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Readlink(cursorGreet); err != nil {
				t.Skipf("shared-skills kept a real copy: %v", err)
			}
			body := readFile(t, canonical)
			mustWriteFile(t, "vendor/greet/SKILL.md", "vendored\n")
			wrong := filepath.Join("..", "..", "vendor", "greet")
			if err := os.Remove(cursorGreet); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(wrong, cursorGreet); err != nil {
				t.Fatal(err)
			}
			if tc.edit {
				mustWriteFile(t, ".agnostic-ai/skills/greet.md", "---\nname: greet\ndescription: Say hi\n---\nGreet the user warmly.\n")
			}
			if err := os.Remove(".gitignore"); err != nil {
				t.Fatal(err)
			}
			// A directory fails the write on every OS, even for root.
			if err := os.Mkdir(".gitignore", 0o755); err != nil {
				t.Fatal(err)
			}

			var out string
			var err error
			stderr := captureStderr(t, func() { out, err = runCLI(t, "sync") })

			if err == nil {
				t.Fatalf("sync succeeded with .gitignore a directory:\n%s", out)
			}
			if strings.Contains(stderr, "rollback:") {
				t.Errorf("rollback failed:\n%s", stderr)
			}
			if got, err := os.ReadFile(canonical); err != nil || string(got) != body {
				t.Errorf("canonical skill = %q, %v; want %q", got, err, body)
			}
			if got, err := os.Readlink(cursorGreet); err != nil || got != wrong {
				t.Errorf("link = %q, %v; want %q", got, err, wrong)
			}
		})
	}
}

func TestSync_FailedReconcileRestoresRemovedLink(t *testing.T) {
	const dangling = ".cursor/skills/b"
	dir := setupSharedSkillsFixture(t, "version: 1\ntargets: [codex, cursor]\nsync:\n  unmanaged:\n    - "+dangling+"/notes.md\n")
	testutil.Chdir(t, dir)
	silence(t)
	mustWriteFile(t, ".agents/skills/a/SKILL.md", "body\n")
	const stale = ".cursor/skills/a"
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join("..", "..", ".agents", "skills", "a")
	if err := os.Symlink(target, stale); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	// An owned file makes reconcile copy what the link resolves to, which
	// fails once the link dangles.
	if err := os.Symlink(filepath.Join("..", "..", ".agents", "skills", "missing"), dangling); err != nil {
		t.Fatal(err)
	}
	if err := writeStateFile(".", 0, "", "", syncLedger{outputs: []string{stale, dangling}}); err != nil {
		t.Fatal(err)
	}

	if out, err := runCLI(t, "sync"); err == nil {
		t.Fatalf("sync succeeded with a dangling owned link:\n%s", out)
	}

	if got, err := os.Readlink(stale); err != nil || got != target {
		t.Errorf("link not restored: %q, %v; want %q", got, err, target)
	}
}
