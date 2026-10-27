package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

var handoffHookTargets = []string{"claude", "codex", "gemini", "qoder", "factory"}
var handoffHookPaths = map[string]string{
	"claude": ".claude/settings.json", "codex": ".codex/hooks.json", "gemini": ".gemini/settings.json", "qoder": ".qoder/settings.json", "factory": ".factory/hooks.json",
}

type handoffNativeHandler struct {
	Type           string            `json:"type"`
	Command        string            `json:"command"`
	CommandWindows string            `json:"commandWindows"`
	Shell          string            `json:"shell"`
	Env            map[string]string `json:"env"`
}
type handoffNativeGroup struct {
	Matcher string                 `json:"matcher"`
	Hooks   []handoffNativeHandler `json:"hooks"`
}

func TestBuiltinHandoffHooks(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Clean(filepath.Join(packageDir, "..", ".."))
	name := "agnostic-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-ldflags", "-X main.version=0.80.0 -X github.com/chemaclass/agnostic-ai/internal/cli.candidateVersion=0.80.0", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	fixture := handoffRead(t, filepath.Join(packageDir, "fixtures", "builtin-handoff-hook", "agnostic-ai.yaml"))
	run := func(t *testing.T, dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	project := func(t *testing.T) string {
		t.Helper()
		t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
		dir := filepath.Join(t.TempDir(), "checkout with ' apostrophe 雪")
		handoffWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), fixture)
		return dir
	}
	syncHooks := func(t *testing.T, dir string) map[string]map[string]handoffNativeHandler {
		t.Helper()
		run(t, dir, "sync", "--gitignore=off")
		return handoffReadHandlers(t, dir)
	}

	t.Run("claude-command-source-and-native-input", func(t *testing.T) {
		payload := `{"cwd":"C:\\checkout\\雪"}`
		handler := handoffNativeHandler{Command: `IFS= read -r payload
printf '%s\n' "$payload"
awk 'BEGIN { print "\\u"; if ("x" ~ /^[^"\\]+$/) print "matched" }'`}
		out := handoffExecute(t, "claude", handler, t.TempDir(), payload+"\n", "", "")
		if want := payload + "\n\\u\nmatched\n"; out != want {
			t.Errorf("command source or native input changed: got %q, want %q", out, want)
		}
	})

	t.Run("shared-auto-snapshots-survive-worktrees", func(t *testing.T) {
		main := project(t)
		handoffInitGit(t, main)
		handoffGit(t, main, "branch", "-M", "auto-main")
		handlers := syncHooks(t, main)
		active := filepath.Join(t.TempDir(), "automatic checkout")
		handoffGit(t, main, "worktree", "add", "-b", "feat/auto", active)
		handoffWrite(t, filepath.Join(active, "agnostic-ai.yaml"), fixture)
		path := filepath.Join(main, ".agnostic-ai", "local", "handoffs", "feat-auto.auto.md")
		for _, target := range handoffHookTargets {
			event := "PreCompact"
			if target == "gemini" {
				event = "PreCompress"
			}
			for _, event := range []string{event, "SessionEnd"} {
				handoffAssertReply(t, target, handoffExecute(t, target, handlers[target][event], active, "{}", "foreign", ""), "")
				snapshot := handoffRead(t, path)
				if !strings.Contains(snapshot, "branch: feat/auto") || !strings.Contains(snapshot, "worktree: "+strings.TrimSpace(handoffGit(t, active, "rev-parse", "--show-toplevel"))) || !strings.Contains(snapshot, "tool: "+target) {
					t.Errorf("shared snapshot header:\n%s", snapshot)
				}
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
					t.Errorf("snapshot mode = %o", info.Mode().Perm())
				}
			}
			handoffAssertReply(t, target, handoffExecute(t, target, handlers[target]["SessionStart"], active, "{}", "foreign", ""), "Git snapshot available: .agnostic-ai/local/handoffs/feat-auto.auto.md (branch feat/auto). Ask to resume the handoff.")
		}
		manual := "# Handoff\ntool: codex | branch: feat/auto | head: abc1234\nworktree: " + active + " | status: ready | pr: #123\n"
		handoffWrite(t, filepath.Join(filepath.Dir(path), "feat-auto.md"), manual)
		for _, target := range handoffHookTargets {
			handoffAssertReply(t, target, handoffExecute(t, target, handlers[target]["SessionStart"], main, "{}", "foreign", ""), "1 task handoff available. Ask to resume a task handoff.")
		}
		if got := handoffRead(t, filepath.Join(filepath.Dir(path), "feat-auto.md")); got != manual {
			t.Errorf("snapshot replaced manual task: %q", got)
		}
		handoffGit(t, main, "worktree", "remove", "--force", active)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("removed worktree lost automatic snapshot: %v", err)
		}
	})

	t.Run("automatic-snapshots-preserve-repository-boundaries", func(t *testing.T) {
		const protected = "# Automatic handoff\ntool: codex | branch: feat/probe | head: abc1234\nworktree: protected | detached: false\n\nPROTECTED_SENTINEL\n"
		cases := []struct {
			name        string
			relativeGit bool
			prepare     func(t *testing.T, main, store string)
		}{
			{name: "native-git-rejects-shell-paths", relativeGit: true},
			{name: "external-shared-store", prepare: func(t *testing.T, main, store string) {
				external := t.TempDir()
				handoffGit(t, external, "init")
				handoffWrite(t, filepath.Join(external, ".gitignore"), "*\n")
				handoffWrite(t, filepath.Join(external, "feat-probe.auto.md"), protected)
				if err := os.MkdirAll(filepath.Dir(store), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(external, store); err != nil {
					t.Skipf("directory symlinks unavailable: %v", err)
				}
			}},
			{name: "parent-tracked-note-with-nested-git", prepare: func(t *testing.T, main, store string) {
				handoffWrite(t, filepath.Join(store, "feat-probe.auto.md"), protected)
				handoffGit(t, main, "add", "-f", "--", ".agnostic-ai/local/handoffs/feat-probe.auto.md")
				handoffGit(t, store, "init")
				handoffWrite(t, filepath.Join(store, ".gitignore"), "*\n")
			}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				main := project(t)
				handoffInitGit(t, main)
				handlers := syncHooks(t, main)
				active := filepath.Join(t.TempDir(), "automatic probe checkout")
				handoffGit(t, main, "worktree", "add", "-b", "feat/probe", active)
				handoffWrite(t, filepath.Join(active, "agnostic-ai.yaml"), fixture)
				store := filepath.Join(main, ".agnostic-ai", "local", "handoffs")
				if tc.prepare != nil {
					tc.prepare(t, main, store)
				}
				shim, marker := "", ""
				if tc.relativeGit {
					shim, marker = handoffGitPathShim(t)
				}
				for _, target := range handoffHookTargets {
					event := "PreCompact"
					if target == "gemini" {
						event = "PreCompress"
					}
					for _, event := range []string{event, "SessionEnd"} {
						handoffAssertReply(t, target, handoffExecute(t, target, handlers[target][event], active, "{}", "foreign", shim), "")
						path := filepath.Join(store, "feat-probe.auto.md")
						if tc.prepare != nil {
							if got := handoffRead(t, path); got != protected {
								t.Errorf("%s %s changed protected snapshot: %q", target, event, got)
							}
							path = filepath.Join(active, ".agnostic-ai", "local", "handoffs", "feat-probe.auto.md")
						}
						if got := handoffRead(t, path); !strings.Contains(got, "branch: feat/probe") || !strings.Contains(got, "tool: "+target) {
							t.Errorf("%s %s snapshot header: %q", target, event, got)
						}
						if got := handoffRead(t, filepath.Join(active, ".agnostic-ai", "local", "HANDOFF.auto.md")); !strings.Contains(got, "tool: "+target) {
							t.Errorf("%s %s legacy snapshot header: %q", target, event, got)
						}
					}
				}
				if tc.relativeGit {
					if _, err := os.Stat(marker); err != nil {
						t.Errorf("native Git boundary was not exercised: %v", err)
					}
				}
			})
		}
	})

	t.Run("detached-snapshots-have-distinct-task-identities", func(t *testing.T) {
		main := project(t)
		handoffInitGit(t, main)
		handoffGit(t, main, "branch", "-M", "detached-main")
		handlers := syncHooks(t, main)["claude"]
		first := filepath.Join(t.TempDir(), "first detached")
		handoffGit(t, main, "worktree", "add", "--detach", first)
		firstHead := strings.TrimSpace(handoffGit(t, first, "rev-parse", "HEAD"))
		handoffWrite(t, filepath.Join(main, "tracked.txt"), "next detached commit\n")
		handoffGit(t, main, "add", "tracked.txt")
		handoffGit(t, main, "commit", "-m", "second detached head")
		second := filepath.Join(t.TempDir(), "second detached")
		handoffGit(t, main, "worktree", "add", "--detach", second)
		named := filepath.Join(t.TempDir(), "named detached branch")
		handoffGit(t, main, "worktree", "add", "-b", "detached", named)
		for _, dir := range []string{first, second, named} {
			handoffWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), fixture)
			handoffAssertReply(t, "claude", handoffExecute(t, "claude", handlers["SessionEnd"], dir, "{}", "foreign", ""), "")
		}
		handoffAssertReply(t, "claude", handoffExecute(t, "claude", handlers["SessionStart"], main, "{}", "foreign", ""), "3 task handoffs available. Ask to resume a task handoff.")
		handoffAssertReply(t, "claude", handoffExecute(t, "claude", handlers["SessionStart"], named, "{}", "foreign", ""), "Git snapshot available: .agnostic-ai/local/handoffs/detached.auto.md (branch detached). Ask to resume the handoff.")
		handoffAssertReply(t, "claude", handoffExecute(t, "claude", handlers["SessionStart"], first, "{}", "foreign", ""), "Git snapshot available: .agnostic-ai/local/handoffs/detached-"+firstHead+".auto.md (branch detached-"+firstHead+"). Ask to resume the handoff.")
	})

	t.Run("named-detached-path-marker-remains-path-text", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows paths cannot contain the colon in the literal marker")
		}
		main := project(t)
		handoffInitGit(t, main)
		handlers := syncHooks(t, main)
		active := filepath.Join(t.TempDir(), "named checkout | detached: true")
		handoffGit(t, main, "worktree", "add", "-b", "detached", active)
		handoffWrite(t, filepath.Join(active, "agnostic-ai.yaml"), fixture)
		path := filepath.Join(main, ".agnostic-ai", "local", "handoffs", "detached.auto.md")
		for _, target := range handoffHookTargets {
			handoffAssertReply(t, target, handoffExecute(t, target, handlers[target]["SessionEnd"], active, "{}", "foreign", ""), "")
			snapshot := handoffRead(t, path)
			want := "worktree: " + strings.TrimSpace(handoffGit(t, active, "rev-parse", "--show-toplevel")) + " | detached: false"
			if !strings.Contains(snapshot, want) {
				t.Errorf("named branch lacks final false marker:\n%s", snapshot)
			}
			handoffAssertReply(t, target, handoffExecute(t, target, handlers[target]["SessionStart"], active, "{}", "foreign", ""), "Git snapshot available: .agnostic-ai/local/handoffs/detached.auto.md (branch detached). Ask to resume the handoff.")
		}
	})

	t.Run("named-detached-branch-keeps-its-auto-file", func(t *testing.T) {
		dir := project(t)
		handoffInitGit(t, dir)
		handoffGit(t, dir, "branch", "-M", "detached")
		handler := syncHooks(t, dir)["claude"]["SessionEnd"]
		handoffAssertReply(t, "claude", handoffExecute(t, "claude", handler, dir, "{}", "foreign", ""), "")
		path := filepath.Join(dir, ".agnostic-ai", "local", "handoffs", "detached.auto.md")
		if snapshot := handoffRead(t, path); !strings.Contains(snapshot, "branch: detached") {
			t.Errorf("named branch snapshot:\n%s", snapshot)
		}
	})

	t.Run("shared-auto-unignored-store-falls-back", func(t *testing.T) {
		t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
		main := project(t)
		handoffInitGit(t, main)
		handlers := syncHooks(t, main)
		active := filepath.Join(t.TempDir(), "automatic fallback checkout")
		handoffGit(t, main, "worktree", "add", "-b", "feat/fallback", active)
		handoffWrite(t, filepath.Join(active, "agnostic-ai.yaml"), fixture)
		handoffWrite(t, filepath.Join(main, ".gitignore"), ".agnostic-ai/local/handoffs/*.auto.md\n.env\n")
		for _, target := range handoffHookTargets {
			handoffAssertReply(t, target, handoffExecute(t, target, handlers[target]["SessionEnd"], active, "{}", "foreign", ""), "")
			path := filepath.Join(active, ".agnostic-ai", "local", "handoffs", "feat-fallback.auto.md")
			if snapshot := handoffRead(t, path); !strings.Contains(snapshot, "worktree: "+strings.TrimSpace(handoffGit(t, active, "rev-parse", "--show-toplevel"))) {
				t.Errorf("fallback snapshot header:\n%s", snapshot)
			}
		}
		if _, err := os.Stat(filepath.Join(main, ".agnostic-ai", "local", "handoffs", "feat-fallback.auto.md")); !os.IsNotExist(err) {
			t.Errorf("snapshot reached unignored shared store: %v", err)
		}
	})

	t.Run("task-handoff-count-wording", func(t *testing.T) {
		main := project(t)
		handoffInitGit(t, main)
		handoffGit(t, main, "branch", "-M", "count-main")
		handlers := syncHooks(t, main)
		store := filepath.Join(main, ".agnostic-ai", "local", "handoffs")
		written := 0
		for _, want := range []struct {
			count   int
			message string
		}{
			{1, "1 task handoff available. Ask to resume a task handoff."},
			{3, "3 task handoffs available. Ask to resume a task handoff."},
		} {
			for ; written < want.count; written++ {
				branch := fmt.Sprintf("feat/count-%d", written)
				handoffWrite(t, filepath.Join(store, fmt.Sprintf("feat-count-%d.md", written)), "# Handoff\ntool: codex | branch: "+branch+" | head: abc1234\nworktree: "+main+" | status: active | pr: none\n")
			}
			for _, target := range handoffHookTargets {
				handoffAssertReply(t, target, handoffExecute(t, target, handlers[target]["SessionStart"], main, "{}", "foreign", ""), want.message)
			}
			windows := handlers["codex"]["SessionStart"]
			windows.Command = windows.CommandWindows
			handoffAssertReply(t, "codex", handoffExecute(t, "codex", windows, main, "{}", "foreign", ""), want.message)
		}
	})

	t.Run("task-handoffs-across-worktrees", func(t *testing.T) {
		main := project(t)
		handoffInitGit(t, main)
		handoffGit(t, main, "branch", "-M", "task-main")
		handlers := syncHooks(t, main)
		active := filepath.Join(t.TempDir(), "active checkout ' 雪")
		handoffGit(t, main, "worktree", "add", "-b", "feat/b", active)
		handoffWrite(t, filepath.Join(active, "agnostic-ai.yaml"), fixture)
		store := filepath.Join(main, ".agnostic-ai", "local", "handoffs")
		for _, branch := range []string{"feat/a", "feat/b"} {
			name := strings.ReplaceAll(branch, "/", "-") + ".md"
			handoffWrite(t, filepath.Join(store, name), "# Handoff\ntool: codex | date: 2026-10-13T12:00Z | branch: "+branch+" | head: abc1234\nworktree: "+active+" | status: active | pr: none\n\nTASK_CONTENT_MUST_NOT_LOAD\n")
		}
		shim, marker := handoffGitPathShim(t)
		for _, target := range handoffHookTargets {
			t.Run(target, func(t *testing.T) {
				handler := handlers[target]["SessionStart"]
				defer func() {
					if t.Failed() {
						handoffTraceStartup(t, target, handler, active)
						handoffTraceStartup(t, target, handler, main)
					}
				}()
				out := handoffExecute(t, target, handler, active, "{}", "foreign", "")
				handoffAssertReply(t, target, out, "Task handoff available: .agnostic-ai/local/handoffs/feat-b.md (branch feat/b). Ask to resume the handoff.")
				out = handoffExecute(t, target, handler, main, "{}", "foreign", "")
				handoffAssertReply(t, target, out, "2 task handoffs available. Ask to resume a task handoff.")
				if strings.Contains(out, "TASK_CONTENT_MUST_NOT_LOAD") {
					t.Error("startup read task contents")
				}
				t.Run("native-git-rejects-shell-paths", func(t *testing.T) {
					if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
						t.Fatal(err)
					}
					out := handoffExecute(t, target, handler, active, "{}", "foreign", shim)
					handoffAssertReply(t, target, out, "Task handoff available: .agnostic-ai/local/handoffs/feat-b.md (branch feat/b). Ask to resume the handoff.")
					out = handoffExecute(t, target, handler, main, "{}", "foreign", shim)
					handoffAssertReply(t, target, out, "2 task handoffs available. Ask to resume a task handoff.")
					if _, err := os.Stat(marker); err != nil {
						t.Errorf("native Git boundary was not exercised: %v", err)
					}
				})
			})
		}
		handoffGit(t, main, "worktree", "remove", "--force", active)
		if _, err := os.Stat(filepath.Join(store, "feat-b.md")); err != nil {
			t.Errorf("removed worktree lost handoff: %v", err)
		}
	})

	assertSharedStoreFallback := func(t *testing.T, prepare func(main, store string)) {
		t.Helper()
		main := project(t)
		handoffInitGit(t, main)
		handlers := syncHooks(t, main)
		active := filepath.Join(t.TempDir(), "private checkout")
		handoffGit(t, main, "worktree", "add", "-b", "feat/private", active)
		handoffWrite(t, filepath.Join(active, "agnostic-ai.yaml"), fixture)
		handoffWrite(t, filepath.Join(active, ".agnostic-ai", "local", "HANDOFF.md"), "LOCAL_FALLBACK\n")
		prepare(main, filepath.Join(main, ".agnostic-ai", "local", "handoffs"))
		for _, target := range handoffHookTargets {
			out := handoffExecute(t, target, handlers[target]["SessionStart"], active, "{}", "foreign", "")
			handoffAssertReply(t, target, out, "Handoff available: .agnostic-ai/local/HANDOFF.md. Ask to resume the handoff.")
		}
	}
	t.Run("task-handoff-external-shared-store-rejected", func(t *testing.T) {
		assertSharedStoreFallback(t, func(main, store string) {
			external := t.TempDir()
			handoffGit(t, external, "init")
			handoffWrite(t, filepath.Join(external, ".gitignore"), "*.md\n")
			handoffWrite(t, filepath.Join(external, "feat-private.md"), "# Handoff\ntool: codex | branch: feat/private | head: abc1234\n")
			if err := os.MkdirAll(filepath.Dir(store), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(external, store); err != nil {
				t.Skipf("directory symlinks unavailable: %v", err)
			}
		})
	})
	t.Run("task-handoff-shared-store-nested-git-preserves-tracked-guard", func(t *testing.T) {
		assertSharedStoreFallback(t, func(main, store string) {
			handoffWrite(t, filepath.Join(store, "feat-private.md"), "# Handoff\ntool: codex | branch: feat/private | head: abc1234\n")
			handoffGit(t, main, "add", "-f", "--", ".agnostic-ai/local/handoffs/feat-private.md")
			handoffGit(t, store, "init")
			handoffWrite(t, filepath.Join(store, ".gitignore"), "*.md\n")
		})
	})

	t.Run("task-handoff-unreadable-shared-store-falls-back", func(t *testing.T) {
		main := project(t)
		handoffInitGit(t, main)
		handlers := syncHooks(t, main)
		active := filepath.Join(t.TempDir(), "sandbox fallback checkout")
		handoffGit(t, main, "worktree", "add", "-b", "feat/private", active)
		handoffWrite(t, filepath.Join(active, "agnostic-ai.yaml"), fixture)
		store := filepath.Join(main, ".agnostic-ai", "local", "handoffs")
		handoffWrite(t, filepath.Join(store, "feat-private.md"), "# Handoff\ntool: codex | branch: feat/private | head: abc1234\n")
		handoffWrite(t, filepath.Join(active, ".agnostic-ai", "local", "handoffs", "feat-private.md"), "# Handoff\ntool: codex | branch: feat/private | head: abc1234\n")
		if err := os.Chmod(store, 0); err != nil {
			t.Skipf("directory permissions unsupported: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(store, 0o755) })
		if f, err := os.Open(store); err == nil {
			_ = f.Close()
			t.Skip("this platform or user can still read a mode-000 directory")
		} else if !os.IsPermission(err) {
			t.Fatalf("probe shared store permissions: %v", err)
		}
		for _, target := range handoffHookTargets {
			out := handoffExecute(t, target, handlers[target]["SessionStart"], active, "{}", "foreign", "")
			handoffAssertReply(t, target, out, "Task handoff available: .agnostic-ai/local/handoffs/feat-private.md (branch feat/private). Ask to resume the handoff.")
		}
	})

	t.Run("task-handoff-local-fallback", func(t *testing.T) {
		main := project(t)
		handoffInitGit(t, main)
		handlers := syncHooks(t, main)
		active := filepath.Join(t.TempDir(), "fallback checkout")
		handoffGit(t, main, "worktree", "add", "-b", "feat/local", active)
		handoffWrite(t, filepath.Join(active, "agnostic-ai.yaml"), fixture)
		// The shared path is not ignored, while the worktree still uses its committed ignore rule.
		handoffWrite(t, filepath.Join(main, ".gitignore"), ".env\n")
		handoffWrite(t, filepath.Join(main, ".agnostic-ai", "local", "handoffs", "feat-local.md"), "# Handoff\ntool: codex | branch: wrong | head: abc1234\n")
		handoffWrite(t, filepath.Join(active, ".agnostic-ai", "local", "handoffs", "feat-local.md"), "# Handoff\ntool: codex | branch: feat/local | head: abc1234\n")
		for _, target := range handoffHookTargets {
			out := handoffExecute(t, target, handlers[target]["SessionStart"], active, "{}", "foreign", "")
			handoffAssertReply(t, target, out, "Task handoff available: .agnostic-ai/local/handoffs/feat-local.md (branch feat/local). Ask to resume the handoff.")
		}
	})

	t.Run("task-handoff-slug-collision-and-bare-fallback", func(t *testing.T) {
		source := project(t)
		handlers := syncHooks(t, source)
		main := project(t)
		handoffInitGit(t, main)
		handoffGit(t, main, "branch", "-M", "collision-main")
		handoffWrite(t, filepath.Join(main, ".agnostic-ai", "local", "handoffs", "feat-a.md"), "# Handoff\ntool: codex | branch: feat-a | head: abc1234\n")
		active := filepath.Join(t.TempDir(), "collision checkout")
		handoffGit(t, main, "worktree", "add", "-b", "feat/a", active)
		handoffWrite(t, filepath.Join(active, "agnostic-ai.yaml"), fixture)
		handoffWrite(t, filepath.Join(active, ".agnostic-ai", "local", "HANDOFF.md"), "LEGACY_FALLBACK\n")
		for _, target := range handoffHookTargets {
			out := handoffExecute(t, target, handlers[target]["SessionStart"], active, "{}", "foreign", "")
			handoffAssertReply(t, target, out, "Handoff available: .agnostic-ai/local/HANDOFF.md. Ask to resume the handoff.")
		}
		// A bare main checkout has no shared project folder; the linked checkout owns the fallback.
		bare := t.TempDir()
		handoffGit(t, bare, "init", "--bare")
		linked := filepath.Join(t.TempDir(), "bare linked checkout")
		handoffGit(t, bare, "fetch", main, "HEAD")
		handoffGit(t, bare, "worktree", "add", "-b", "feat/bare", linked, "FETCH_HEAD")
		handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), fixture)
		handoffWrite(t, filepath.Join(linked, ".gitignore"), ".agnostic-ai/local/\n")
		handoffWrite(t, filepath.Join(linked, ".agnostic-ai", "local", "handoffs", "feat-bare.md"), "# Handoff\ntool: codex | branch: feat/bare | head: abc1234\n")
		for _, target := range handoffHookTargets {
			out := handoffExecute(t, target, handlers[target]["SessionStart"], linked, "{}", "foreign", "")
			handoffAssertReply(t, target, out, "Task handoff available: .agnostic-ai/local/handoffs/feat-bare.md (branch feat/bare). Ask to resume the handoff.")
		}
	})

	t.Run("claude-native-worktree-directory", func(t *testing.T) {
		for _, layout := range []string{"external-with-config", "nested-without-config"} {
			t.Run(layout, func(t *testing.T) {
				main := project(t)
				handoffInitGit(t, main)
				handoffGit(t, main, "branch", "-M", "audit-main")
				handlers := syncHooks(t, main)["claude"]
				if handlers["SessionStart"].Env["AGNOSTIC_AI_TARGET"] != "claude" {
					t.Fatal("emitted Claude handler environment is missing target identity")
				}
				active := filepath.Join(t.TempDir(), "active checkout ' 雪")
				if layout == "nested-without-config" {
					active = filepath.Join(main, ".claude", "worktrees", "active checkout ' 雪")
				}
				sibling := filepath.Join(t.TempDir(), "sibling checkout")
				handoffGit(t, main, "worktree", "add", "-b", "audit-worktree", active)
				handoffGit(t, main, "worktree", "add", "-b", "audit-sibling", sibling)
				if layout == "external-with-config" {
					handoffWrite(t, filepath.Join(active, "agnostic-ai.yaml"), fixture)
				}
				handoffWrite(t, filepath.Join(active, "tracked.txt"), "active committed change\n")
				handoffGit(t, active, "add", "tracked.txt")
				handoffGit(t, active, "commit", "-m", "active checkout commit")
				handoffWrite(t, filepath.Join(active, "tracked.txt"), "active dirty change\n")
				handoffWrite(t, filepath.Join(sibling, "tracked.txt"), "sibling committed change\n")
				handoffGit(t, sibling, "add", "tracked.txt")
				handoffGit(t, sibling, "commit", "-m", "sibling checkout commit")
				mainAuto, siblingManual, siblingAuto := "MAIN_AUTO_SENTINEL\n", "SIBLING_MANUAL_SENTINEL\n", "SIBLING_AUTO_SENTINEL\n"
				activeManual := "ACTIVE_MANUAL_SENTINEL\n"
				handoffWrite(t, filepath.Join(main, ".agnostic-ai/local/HANDOFF.auto.md"), mainAuto)
				handoffWrite(t, filepath.Join(sibling, ".agnostic-ai/local/HANDOFF.md"), siblingManual)
				handoffWrite(t, filepath.Join(sibling, ".agnostic-ai/local/HANDOFF.auto.md"), siblingAuto)
				handoffWrite(t, filepath.Join(active, ".agnostic-ai/local/HANDOFF.md"), activeManual)
				cwd := filepath.Join(active, "nested", "session directory")
				if err := os.MkdirAll(cwd, 0o755); err != nil {
					t.Fatal(err)
				}
				t.Setenv("CLAUDE_PROJECT_DIR", main)
				payload := func(event string) string {
					data, err := json.Marshal(map[string]string{"cwd": cwd, "hook_event_name": event, "session_id": "PAYLOAD_PRIVATE_SENTINEL"})
					if err != nil {
						t.Fatal(err)
					}
					return string(data)
				}
				out := handoffExecute(t, "claude", handlers["SessionStart"], main, payload("SessionStart"), "foreign", "")
				handoffAssertReply(t, "claude", out, "Handoff available: .agnostic-ai/local/HANDOFF.md. Ask to resume the handoff.")
				if got := handoffRead(t, filepath.Join(main, ".agnostic-ai/local/HANDOFF.auto.md")); got != mainAuto {
					t.Errorf("startup changed main snapshot: %q", got)
				}
				mainManual := "MAIN_MANUAL_SENTINEL\n"
				handoffWrite(t, filepath.Join(main, ".agnostic-ai/local/HANDOFF.md"), mainManual)
				for _, event := range []string{"PreCompact", "SessionEnd"} {
					t.Run(event, func(t *testing.T) {
						handoffWrite(t, filepath.Join(active, ".agnostic-ai/local/HANDOFF.auto.md"), "ACTIVE_OLD_AUTO_SENTINEL\n")
						out := handoffExecute(t, "claude", handlers[event], main, payload(event), "foreign", "")
						handoffAssertReply(t, "claude", out, "")
						snapshot := handoffRead(t, filepath.Join(active, ".agnostic-ai/local/HANDOFF.auto.md"))
						head := strings.TrimSpace(handoffGit(t, active, "rev-parse", "HEAD"))
						for _, want := range []string{"# Automatic handoff", "tool: claude", "branch: audit-worktree", "head: " + head, " M tracked.txt"} {
							if !strings.Contains(snapshot, want) {
								t.Errorf("active snapshot missing %q: %s", want, snapshot)
							}
						}
						for _, forbidden := range []string{"audit-main", "audit-sibling", "ACTIVE_OLD_AUTO_SENTINEL", "PAYLOAD_PRIVATE_SENTINEL", "ACTIVE_MANUAL_SENTINEL"} {
							if strings.Contains(snapshot, forbidden) {
								t.Errorf("active snapshot includes %q: %s", forbidden, snapshot)
							}
						}
						if got := handoffRead(t, filepath.Join(active, ".agnostic-ai/local/HANDOFF.md")); got != activeManual {
							t.Errorf("active manual changed: %q", got)
						}
						handoffAssertLocal(t, main, mainManual, mainAuto)
						handoffAssertLocal(t, sibling, siblingManual, siblingAuto)
					})
				}
			})
		}
	})

	t.Run("claude-native-input-validation", func(t *testing.T) {
		main := project(t)
		handoffInitGit(t, main)
		handlers := syncHooks(t, main)["claude"]
		name := "active 雪 😀 apostrophe '"
		if runtime.GOOS != "windows" {
			name += " quote \" backslash \\ literal $(touch SHOULD_NOT_EXIST) `touch SHOULD_NOT_EXIST`\n"
		}
		active := filepath.Join(t.TempDir(), name)
		handoffGit(t, main, "worktree", "add", "-b", "input-worktree", active)
		handoffWrite(t, filepath.Join(active, "tracked.txt"), "active dirty\n")
		mainManual, mainAuto := "MAIN_MANUAL\n", "MAIN_AUTO\n"
		activeManual, activeAuto := "ACTIVE_MANUAL\n", "ACTIVE_AUTO\n"
		quoted, err := json.Marshal(active)
		if err != nil {
			t.Fatal(err)
		}
		cwd := string(quoted)
		valid := "{\"cwd\":" + cwd + "}"
		invalid := []struct{ name, input string }{
			{"truncated", "{\"cwd\":" + cwd},
			{"trailing data", valid + " false"},
			{"duplicate", "{\"cwd\":" + cwd + ",\"c\\u0077d\":" + cwd + "}"},
			{"wrong type", "{\"cwd\":[]}"},
			{"null", "{\"cwd\":null}"},
			{"empty", "{\"cwd\":\"\"}"},
			{"relative", "{\"cwd\":\"nested\"}"},
			{"missing", "{\"cwd\":" + cwd[:len(cwd)-1] + "/missing\"}"},
			{"nul escape", "{\"cwd\":\"/\\u0000\"}"},
			{"raw nul", valid + "\x00"},
			{"raw soh", valid + "\x01"},
			{"high surrogate", "{\"cwd\":\"/\\ud800\"}"},
			{"low surrogate", "{\"cwd\":\"/\\udc00\"}"},
			{"bad escape", "{\"cwd\":\"/\\x20\"}"},
			{"non-object", "[]"},
			{"oversized", "{\"ignored\":\"" + strings.Repeat("x", 65536) + "\",\"cwd\":" + cwd + "}"},
			{"too deep", "{\"ignored\":" + strings.Repeat("[", 33) + "0" + strings.Repeat("]", 33) + ",\"cwd\":" + cwd + "}"},
		}
		reset := func() {
			handoffWrite(t, filepath.Join(main, ".agnostic-ai/local/HANDOFF.md"), mainManual)
			handoffWrite(t, filepath.Join(main, ".agnostic-ai/local/HANDOFF.auto.md"), mainAuto)
			handoffWrite(t, filepath.Join(active, ".agnostic-ai/local/HANDOFF.md"), activeManual)
			handoffWrite(t, filepath.Join(active, ".agnostic-ai/local/HANDOFF.auto.md"), activeAuto)
		}
		for _, tc := range invalid {
			t.Run(tc.name, func(t *testing.T) {
				reset()
				for _, event := range []string{"SessionStart", "PreCompact", "SessionEnd"} {
					out := handoffExecute(t, "claude", handlers[event], main, tc.input, "foreign", "")
					handoffAssertReply(t, "claude", out, "")
					handoffAssertLocal(t, main, mainManual, mainAuto)
					handoffAssertLocal(t, active, activeManual, activeAuto)
				}
			})
		}
		for _, tc := range []struct{ name, input, selected string }{
			{"literal path", valid, active},
			{"escaped Unicode", strings.ReplaceAll(strings.ReplaceAll(valid, "雪", "\\u96ea"), "😀", "\\ud83d\\ude00"), active},
			{"escaped key", strings.Replace(valid, "cwd", "c\\u0077d", 1), active},
			{"unknown values", "{\"unknown\":[null,true,false,-1.25e+2,{\"cwd\":\"wrong\"}],\"cwd\":" + cwd + ",\"text\":\"apparent cwd\"}", active},
			{"empty input", "", main},
			{"whitespace", " \r\n\t", main},
			{"absent cwd", "{\"nested\":{\"cwd\":\"wrong\"}}", main},
		} {
			t.Run(tc.name, func(t *testing.T) {
				reset()
				out := handoffExecute(t, "claude", handlers["PreCompact"], main, tc.input, "foreign", "")
				handoffAssertReply(t, "claude", out, "")
				snapshot := handoffRead(t, filepath.Join(tc.selected, ".agnostic-ai/local/HANDOFF.auto.md"))
				if !strings.Contains(snapshot, "# Automatic handoff") {
					t.Errorf("valid input did not save selected checkout: %q", snapshot)
				}
				if tc.selected == active {
					if !strings.Contains(snapshot, "branch: input-worktree") {
						t.Errorf("wrong checkout: %s", snapshot)
					}
					handoffAssertLocal(t, main, mainManual, mainAuto)
				} else {
					handoffAssertLocal(t, active, activeManual, activeAuto)
				}
			})
		}
		t.Run("stdin stays open", func(t *testing.T) {
			reset()
			read, write, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := read.Close(); err != nil {
					t.Errorf("close pipe reader: %v", err)
				}
				if err := write.Close(); err != nil {
					t.Errorf("close pipe writer: %v", err)
				}
			})
			if _, err := write.WriteString(valid); err != nil {
				t.Fatal(err)
			}
			started := time.Now()
			out := handoffExecuteInput(t, "claude", handlers["PreCompact"], main, read, "foreign", "")
			if elapsed := time.Since(started); elapsed > 8*time.Second {
				t.Errorf("stdin read was not bounded: %s", elapsed)
			}
			handoffAssertReply(t, "claude", out, "")
			handoffAssertLocal(t, main, mainManual, mainAuto)
			handoffAssertLocal(t, active, activeManual, activeAuto)
		})
	})

	t.Run("emission-and-golden", func(t *testing.T) {
		dir := project(t)
		handlers := syncHooks(t, dir)
		output := map[string]string{}
		for _, target := range handoffHookTargets {
			output[handoffHookPaths[target]] = handoffRead(t, filepath.Join(dir, handoffHookPaths[target]))
			events := []string{"PreCompact", "SessionEnd", "SessionStart"}
			if target == "gemini" {
				events[0] = "PreCompress"
			}
			if len(handlers[target]) != len(events) {
				t.Errorf("%s events = %v", target, handlers[target])
			}
			for _, event := range events {
				h, ok := handlers[target][event]
				if !ok {
					t.Errorf("%s missing %s", target, event)
					continue
				}
				if h.Type != "command" {
					t.Errorf("%s %s type = %s", target, event, h.Type)
				}
				if (target == "claude" || target == "qoder") && h.Shell != "bash" {
					t.Errorf("%s shell = %q", target, h.Shell)
				}
				body := h.Command
				if target == "claude" {
					handoffAssertClaudeInput(t, h.Command)
				} else {
					body = handoffAssertTransport(t, h.Command)
				}
				if event != "SessionStart" && (!strings.Contains(body, "cd \"$handoff_root\" || return 1")) {
					t.Errorf("%s %s does not run Git from the resolved root", target, event)
				}
				if target == "factory" && !strings.Contains(h.Command, "| sh -c \"read -r handoff_transport; exec sh -s factory\"; exit 0") {
					t.Error("Factory hook lacks fixed identity")
				}
				if target == "codex" {
					if body := handoffAssertTransport(t, h.CommandWindows); body != handoffAssertTransport(t, h.Command) {
						t.Errorf("Codex Windows %s body differs", event)
					}
					if !strings.Contains(h.CommandWindows, "| sh -c \"read -r handoff_transport; exec sh -s codex\"; exit 0") {
						t.Error("Windows snapshot lacks fixed Codex identity")
					}
				}
			}
			if handlers[target][events[0]].Command != handlers[target]["SessionEnd"].Command {
				t.Errorf("%s departure commands differ", target)
			}
		}
		expected := filepath.Join(packageDir, "fixtures", "golden", "builtin-handoff-hook")
		if os.Getenv("UPDATE_GOLDEN") == "1" {
			updateGolden(t, expected, output)
		} else {
			compareGolden(t, expected, output, "builtin-handoff-hook")
		}
		if _, err := os.Stat(filepath.Join(dir, ".factory", "settings.json")); !os.IsNotExist(err) {
			t.Errorf("built-in changes Factory display preference: %v", err)
		}
		run(t, dir, "sync", "--check", "--gitignore=off")
	})

	for _, target := range handoffHookTargets {
		name := "bom-leading-blank-line-" + target
		if target == "claude" {
			name = "direct-native-input-claude"
		}
		t.Run(name, func(t *testing.T) {
			transports := []string{"command"}
			if target == "codex" && runtime.GOOS != "windows" {
				transports = append(transports, "commandWindows")
			}
			for _, transport := range transports {
				t.Run(transport, func(t *testing.T) {
					dir := project(t)
					handlers := syncHooks(t, dir)[target]
					handoffInitGit(t, dir)
					cwd := filepath.Join(dir, "nested")
					if err := os.MkdirAll(cwd, 0o755); err != nil {
						t.Fatal(err)
					}
					withBOM := func(h handoffNativeHandler) handoffNativeHandler {
						inject := func(command string) string {
							if target == "claude" {
								handoffAssertClaudeInput(t, command)
								return command
							}
							if strings.Count(command, "echo '\n") != 1 {
								t.Fatalf("%s lacks one leading echo blank line: %q", target, command)
							}
							return strings.Replace(command, "echo '\n", "echo '\ufeff\n", 1)
						}
						h.Command = inject(h.Command)
						if h.CommandWindows != "" {
							h.CommandWindows = inject(h.CommandWindows)
						}
						if transport == "commandWindows" {
							h.Command = h.CommandWindows
						}
						return h
					}
					out := handoffExecute(t, target, withBOM(handlers["SessionStart"]), cwd, "{}", "foreign", "")
					handoffAssertReply(t, target, out, "")
					event := "PreCompact"
					if target == "gemini" {
						event = "PreCompress"
					}
					for _, event := range []string{event, "SessionEnd"} {
						t.Run(event, func(t *testing.T) {
							handoffWrite(t, filepath.Join(dir, "tracked.txt"), event+" dirty\n")
							out := handoffExecute(t, target, withBOM(handlers[event]), cwd, "{}", "foreign", "")
							handoffAssertReply(t, target, out, "")
							snapshot := handoffRead(t, filepath.Join(dir, ".agnostic-ai", "local", "HANDOFF.auto.md"))
							branch := strings.TrimSpace(handoffGit(t, dir, "symbolic-ref", "--short", "HEAD"))
							head := strings.TrimSpace(handoffGit(t, dir, "rev-parse", "HEAD"))
							for _, want := range []string{"# Automatic handoff", "tool: " + target, "branch: " + branch, "head: " + head, "## Last five commits", strings.TrimSpace(handoffGit(t, dir, "status", "--short"))} {
								if !strings.Contains(snapshot, want) {
									t.Errorf("snapshot missing %q:\n%s", want, snapshot)
								}
							}
							lines := strings.Split(snapshot, "\n")
							if len(lines) < 2 {
								t.Fatalf("snapshot lacks metadata: %q", snapshot)
							}
							metadata := strings.Split(lines[1], " | ")
							if len(metadata) != 4 {
								t.Fatalf("metadata = %v", metadata)
							}
							if _, err := time.Parse(time.RFC3339, strings.TrimPrefix(metadata[1], "date: ")); err != nil {
								t.Errorf("UTC metadata: %v", err)
							}
						})
					}
				})
			}
		})

		t.Run("snapshot-and-notice-"+target, func(t *testing.T) {
			dir := project(t)
			handlers := syncHooks(t, dir)[target]
			subjects := handoffInitGit(t, dir)
			manual := "# Handoff\nMANUAL_GOAL_SENTINEL\n## Next steps\nKEEP_MANUAL_NEXT_STEPS\n"
			handoffWrite(t, filepath.Join(dir, ".agnostic-ai", "local", "HANDOFF.md"), manual)
			handoffWrite(t, filepath.Join(dir, ".agnostic-ai", "local", "HANDOFF.auto.md"), "OLD_AUTO_SENTINEL\n")
			handoffWrite(t, filepath.Join(dir, "tracked.txt"), "dirty\n")
			handoffWrite(t, filepath.Join(dir, "untracked 雪 ' file.txt"), "ordinary data\n")
			handoffWrite(t, filepath.Join(dir, ".env"), "IGNORED_FAKE_SECRET_SENTINEL\n")
			transcript := filepath.Join(dir, ".agnostic-ai", "local", "transcript.jsonl")
			handoffWrite(t, transcript, "TRANSCRIPT_CONTENT_SENTINEL\n")
			cwd := filepath.Join(dir, "nested", "below")
			if err := os.MkdirAll(cwd, 0o755); err != nil {
				t.Fatal(err)
			}
			event := "PreCompact"
			if target == "gemini" {
				event = "PreCompress"
			}
			stdin, err := json.Marshal(map[string]string{"cwd": cwd, "hook_event_name": event, "transcript_path": transcript, "session_id": "STDIN_SENTINEL"})
			if err != nil {
				t.Fatal(err)
			}
			out := handoffExecute(t, target, handlers[event], cwd, string(stdin), "foreign", "")
			handoffAssertReply(t, target, out, "")
			snapshot := handoffRead(t, filepath.Join(dir, ".agnostic-ai", "local", "HANDOFF.auto.md"))
			branch := strings.TrimSpace(handoffGit(t, dir, "symbolic-ref", "--short", "HEAD"))
			head := strings.TrimSpace(handoffGit(t, dir, "rev-parse", "HEAD"))
			for _, text := range []string{"# Automatic handoff", "tool: " + target, "branch: " + branch, "head: " + head, handoffGit(t, dir, "status", "--short")} {
				if !strings.Contains(snapshot, strings.TrimSpace(text)) {
					t.Errorf("snapshot missing %q:\n%s", text, snapshot)
				}
			}
			metadata := strings.Split(strings.Split(snapshot, "\n")[1], " | ")
			if len(metadata) != 4 {
				t.Fatalf("metadata = %v", metadata)
			}
			if _, err := time.Parse(time.RFC3339, strings.TrimPrefix(metadata[1], "date: ")); err != nil {
				t.Errorf("UTC metadata: %v", err)
			}
			_, commitSection, found := strings.Cut(snapshot, "## Last five commits\n\n")
			newest := slices.Clone(subjects[2:])
			slices.Reverse(newest)
			if !found || strings.TrimSpace(commitSection) != strings.Join(newest, "\n") {
				t.Errorf("last five subjects = %q", commitSection)
			}
			for i, subject := range subjects {
				if strings.Contains(snapshot, subject) != (i >= 2) {
					t.Errorf("subject %d inclusion: %q", i, subject)
				}
			}
			for _, forbidden := range []string{"OLD_AUTO_SENTINEL", "MANUAL_GOAL_SENTINEL", "KEEP_MANUAL_NEXT_STEPS", "IGNORED_FAKE_SECRET_SENTINEL", "TRANSCRIPT_CONTENT_SENTINEL", "STDIN_SENTINEL"} {
				if strings.Contains(snapshot+out, forbidden) {
					t.Errorf("leaked %s", forbidden)
				}
			}
			handoffAssertLocal(t, dir, manual, snapshot)
			handoffWrite(t, filepath.Join(dir, "second.txt"), "second event\n")
			out = handoffExecute(t, target, handlers["SessionEnd"], cwd, string(stdin), "claude", "")
			handoffAssertReply(t, target, out, "")
			if refreshed := handoffRead(t, filepath.Join(dir, ".agnostic-ai", "local", "HANDOFF.auto.md")); !strings.Contains(refreshed, "second.txt") {
				t.Errorf("SessionEnd did not refresh snapshot:\n%s", refreshed)
			}
			handoffRemoveTaskSnapshot(t, dir)
			for _, files := range []struct {
				name         string
				manual, auto bool
				message      string
			}{
				{"both", true, true, "Handoff available: .agnostic-ai/local/HANDOFF.md and HANDOFF.auto.md. Ask to resume the handoff."},
				{"manual", true, false, "Handoff available: .agnostic-ai/local/HANDOFF.md. Ask to resume the handoff."},
				{"auto", false, true, "Git snapshot available: .agnostic-ai/local/HANDOFF.auto.md. Ask to resume the handoff."},
				{"neither", false, false, ""},
			} {
				t.Run(files.name, func(t *testing.T) {
					for name, exists := range map[string]bool{"HANDOFF.md": files.manual, "HANDOFF.auto.md": files.auto} {
						path := filepath.Join(dir, ".agnostic-ai", "local", name)
						if exists {
							handoffWrite(t, path, "CONTENTS_MUST_NOT_BE_READ\n")
						} else if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
							t.Fatal(err)
						}
					}
					out := handoffExecute(t, target, handlers["SessionStart"], cwd, string(stdin), "foreign", "")
					handoffAssertReply(t, target, out, files.message)
				})
			}
			if target == "codex" {
				for _, inherited := range []string{"", "claude", "gemini", "factory", "unknown"} {
					h := handlers[event]
					h.Command = h.CommandWindows
					out := handoffExecute(t, target, h, cwd, string(stdin), inherited, "")
					handoffAssertReply(t, target, out, "")
					handoffRemoveTaskSnapshot(t, dir)
					start := handlers["SessionStart"]
					start.Command = start.CommandWindows
					message := "Git snapshot available: .agnostic-ai/local/HANDOFF.auto.md. Ask to resume the handoff."
					handoffAssertReply(t, target, handoffExecute(t, target, start, cwd, "{}", inherited, ""), message)
					if snapshot := handoffRead(t, filepath.Join(dir, ".agnostic-ai", "local", "HANDOFF.auto.md")); !strings.Contains(snapshot, "tool: codex") {
						t.Errorf("inherited %q changed Windows identity:\n%s", inherited, snapshot)
					}
				}
			}
		})
	}

	t.Run("root-choice-and-detached", func(t *testing.T) {
		dir := project(t)
		handlers := syncHooks(t, dir)["claude"]
		handoffInitGit(t, dir)
		handoffWrite(t, filepath.Join(dir, ".agnostic-ai", "local", "HANDOFF.auto.md"), "PARENT_SENTINEL\n")
		for _, configName := range []string{"agnostic-ai.yaml", "agnostic.config.yaml"} {
			child := filepath.Join(dir, configName+" child")
			handoffWrite(t, filepath.Join(child, configName), "version: 1\n")
			handoffWrite(t, filepath.Join(child, ".gitignore"), ".agnostic-ai/local/\n")
			cwd := filepath.Join(child, "nested")
			if err := os.MkdirAll(cwd, 0o755); err != nil {
				t.Fatal(err)
			}
			handoffAssertReply(t, "claude", handoffExecute(t, "claude", handlers["PreCompact"], cwd, "{}", "", ""), "")
			if snapshot := handoffRead(t, filepath.Join(child, ".agnostic-ai", "local", "HANDOFF.auto.md")); !strings.Contains(snapshot, "head: ") {
				t.Errorf("child snapshot = %s", snapshot)
			}
			handoffRemoveTaskSnapshot(t, child)
			handoffAssertReply(t, "claude", handoffExecute(t, "claude", handlers["SessionStart"], cwd, "{}", "", ""), "Git snapshot available: .agnostic-ai/local/HANDOFF.auto.md. Ask to resume the handoff.")
		}
		if got := handoffRead(t, filepath.Join(dir, ".agnostic-ai", "local", "HANDOFF.auto.md")); got != "PARENT_SENTINEL\n" {
			t.Errorf("child replaced parent: %s", got)
		}
		handoffGit(t, dir, "checkout", "--detach", "HEAD")
		handoffAssertReply(t, "claude", handoffExecute(t, "claude", handlers["PreCompact"], dir, "{}", "", ""), "")
		if snapshot := handoffRead(t, filepath.Join(dir, ".agnostic-ai", "local", "HANDOFF.auto.md")); !strings.Contains(snapshot, "branch: detached") {
			t.Errorf("detached snapshot = %s", snapshot)
		}
	})

	t.Run("unavailable-git-preserves-snapshots", func(t *testing.T) {
		source := project(t)
		h := syncHooks(t, source)["claude"]["PreCompact"]
		for _, mode := range []string{"non-git", "unborn", "bare"} {
			t.Run(mode, func(t *testing.T) {
				dir := t.TempDir()
				handoffWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\n")
				if mode == "unborn" {
					handoffGit(t, dir, "init")
				}
				if mode == "bare" {
					handoffGit(t, dir, "init", "--bare")
				}
				handoffWrite(t, filepath.Join(dir, ".agnostic-ai", "local", "HANDOFF.md"), "MANUAL\n")
				handoffWrite(t, filepath.Join(dir, ".agnostic-ai", "local", "HANDOFF.auto.md"), "AUTO\n")
				handoffAssertReply(t, "claude", handoffExecute(t, "claude", h, dir, "{}", "", ""), "")
				handoffAssertLocal(t, dir, "MANUAL\n", "AUTO\n")
			})
		}
	})

	t.Run("startup-outside-config-and-git-skips-notice", func(t *testing.T) {
		source := project(t)
		handlers := syncHooks(t, source)
		dir := t.TempDir()
		handoffWrite(t, filepath.Join(dir, ".agnostic-ai", "local", "HANDOFF.md"), "UNRELATED_MANUAL\n")
		for _, target := range handoffHookTargets {
			out := handoffExecute(t, target, handlers[target]["SessionStart"], dir, "{}", "foreign", "")
			handoffAssertReply(t, target, out, "")
		}
	})

	t.Run("metadata-and-replacement-failures-preserve-files", func(t *testing.T) {
		dir := project(t)
		handlers := syncHooks(t, dir)
		handoffInitGit(t, dir)
		realGit, err := exec.LookPath("git")
		if err != nil {
			t.Fatal(err)
		}
		for _, mode := range []string{"status", "symbolic-ref", "rename", "signal"} {
			t.Run(mode, func(t *testing.T) {
				shim := t.TempDir()
				marker := filepath.Join(shim, "failure-hit")
				mark := "printf '%s\\n' " + handoffShellQuote(mode) + " > " + handoffShellQuote(filepath.ToSlash(marker)) + "\n"
				if mode == "status" || mode == "symbolic-ref" {
					script := "#!/bin/sh\nif [ \"$1\" = " + handoffShellQuote(mode) + " ]; then\n" + mark + "exit 128\nfi\nexec " + handoffShellQuote(filepath.ToSlash(realGit)) + " \"$@\"\n"
					handoffWrite(t, filepath.Join(shim, "git"), script)
					if err := os.Chmod(filepath.Join(shim, "git"), 0o755); err != nil {
						t.Fatal(err)
					}
				} else {
					script := "#!/bin/sh\n" + mark + "exit 1\n"
					if mode == "signal" {
						script = "#!/bin/sh\n" + mark + "kill -TERM \"$PPID\"\nexit 1\n"
					}
					handoffWrite(t, filepath.Join(shim, "mv"), script)
					if err := os.Chmod(filepath.Join(shim, "mv"), 0o755); err != nil {
						t.Fatal(err)
					}
				}
				for _, target := range handoffHookTargets {
					if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
						t.Fatal(err)
					}
					event := "PreCompact"
					if target == "gemini" {
						event = "PreCompress"
					}
					handoffWrite(t, filepath.Join(dir, ".agnostic-ai", "local", "HANDOFF.md"), "MANUAL\n")
					handoffWrite(t, filepath.Join(dir, ".agnostic-ai", "local", "HANDOFF.auto.md"), "PREVIOUS_COMPLETE_AUTO\n")
					out := handoffExecute(t, target, handlers[target][event], dir, "{}", "foreign", shim)
					handoffAssertReply(t, target, out, "")
					if hit, err := os.ReadFile(marker); err != nil || string(hit) != mode+"\n" {
						t.Errorf("%s did not reach %s failure shim: marker = %q, err = %v", target, mode, hit, err)
					}
					handoffAssertLocal(t, dir, "MANUAL\n", "PREVIOUS_COMPLETE_AUTO\n")
				}
			})
		}
	})

	t.Run("global-fallback-keeps-ignore-files", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		t.Setenv("AGNOSTIC_AI_HOME", filepath.Join(home, ".agnostic-ai"))
		handoffWrite(t, filepath.Join(home, ".agnostic-ai", "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\nbuiltins: [handoff-hook]\n")
		dir := t.TempDir()
		handoffInitGit(t, dir)
		ignore := handoffRead(t, filepath.Join(dir, ".gitignore"))
		exclude := handoffRead(t, filepath.Join(dir, ".git", "info", "exclude"))
		run(t, dir, "sync", "--global")
		h := handoffReadHandlersFor(t, home, "claude")["PreCompact"]
		cwd := filepath.Join(dir, "nested")
		if err := os.MkdirAll(cwd, 0o755); err != nil {
			t.Fatal(err)
		}
		handoffAssertReply(t, "claude", handoffExecute(t, "claude", h, cwd, "{}", "foreign", ""), "")
		if got := handoffRead(t, filepath.Join(dir, ".gitignore")); got != ignore {
			t.Errorf("global hook changed .gitignore: %s", got)
		}
		if got := handoffRead(t, filepath.Join(dir, ".git", "info", "exclude")); got != exclude {
			t.Errorf("global hook changed exclude: %s", got)
		}
		if got := handoffRead(t, filepath.Join(dir, ".agnostic-ai", "local", "HANDOFF.auto.md")); !strings.Contains(got, "tool: claude") {
			t.Errorf("global snapshot = %s", got)
		}
		if _, err := os.Stat(filepath.Join(home, ".agnostic-ai", "local")); !os.IsNotExist(err) {
			t.Errorf("global hook wrote home handoff: %v", err)
		}
	})

	t.Run("global-source-at-home-keeps-checkout-root", func(t *testing.T) {
		source := project(t)
		handlers := syncHooks(t, source)
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		t.Setenv("AGNOSTIC_AI_HOME", home)
		handoffWrite(t, filepath.Join(home, "agnostic-ai.yaml"), fixture)
		handoffWrite(t, filepath.Join(home, ".gitignore"), "HOME_IGNORE_SENTINEL\n")
		dir := filepath.Join(home, "repos", "example checkout 雪 ' apostrophe")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		handoffInitGit(t, dir)
		ignore := handoffRead(t, filepath.Join(dir, ".gitignore"))
		exclude := handoffRead(t, filepath.Join(dir, ".git", "info", "exclude"))
		manual := "CHECKOUT_MANUAL_SENTINEL\n"
		handoffWrite(t, filepath.Join(dir, ".agnostic-ai", "local", "HANDOFF.md"), manual)
		cwd := filepath.Join(dir, "nested")
		if err := os.MkdirAll(cwd, 0o755); err != nil {
			t.Fatal(err)
		}
		globalRoots := []struct{ name, path string }{{"home", home}}
		if runtime.GOOS != "windows" {
			alias := filepath.Join(t.TempDir(), "global home alias")
			if err := os.Symlink(home, alias); err != nil {
				t.Fatal(err)
			}
			globalRoots = append(globalRoots, struct{ name, path string }{"symlink", alias})
		}
		for _, globalRoot := range globalRoots {
			t.Run(globalRoot.name, func(t *testing.T) {
				t.Setenv("AGNOSTIC_AI_HOME", globalRoot.path)
				for _, target := range handoffHookTargets {
					t.Run(target, func(t *testing.T) {
						event := "PreCompact"
						if target == "gemini" {
							event = "PreCompress"
						}
						out := handoffExecute(t, target, handlers[target][event], cwd, "{}", "foreign", "")
						handoffAssertReply(t, target, out, "")
						snapshot := handoffRead(t, filepath.Join(dir, ".agnostic-ai", "local", "HANDOFF.auto.md"))
						if !strings.Contains(snapshot, "tool: "+target) {
							t.Errorf("checkout snapshot = %s", snapshot)
						}
						handoffAssertLocal(t, dir, manual, snapshot)
						handoffRemoveTaskSnapshot(t, dir)
						message := "Handoff available: .agnostic-ai/local/HANDOFF.md and HANDOFF.auto.md. Ask to resume the handoff."
						handoffAssertReply(t, target, handoffExecute(t, target, handlers[target]["SessionStart"], cwd, "{}", "foreign", ""), message)
						for _, path := range []string{filepath.Join(home, ".agnostic-ai", "local"), filepath.Join(home, "local")} {
							if _, err := os.Stat(path); !os.IsNotExist(err) {
								t.Errorf("hook wrote global-home artifacts at %s: %v", path, err)
							}
						}
						if got := handoffRead(t, filepath.Join(home, ".gitignore")); got != "HOME_IGNORE_SENTINEL\n" {
							t.Errorf("home ignore changed: %s", got)
						}
						if got := handoffRead(t, filepath.Join(dir, ".gitignore")); got != ignore {
							t.Errorf("checkout ignore changed: %s", got)
						}
						if got := handoffRead(t, filepath.Join(dir, ".git", "info", "exclude")); got != exclude {
							t.Errorf("checkout exclude changed: %s", got)
						}
					})
				}
			})
		}
	})

	t.Run("factory-display-setting-is-user-owned", func(t *testing.T) {
		dir := project(t)
		handoffWrite(t, filepath.Join(dir, ".agnostic-ai", "settings", "handoff-notices.yaml"), "name: handoff-notices\ntargets: [factory]\nx-factory:\n  showHookOutput: true\n")
		syncHooks(t, dir)
		var settings map[string]any
		if err := json.Unmarshal([]byte(handoffRead(t, filepath.Join(dir, ".factory", "settings.json"))), &settings); err != nil {
			t.Fatal(err)
		}
		if settings["showHookOutput"] != true {
			t.Errorf("settings = %v", settings)
		}
	})

	for _, target := range handoffHookTargets {
		t.Run("import-opt-out-and-override-"+target, func(t *testing.T) {
			dir := project(t)
			handoffWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+target+"]\nbuiltins: [handoff-hook]\n")
			run(t, dir, "sync", "--gitignore=off")
			run(t, dir, "import", target)
			out := run(t, dir, "list")
			builtinCount := 0
			for _, line := range strings.Split(out, "\n") {
				fields := strings.Split(line, "\t")
				if len(fields) >= 3 && fields[2] == "builtin" {
					builtinCount++
				}
			}
			if builtinCount != 3 {
				t.Errorf("import lost builtin provenance: %s", out)
			}
			for _, name := range []string{"handoff-pre-compact", "handoff-session-end", "handoff-session-start"} {
				if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "hooks", name+".yaml")); !os.IsNotExist(err) {
					t.Errorf("import copied %s: %v", name, err)
				}
			}
			run(t, dir, "sync", "--check", "--gitignore=off")
			userHook := filepath.Join(dir, ".agnostic-ai", "hooks", "user-hook.yaml")
			handoffWrite(t, userHook, "name: user-hook\nevent: UserPromptSubmit\ncommand: echo user-hook\n")
			override := filepath.Join(dir, ".agnostic-ai", "hooks", "handoff-session-start.yaml")
			handoffWrite(t, override, "name: handoff-session-start\nevent: SessionStart\ncommand: echo MY_PROJECT_OVERRIDE\n")
			run(t, dir, "sync", "--gitignore=off")
			run(t, dir, "import", target)
			if body := handoffRead(t, override); !strings.Contains(body, "MY_PROJECT_OVERRIDE") {
				t.Errorf("import lost override: %s", body)
			}
			handoffWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+target+"]\nbuiltins: []\n")
			run(t, dir, "sync", "--gitignore=off")
			handlers := handoffReadHandlersFor(t, dir, target)
			if len(handlers) != 2 || handlers["SessionStart"].Command == "" || handlers["UserPromptSubmit"].Command == "" {
				t.Errorf("opt-out handlers = %v", handlers)
			}
			run(t, dir, "sync", "--check", "--gitignore=off")
		})
	}
}

func handoffReadHandlers(t *testing.T, dir string) map[string]map[string]handoffNativeHandler {
	t.Helper()
	out := map[string]map[string]handoffNativeHandler{}
	for _, target := range handoffHookTargets {
		out[target] = handoffReadHandlersFor(t, dir, target)
	}
	return out
}

func handoffReadHandlersFor(t *testing.T, dir, target string) map[string]handoffNativeHandler {
	t.Helper()
	var doc struct {
		Hooks map[string][]handoffNativeGroup `json:"hooks"`
		Env   map[string]string               `json:"env"`
	}
	data := []byte(handoffRead(t, filepath.Join(dir, handoffHookPaths[target])))
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if target == "factory" {
		if err := json.Unmarshal(data, &doc.Hooks); err != nil {
			t.Fatal(err)
		}
	}
	out := map[string]handoffNativeHandler{}
	for event, groups := range doc.Hooks {
		if len(groups) != 1 || len(groups[0].Hooks) != 1 {
			t.Fatalf("%s %s groups = %v, want one handler", target, event, groups)
		}
		if groups[0].Matcher != "" {
			t.Errorf("%s %s matcher = %s", target, event, groups[0].Matcher)
		}
		h := groups[0].Hooks[0]
		if h.Env == nil {
			h.Env = map[string]string{}
		}
		for k, v := range doc.Env {
			h.Env[k] = v
		}
		out[event] = h
	}
	return out
}

func handoffAssertClaudeInput(t *testing.T, command string) {
	t.Helper()
	if !strings.HasPrefix(command, "export LC_ALL=C\n") || !strings.Contains(command, "read -r -d '' -n 65537 -t 2 handoff_payload") {
		t.Errorf("Claude command does not retain bounded native stdin: %q", command)
	}
	if strings.Contains(command, "echo '\n") || strings.Contains(command, "</dev/null") || strings.Contains(command, "exec sh -s") {
		t.Errorf("Claude command replaces native input with program source: %q", command)
	}
}

func handoffAssertTransport(t *testing.T, command string) string {
	t.Helper()
	_, after, ok := strings.Cut(command, "echo '\n")
	if !ok {
		t.Fatalf("command does not transport readable stdin: %q", command)
	}
	body, suffix, ok := strings.Cut(after, "' | sh -c ")
	if !ok || !strings.HasSuffix(body, "\n#") || strings.ContainsAny(body, "'\\\r") {
		t.Errorf("unsafe stdin body: %q", command)
	}
	for _, c := range body {
		if c > 127 {
			t.Errorf("non-ASCII source: %q", c)
			break
		}
	}
	tails := []string{
		"\"read -r handoff_transport; exec sh -s\"; exit 0",
		"\"read -r handoff_transport; exec sh -s codex\"; exit 0",
		"\"read -r handoff_transport; exec sh -s factory\"; exit 0",
	}
	if strings.Contains(command, "agnostic-ai-builtin-") || strings.Contains(command, "$(go ") || !slices.Contains(tails, strings.TrimSpace(suffix)) {
		t.Errorf("unexpected hook transport: %q", command)
	}
	return body
}

func handoffExecute(t *testing.T, target string, h handoffNativeHandler, dir, stdin, inherited, shimDir string) string {
	t.Helper()
	return handoffExecuteInput(t, target, h, dir, strings.NewReader(stdin), inherited, shimDir)
}

func handoffExecuteInput(t *testing.T, target string, h handoffNativeHandler, dir string, stdin io.Reader, inherited, shimDir string) string {
	t.Helper()
	stdout, stderr := handoffRunInput(t, target, h, dir, stdin, inherited, shimDir)
	if stderr != "" {
		t.Errorf("%s stderr: %s", target, stderr)
	}
	return stdout
}

func handoffTraceStartup(t *testing.T, target string, h handoffNativeHandler, dir string) {
	t.Helper()
	trace := func(command string) string {
		command = strings.ReplaceAll(command, "handoff_notice() {\n", "handoff_notice() {\n  set -x\n")
		command = strings.ReplaceAll(command, "handoff_notice </dev/null 2>/dev/null", "handoff_notice </dev/null")
		return strings.ReplaceAll(command, "handoff_notice 2>/dev/null", "handoff_notice")
	}
	h.Command = trace(h.Command)
	h.CommandWindows = trace(h.CommandWindows)
	stdout, stderr := handoffRunInput(t, target, h, dir, strings.NewReader("{}"), "foreign", "")
	t.Logf("%s startup trace in %s:\nstdout: %s\nstderr: %s", target, dir, stdout, stderr)
}

func handoffRunInput(t *testing.T, target string, h handoffNativeHandler, dir string, stdin io.Reader, inherited, shimDir string) (string, string) {
	t.Helper()
	shell, args := "bash", []string{"-c", h.Command}
	if target == "codex" {
		shell = "sh"
	}
	var pathParts []string
	if shimDir != "" {
		pathParts = append(pathParts, shimDir)
	}
	if runtime.GOOS == "windows" {
		git, err := exec.LookPath("git")
		if err != nil {
			t.Fatal(err)
		}
		root := filepath.Dir(filepath.Dir(git))
		if slices.Contains([]string{"mingw64", "mingw32", "ucrt64", "clang64", "clang32", "clangarm64"}, strings.ToLower(filepath.Base(filepath.Dir(filepath.Dir(git))))) {
			root = filepath.Dir(root)
		}
		bash := filepath.Join(root, "usr", "bin", "bash.exe")
		if _, err := os.Stat(bash); err != nil {
			t.Fatalf("Git Bash required for native boundary test: %v", err)
		}
		pathParts = append(pathParts, filepath.Dir(bash), filepath.Join(root, "bin"))
		if target == "codex" || target == "gemini" {
			command := h.Command
			if target == "codex" {
				command = h.CommandWindows
			}
			shell, args = "powershell.exe", []string{"-NoProfile", "-NonInteractive", "-Command", command}
		} else {
			shell = bash
			if target == "claude" {
				// Native Go/MSYS argument quoting drops backslashes in the emitted command.
				script := filepath.Join(t.TempDir(), "claude-handoff.sh")
				handoffWrite(t, script, h.Command)
				args = []string{filepath.ToSlash(script)}
			}
		}
	}
	pathParts = append(pathParts, os.Getenv("PATH"))
	timeout := 15 * time.Second
	if runtime.GOOS == "windows" {
		timeout = 45 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, args...)
	cmd.Dir = dir
	env := os.Environ()
	env = slices.DeleteFunc(env, func(value string) bool {
		key, _, _ := strings.Cut(value, "=")
		if strings.EqualFold(key, "AGNOSTIC_AI_TARGET") || strings.EqualFold(key, "PATH") {
			return true
		}
		for name := range h.Env {
			if strings.EqualFold(key, name) {
				return true
			}
		}
		return false
	})
	if inherited != "" {
		env = append(env, "AGNOSTIC_AI_TARGET="+inherited)
	}
	for k, v := range h.Env {
		env = append(env, k+"="+v)
	}
	env = append(env, "PATH="+strings.Join(pathParts, string(os.PathListSeparator)))
	cmd.Env = env
	cmd.Stdin = stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("direct %s command failed: %v (context: %v)\nstdout: %s\nstderr: %s", target, err, ctx.Err(), &stdout, &stderr)
	}
	return stdout.String(), stderr.String()
}

func handoffAssertReply(t *testing.T, target, out, message string) {
	t.Helper()
	if target == "factory" {
		expected := ""
		if message != "" {
			expected = message + "\n"
		}
		if strings.ReplaceAll(out, "\r\n", "\n") != expected {
			t.Errorf("Factory stdout = %q, want %q", out, expected)
		}
		return
	}
	var reply map[string]any
	if err := json.Unmarshal([]byte(out), &reply); err != nil {
		t.Fatalf("%s reply = %q: %v", target, out, err)
	}
	if strings.Count(strings.TrimSpace(out), "\n") != 0 {
		t.Errorf("%s reply has multiple lines: %q", target, out)
	}
	if message == "" {
		if len(reply) != 0 {
			t.Errorf("%s neutral reply = %v", target, reply)
		}
	} else if len(reply) != 1 || reply["systemMessage"] != message {
		t.Errorf("%s reply = %v, want %q", target, reply, message)
	}
}

func handoffInitGit(t *testing.T, dir string) []string {
	t.Helper()
	handoffGit(t, dir, "init")
	handoffGit(t, dir, "config", "user.name", "Handoff Test")
	handoffGit(t, dir, "config", "user.email", "handoff@example.invalid")
	handoffGit(t, dir, "config", "core.autocrlf", "false")
	handoffGit(t, dir, "config", "commit.gpgsign", "false")
	handoffWrite(t, filepath.Join(dir, ".gitignore"), ".agnostic-ai/local/\n.env\n.claude/\n.codex/\n.gemini/\n.qoder/\n.factory/\n.agnostic-ai/.sync-state.json\n")
	subjects := make([]string, 7)
	for i := range subjects {
		subjects[i] = fmt.Sprintf("subject-%d 雪 apostrophe ' literal $(touch SHOULD_NOT_EXIST) backslash \\", i)
		handoffWrite(t, filepath.Join(dir, "tracked.txt"), fmt.Sprintf("commit %d\n", i))
		handoffGit(t, dir, "add", ".gitignore", "tracked.txt")
		handoffGit(t, dir, "commit", "-m", subjects[i])
	}
	return subjects
}
func handoffGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}
func handoffGitPathShim(t *testing.T) (string, string) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	shim := t.TempDir()
	marker := filepath.Join(shim, "git-used")
	script := "#!/bin/sh\nfor arg do\ncase \"$arg\" in /*) exit 128 ;; esac\ndone\nprintf used > " + handoffShellQuote(filepath.ToSlash(marker)) + "\nexec " + handoffShellQuote(filepath.ToSlash(realGit)) + " \"$@\"\n"
	handoffWrite(t, filepath.Join(shim, "git"), script)
	if err := os.Chmod(filepath.Join(shim, "git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return shim, marker
}

func handoffWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
func handoffRead(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
func handoffAssertLocal(t *testing.T, dir, manual, auto string) {
	t.Helper()
	local := filepath.Join(dir, ".agnostic-ai", "local")
	if got := handoffRead(t, filepath.Join(local, "HANDOFF.md")); got != manual {
		t.Errorf("manual changed: %q", got)
	}
	if got := handoffRead(t, filepath.Join(local, "HANDOFF.auto.md")); got != auto {
		t.Errorf("auto changed: %q", got)
	}
	files, err := os.ReadDir(local)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "HANDOFF.auto.md.") {
			t.Errorf("left temporary snapshot %s", file.Name())
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "SHOULD_NOT_EXIST")); !os.IsNotExist(err) {
		t.Errorf("runtime data evaluated: %v", err)
	}
}

func handoffShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func handoffRemoveTaskSnapshot(t *testing.T, dir string) {
	t.Helper()
	branch := strings.TrimSpace(handoffGit(t, dir, "branch", "--show-current"))
	if branch == "" {
		branch = "detached-" + strings.TrimSpace(handoffGit(t, dir, "rev-parse", "HEAD"))
	}
	slug := strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || strings.ContainsRune("._-", r) {
			return r
		}
		return '-'
	}, branch)
	path := filepath.Join(dir, ".agnostic-ai", "local", "handoffs", slug+".auto.md")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove task snapshot for legacy notice case: %v", err)
	}
}
