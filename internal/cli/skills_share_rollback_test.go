package cli

import (
	"os"
	"path/filepath"
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
