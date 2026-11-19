package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsolatedAuditSurvivesSourceCheckoutChanges(t *testing.T) {
	t.Parallel()
	a, _ := fixture(t)
	out := filepath.Join(t.TempDir(), "audit-worktree")
	call(t, a, "isolate", "--out", out)
	isolated := app{root: out, lock: filepath.Join(out, "scripts/target-audit/sources.lock")}
	run := call(t, isolated, "start", "--builtins-since", "HEAD")
	if err := os.WriteFile(filepath.Join(a.root, "internal/builtins/data/skill.md"), []byte("concurrent change"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"switch", "-c", "concurrent-work"}, {"add", "."}, {"-c", "user.name=Test", "-c", "user.email=test@example.test", "-c", "commit.gpgsign=false", "commit", "-qm", "concurrent change"}} {
		if _, err := a.command("git", args...); err != nil {
			t.Fatal(err)
		}
	}
	fetched(t, isolated, run, "changed")
	covered(t, isolated, run, false)
	call(t, isolated, "finalize", run)
	if _, err := os.Stat(filepath.Join(a.root, "local/target-audit", filepath.Base(run), "audit.json")); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(filepath.Join(a.root, "local/target-audit/go.mod")); err != nil {
		t.Error(err)
	}
	lock, err := os.ReadFile(a.lock)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(lock), "2026-10-16") {
		t.Error("source lock changed")
	}
}

func TestIsolateRequiresExplicitCommitForDirtyInputs(t *testing.T) {
	t.Parallel()
	a, _ := fixture(t)
	if err := os.WriteFile(filepath.Join(a.root, "untracked"), []byte("work"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "isolated")
	if _, err := a.run([]string{"isolate", "--out", out}); err == nil {
		t.Error("dirty source accepted implicitly")
	}
	call(t, a, "isolate", "--out", out, "--commit", "HEAD")
	if _, err := os.Stat(filepath.Join(out, "untracked")); !os.IsNotExist(err) {
		t.Error("unexpected working input")
	}
}

func TestIsolatedAuditRejectsChangedResearchInputs(t *testing.T) {
	t.Parallel()
	a, _ := fixture(t)
	out := filepath.Join(t.TempDir(), "audit-worktree")
	call(t, a, "isolate", "--out", out)
	isolated := app{root: out, lock: filepath.Join(out, "scripts/target-audit/sources.lock")}
	run := call(t, isolated, "start", "--builtins-since", "HEAD")
	fetched(t, isolated, run, "changed")
	covered(t, isolated, run, false)
	if err := os.WriteFile(filepath.Join(out, "internal/builtins/data/skill.md"), []byte("changed research"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(isolated.lock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = isolated.run([]string{"finalize", run}); err == nil {
		t.Error("changed research finalized")
	}
	after, err := os.ReadFile(isolated.lock)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("rejected finalization changed lock")
	}
}

func TestIsolatedAuditKeepsEvidenceAfterWorktreeRemoval(t *testing.T) {
	t.Parallel()
	a, _ := fixture(t)
	out := filepath.Join(t.TempDir(), "audit-worktree")
	call(t, a, "isolate", "--out", out)
	isolated := app{root: out, lock: filepath.Join(out, "scripts/target-audit/sources.lock")}
	run := call(t, isolated, "start", "--builtins-since", "HEAD")
	fetched(t, isolated, run, "changed")
	covered(t, isolated, run, false)
	call(t, isolated, "finalize", run)
	for _, args := range [][]string{{"switch", "-c", "audit-delivery"}, {"add", "scripts/target-audit/sources.lock"}, {"-c", "user.name=Test", "-c", "user.email=test@example.test", "-c", "commit.gpgsign=false", "commit", "-qm", "audit lock"}} {
		if _, err := isolated.command("git", args...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.command("git", "worktree", "remove", out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(run, "report.md")); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(filepath.Join(run, "pages/alpha-docs.body")); err != nil {
		t.Error(err)
	}
	if _, err := a.command("git", "rev-parse", "--verify", "audit-delivery"); err != nil {
		t.Error("delivery commit lost:", err)
	}
}

func TestIsolateRejectsImplicitlyDroppingIgnoredBuiltins(t *testing.T) {
	t.Parallel()
	a, _ := fixture(t)
	ignored := filepath.Join(a.root, "internal/builtins/data/ignored.bin")
	if err := os.WriteFile(filepath.Join(a.root, ".git/info/exclude"), []byte("ignored.bin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ignored, []byte("embedded input"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.run([]string{"isolate", "--out", filepath.Join(t.TempDir(), "audit")}); err == nil {
		t.Error("ignored built-in omitted without explicit commit")
	}
}

func TestIsolatedAuditResumesWithinSameWorktree(t *testing.T) {
	t.Parallel()
	a, _ := fixture(t)
	out := filepath.Join(t.TempDir(), "audit")
	call(t, a, "isolate", "--out", out)
	isolated := app{root: out, lock: filepath.Join(out, "scripts/target-audit/sources.lock")}
	prior := call(t, isolated, "start", "--builtins-since", "HEAD")
	fetched(t, isolated, prior, "changed")
	covered(t, isolated, prior, false)
	next := call(t, isolated, "start", "--builtins-since", "HEAD")
	fetched(t, isolated, next, "changed")
	call(t, isolated, "resume", next, prior)
	call(t, isolated, "finalize", next)
}

func TestStartKeepsSavedGoEvidenceOutOfTheCheckoutModule(t *testing.T) {
	t.Parallel()
	a, _ := fixture(t)
	for name, body := range map[string]string{"go.mod": "module example.test/checkout\n\ngo 1.21\n", "main.go": "package main\n\nfunc main() {}\n"} {
		if err := os.WriteFile(filepath.Join(a.root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run := call(t, a, "start", "--builtins-since", "HEAD")
	if _, err := os.Stat(filepath.Join(a.root, "local/target-audit/go.mod")); err != nil {
		t.Fatal(err)
	}
	evidence := filepath.Join(run, "crush-evidence")
	if err := os.MkdirAll(evidence, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, pkg := range map[string]string{"a.go": "agent", "b.go": "config"} {
		if err := os.WriteFile(filepath.Join(evidence, name), []byte("package "+pkg+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("go", "list", "./...")
	cmd.Dir = a.root
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOWORK=off", "GOTOOLCHAIN=local", "GOMODCACHE="+t.TempDir(), "GOCACHE="+t.TempDir())
	out, err := cmd.CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "example.test/checkout" {
		t.Fatalf("go list ./...: %v\n%s", err, out)
	}
}

func TestStartKeepsAnExistingEvidenceGoMod(t *testing.T) {
	t.Parallel()
	a, _ := fixture(t)
	path := filepath.Join(a.root, "local/target-audit/go.mod")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	want := "module mine\n\ngo 1.27\n"
	if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}
	call(t, a, "start", "--builtins-since", "HEAD")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("go.mod = %q, want %q", got, want)
	}
}

func TestStartWritesTheEvidenceGoModForAnExplicitRunInsideIt(t *testing.T) {
	t.Parallel()
	a, _ := fixture(t)
	call(t, a, "start", "--builtins-since", "HEAD", "--out", filepath.Join(a.root, "local/target-audit/named-run"))
	if _, err := os.Stat(filepath.Join(a.root, "local/target-audit/go.mod")); err != nil {
		t.Error(err)
	}
}

func TestStartLeavesTheEvidenceFolderAloneForARunOutsideIt(t *testing.T) {
	t.Parallel()
	a, _ := fixture(t)
	call(t, a, "start", "--builtins-since", "HEAD", "--out", filepath.Join(t.TempDir(), "run"))
	if _, err := os.Stat(filepath.Join(a.root, "local/target-audit")); !os.IsNotExist(err) {
		t.Errorf("evidence folder created: %v", err)
	}
}
