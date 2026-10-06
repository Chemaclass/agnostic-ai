package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
				body := handoffAssertTransport(t, h.Command)
				if event != "SessionStart" && (!strings.Contains(body, "cd \"$handoff_root\" || return 1") || strings.Contains(body, "git -C")) {
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
		t.Run("bom-leading-blank-line-"+target, func(t *testing.T) {
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
			cwd := filepath.Join(child, "nested")
			if err := os.MkdirAll(cwd, 0o755); err != nil {
				t.Fatal(err)
			}
			handoffAssertReply(t, "claude", handoffExecute(t, "claude", handlers["PreCompact"], cwd, "{}", "", ""), "")
			if snapshot := handoffRead(t, filepath.Join(child, ".agnostic-ai", "local", "HANDOFF.auto.md")); !strings.Contains(snapshot, "head: ") {
				t.Errorf("child snapshot = %s", snapshot)
			}
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
		if strings.HasPrefix(strings.ToLower(filepath.Base(filepath.Dir(filepath.Dir(git)))), "mingw") {
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
		}
	}
	pathParts = append(pathParts, os.Getenv("PATH"))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("direct %s command failed: %v\nstdout: %s\nstderr: %s", target, err, &stdout, &stderr)
	}
	if stderr.Len() != 0 {
		t.Errorf("%s stderr: %s", target, &stderr)
	}
	return stdout.String()
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
