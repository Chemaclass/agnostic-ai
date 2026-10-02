package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestProjectCommands_RefuseRunningCommand(t *testing.T) {
	for _, args := range [][]string{{"sync", "-t", "claude"}, {"import", "claude"}, {"use", "codex"}, {"init", "--all"}} {
		t.Run(args[0], func(t *testing.T) {
			dir := setupFixture(t)
			testutil.Chdir(t, dir)
			captureLogOut(t)
			startProjectLockHelper(t, dir, "hold")
			before, err := os.ReadFile("agnostic-ai.yaml")
			if err != nil {
				t.Fatal(err)
			}
			root := NewRootCmd("test")
			root.SetArgs(args)
			root.SetIn(strings.NewReader(""))
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			err = root.Execute()
			if err == nil || !strings.Contains(err.Error(), "project is locked by agnostic-ai use") {
				t.Errorf("%v: got %v, want the running use command named", args, err)
			}
			after, err := os.ReadFile("agnostic-ai.yaml")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Error("contending command changed project targets")
			}
			if _, err := os.Stat(stateFilePath(dir)); !os.IsNotExist(err) {
				t.Errorf("contending command wrote state: %v", err)
			}
		})
	}
}

func TestProjectCommands_RefreshRuntimeIgnoresOnUpgrade(t *testing.T) {
	for _, args := range [][]string{{"import", "claude"}, {"sync", "-t", "claude"}, {"sync", "--json", "-t", "claude"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			dir := setupFixture(t)
			testutil.Chdir(t, dir)
			captureLogOut(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\ngitignore:\n  enabled: false\n")
			oldBlock := "node_modules/\n# Personal ignore.\nprivate.txt\n\n" + gitignoreBlockStart + "\n" + gitignoreBlockNote + "\n" + gitignoreBlockHint + "\n/.agnostic-ai/.sync-state\n/.agnostic-ai/packs/\n/.agnostic-ai/local/\n/agnostic-ai.local.yaml\n# Keep this managed comment.\n!/fixtures/\n" + gitignoreBlockEnd + "\n\n# Personal exception.\n!important.txt\n"
			mustWriteFile(t, ".gitignore", oldBlock)
			if args[0] == "import" {
				writeScopedFile(t, ".claude/skills/demo/SKILL.md", "---\nname: demo\ndescription: Demo.\n---\n\nSkill body.\n")
			}
			root := NewRootCmd("test")
			root.SetArgs(args)
			root.SetIn(strings.NewReader(""))
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			if err := root.Execute(); err != nil {
				t.Fatalf("%v: %v", args, err)
			}
			data, err := os.ReadFile(".gitignore")
			if err != nil {
				t.Fatal(err)
			}
			lockEntry := "/.agnostic-ai/" + projectLockName + "\n"
			if !strings.Contains(string(data), lockEntry) {
				t.Errorf("writing command left its runtime lock visible to git: %s", data)
			}
			if strings.ReplaceAll(string(data), lockEntry, "") != oldBlock {
				t.Errorf("runtime ignore refresh changed existing ignore contents:\n%s", data)
			}
			if strings.Contains(string(data), "/.claude/rules/") || strings.Contains(string(data), "/CLAUDE.md") {
				t.Errorf("runtime ignore refresh enabled output ignores: %s", data)
			}
		})
	}
}

func TestProjectCommands_NestedWritesUseOneLock(t *testing.T) {
	for _, args := range [][]string{{"init", "--all", "--from", "claude"}, {"use", "claude"}} {
		t.Run(args[0], func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			captureLogOut(t)
			writeScopedFile(t, ".claude/skills/demo/SKILL.md", "---\nname: demo\ndescription: Demo.\n---\n\nSkill body.\n")
			root := NewRootCmd("test")
			root.SetArgs(args)
			root.SetIn(strings.NewReader(""))
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			if err := root.Execute(); err != nil {
				t.Fatalf("%v: %v", args, err)
			}
			data, err := os.ReadFile(filepath.Join(".agnostic-ai", "skills", "demo", "SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), "Skill body.") {
				t.Errorf("nested import lost the native skill body: %s", data)
			}
			lock, err := acquireProjectLock(".", "sync")
			if err != nil {
				t.Fatalf("lock leaked after nested writes: %v", err)
			}
			_ = lock.Close()
		})
	}
}

func TestProjectLock_KeepsTheSameFileAfterRelease(t *testing.T) {
	dir := t.TempDir()
	first, err := acquireProjectLock(dir, "init")
	if err != nil {
		t.Fatal(err)
	}
	before, err := first.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := acquireProjectLock(dir, "use")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	after, err := second.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Error("releasing the lock replaced its file")
	}
	other, err := acquireProjectLock(dir, "sync")
	if other != nil {
		_ = other.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "agnostic-ai use") {
		t.Errorf("reused-file contention: %v, want the new use holder named", err)
	}
}

func TestProjectPreviews_RunWhileAnotherCommandHoldsLock(t *testing.T) {
	for _, args := range [][]string{{"sync", "--check", "-t", "claude"}, {"sync", "--plan", "-t", "claude"}, {"sync", "--dry-run", "-t", "claude"}, {"import", "claude", "--dry-run"}, {"import", "claude", "--dry-run", "--diff"}, {"status"}, {"list"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			dir := setupFixture(t)
			testutil.Chdir(t, dir)
			captureLogOut(t)
			lock, err := acquireProjectLock(dir, "use")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = lock.Close() }()
			before, err := lock.Stat()
			if err != nil {
				t.Fatal(err)
			}
			root := NewRootCmd("test")
			root.SetArgs(args)
			root.SetIn(strings.NewReader(""))
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			err = root.Execute()
			if err != nil && (strings.Contains(err.Error(), "project is locked") || strings.Contains(err.Error(), "copy project for preview")) {
				t.Errorf("read-only command contended with the writer: %v", err)
			}
			after, err := lock.Stat()
			if err != nil {
				t.Fatal(err)
			}
			if !before.ModTime().Equal(after.ModTime()) {
				t.Error("read-only command changed the lock holder")
			}
		})
	}
}

func TestProjectLock_ReleasesAfterKilledHolder(t *testing.T) {
	dir := setupFixture(t)
	proc := startProjectLockHelper(t, dir, "hold")
	if err := proc.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = proc.Wait()
	testutil.Chdir(t, dir)
	captureLogOut(t)
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	if err := root.Execute(); err != nil {
		t.Fatalf("sync after killed holder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "rules", "r1.md")); err != nil {
		t.Errorf("sync did not emit the rule after holder died: %v", err)
	}
}

func TestProjectLock_WatchHoldsLockUntilKilled(t *testing.T) {
	dir := setupFixture(t)
	proc := startProjectLockHelper(t, dir, "watch")
	lock, err := acquireProjectLock(dir, "import")
	if lock != nil {
		_ = lock.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "agnostic-ai sync --watch") {
		t.Errorf("watch contention: %v, want sync --watch named", err)
	}
	if err := proc.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = proc.Wait()
	// A git child forked by the watch's sync shares the locked file until it execs.
	deadline := time.Now().Add(5 * time.Second)
	for {
		lock, err = acquireProjectLock(dir, "import")
		if err == nil || !strings.Contains(err.Error(), "project is locked") || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("acquire after watch died: %v", err)
	}
	_ = lock.Close()
}

func TestProjectLock_SymlinkAliasContends(t *testing.T) {
	dir := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(dir, alias); err != nil {
		t.Skipf("cannot create a symlink: %v", err)
	}
	lock, err := acquireProjectLock(dir, "init")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	other, err := acquireProjectLock(alias, "use")
	if other != nil {
		_ = other.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "agnostic-ai init") {
		t.Errorf("alias contention: %v, want init named", err)
	}
}

func TestProjectLock_ReleasesOnCommandError(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	if err := root.Execute(); err == nil {
		t.Fatal("sync without config should fail")
	}
	lock, err := acquireProjectLock(".", "init")
	if err != nil {
		t.Fatalf("lock leaked after command error: %v", err)
	}
	_ = lock.Close()
}

func TestProjectLock_PlanNeverPersistsTheFirstSyncSelection(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	captureLogOut(t)
	before := "version: 1\ntargets: [" + strings.Join(allTargetNames(), ", ") + "]\n"
	mustWriteFile(t, "agnostic-ai.yaml", before)
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--plan"})
	root.SetIn(strings.NewReader("claude\n"))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile("agnostic-ai.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != before {
		t.Errorf("plan persisted a target selection: %s", after)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", projectLockName)); !os.IsNotExist(err) {
		t.Errorf("plan created a project lock: %v", err)
	}
}

func TestProjectPreviews_DoNotTouchLock(t *testing.T) {
	for _, args := range [][]string{{"sync", "--check", "-t", "claude"}, {"sync", "--plan", "-t", "claude"}, {"sync", "--dry-run", "-t", "claude"}, {"import", "claude", "--dry-run"}, {"init", "--all", "--dry-run"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			dir := setupFixture(t)
			if args[0] == "init" {
				dir = t.TempDir()
			}
			testutil.Chdir(t, dir)
			captureLogOut(t)
			root := NewRootCmd("test")
			root.SetArgs(args)
			root.SetIn(strings.NewReader(""))
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			_ = root.Execute()
			if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", ".command-lock")); !os.IsNotExist(err) {
				t.Errorf("preview created a project lock: %v", err)
			}
			if args[0] == "init" {
				if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai")); !os.IsNotExist(err) {
					t.Errorf("init preview created the specs directory: %v", err)
				}
			}
		})
	}
}

func startProjectLockHelper(t *testing.T, dir, mode string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestProjectLockHelperProcess$")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "AGNOSTIC_AI_PROJECT_LOCK_HELPER="+mode)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	ready, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	result := make(chan error, 1)
	go func() {
		marker := make([]byte, len("ready\n"))
		_, err := io.ReadFull(ready, marker)
		if err == nil && string(marker) != "ready\n" {
			err = fmt.Errorf("unexpected helper output %q", marker)
		}
		result <- err
	}()
	select {
	case err := <-result:
		if err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			t.Fatalf("lock helper readiness: %v: %s", err, &stderr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("lock helper did not become ready")
	}
	return cmd
}

func TestProjectLockHelperProcess(t *testing.T) {
	switch os.Getenv("AGNOSTIC_AI_PROJECT_LOCK_HELPER") {
	case "hold":
		lock, err := acquireProjectLock(".", "use")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = lock.Close() }()
		fmt.Println("ready")
		for {
			time.Sleep(time.Second)
		}
	case "watch":
		go func() {
			for {
				if _, err := os.Stat(filepath.Join(".claude", "rules", "r1.md")); err == nil {
					fmt.Println("ready")
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
		}()
		logOut = io.Discard
		root := NewRootCmd("test")
		root.SetArgs([]string{"sync", "--watch", "--watch-poll", "-t", "claude"})
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
	}
}
