package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestInstallHookPostCheckout_WritesPostRewrite(t *testing.T) {
	dir := setupGitRepo(t)
	testutil.Chdir(t, dir)

	out, err := runInstallHook("--post-checkout")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".git", "hooks", "post-rewrite")
	if !strings.Contains(out, "installed "+filepath.Join(".git", "hooks", "post-rewrite")) {
		t.Errorf("output %q does not report post-rewrite", out)
	}
	got := readHook(t, path)
	for _, want := range []string{"#!/bin/sh\n", postRewriteHook.sentinel + "\n", `[ "$1" = "rebase" ] || exit 0`, "\n\"$agnostic_ai_bin\" project"} {
		if !strings.Contains(got, want) {
			t.Errorf("post-rewrite missing %q:\n%s", want, got)
		}
	}
	if _, err := runInstallHook("--post-checkout"); err != nil {
		t.Fatal(err)
	}
	if again := readHook(t, path); again != got {
		t.Errorf("a second install changed post-rewrite:\n%s", again)
	}
}

// A rebase, pull --rebase included, runs post-rewrite; an amend does not
// sync, since it switches no branch.
func TestInstallHookPostRewrite_SyncsAfterARebaseOnly(t *testing.T) {
	dir := setupGitRepo(t)
	testutil.Chdir(t, dir)
	writeFile(t, "agnostic-ai.yaml", "version: 1\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "base")
	git(t, dir, "checkout", "-q", "-b", "topic")
	writeFile(t, "a.txt", "topic")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "topic")
	git(t, dir, "checkout", "-q", "-")
	writeFile(t, "b.txt", "main")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "main")
	if _, err := runInstallHook("--post-checkout"); err != nil {
		t.Fatal(err)
	}
	bin, log := fakeAgnosticAI(t)
	if out, err := runGitWithFakeCLI(t, dir, bin, "commit", "-q", "--amend", "-m", "main amended"); err != nil {
		t.Fatalf("amend: %v\n%s", err, out)
	}
	if calls, _ := os.ReadFile(log); len(calls) != 0 {
		t.Errorf("an amend ran sync:\n%s", calls)
	}
	main := git(t, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if out, err := runGitWithFakeCLI(t, dir, bin, "checkout", "-q", "topic"); err != nil {
		t.Fatalf("checkout: %v\n%s", err, out)
	}
	if err := os.Truncate(log, 0); err != nil {
		t.Fatal(err)
	}
	if out, err := runGitWithFakeCLI(t, dir, bin, "-c", "rebase.backend=merge", "rebase", "-q", main); err != nil {
		t.Fatalf("rebase: %v\n%s", err, out)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	// The rebase's own checkout may run post-checkout too; post-rewrite
	// must add the last sync, on the rebased tree.
	lines := strings.Split(strings.TrimSpace(string(calls)), "\n")
	if lines[len(lines)-1] != "project cwd="+root {
		t.Errorf("calls after rebase = %q, want a final %q", calls, "project cwd="+root)
	}
}

func TestInstallHook_HookManagerGetsASnippetAndNoHook(t *testing.T) {
	cases := []struct {
		file, want, wantCheck string
		dir                   bool
	}{
		{file: "lefthook.yml", want: `run: '[ "{1}" = rebase ] || exit 0; agnostic-ai project'`, wantCheck: "agnostic-ai-check:"},
		{file: ".husky", want: "# .husky/post-rewrite\n[ \"$1\" = rebase ] || exit 0\nagnostic-ai project", wantCheck: "# .husky/pre-commit", dir: true},
		{file: ".pre-commit-config.yaml", want: "stages: [post-checkout, post-merge, post-rewrite]", wantCheck: "entry: agnostic-ai project --check --against index"},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			dir := setupGitRepo(t)
			testutil.Chdir(t, dir)
			if tc.dir {
				if err := os.Mkdir(tc.file, 0o755); err != nil {
					t.Fatal(err)
				}
			} else {
				writeFile(t, tc.file, "")
			}
			out, err := runInstallHook("--post-checkout")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, tc.want) || !strings.Contains(out, "install-hook --force") || !strings.Contains(out, tc.file) {
				t.Errorf("output lacks the snippet %q:\n%s", tc.want, out)
			}
			out, err = runInstallHook()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, tc.wantCheck) {
				t.Errorf("pre-commit output lacks %q:\n%s", tc.wantCheck, out)
			}
			for _, name := range []string{"pre-commit", "post-checkout", "post-merge", "post-rewrite"} {
				if _, err := os.Stat(filepath.Join(dir, ".git", "hooks", name)); !os.IsNotExist(err) {
					t.Errorf("%s written despite %s: %v", name, tc.file, err)
				}
			}
		})
	}
}

func TestInstallHook_ForceWritesHooksDespiteAHookManager(t *testing.T) {
	dir := setupGitRepo(t)
	testutil.Chdir(t, dir)
	writeFile(t, "lefthook.yml", "")

	if _, err := runInstallHook("--post-checkout", "--force"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"post-checkout", "post-merge", "post-rewrite"} {
		if got := readHook(t, filepath.Join(dir, ".git", "hooks", name)); !strings.Contains(got, "\"$agnostic_ai_bin\" project") {
			t.Errorf("%s lacks the sync:\n%s", name, got)
		}
	}
	if _, err := runInstallHook("--scope", "user", "--force"); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Errorf("--scope user with --force: %v, want a refusal", err)
	}
}
