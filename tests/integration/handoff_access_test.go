package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHandoffAccess_SharedStoreJoinsNativeWriteRoots(t *testing.T) {
	repository, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	name := "agnostic-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = repository
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	for _, target := range []string{"codex", "qoder", "claude"} {
		for _, mode := range []string{"shared", "unignored store", "unavailable store", "handwritten config", "tracked config", "unmanaged config", "preview", "adopted tracked config", "adopted symlink config", "owned unavailable store", "adopted escaped tracked config"} {
			t.Run(target+"/"+mode, func(t *testing.T) {
				t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
				main, linked := t.TempDir(), filepath.Join(t.TempDir(), "linked")
				handoffInitGit(t, main)
				if mode == "unignored store" {
					handoffWrite(t, filepath.Join(main, ".gitignore"), ".claude/\n.codex/\n.qoder/\n")
				}
				if mode == "unavailable store" {
					handoffWrite(t, filepath.Join(main, ".agnostic-ai/local/handoffs"), "keep this file\n")
				}
				handoffGit(t, main, "worktree", "add", "-b", "handoff-access", linked)
				file := map[string]string{"codex": ".codex/config.toml", "qoder": ".qoder/settings.json", "claude": ".claude/settings.local.json"}[target]
				config := "version: 1\nbuiltins: [handoff]\ntargets: [" + target + "]\ngitignore:\n  enabled: true\n"
				if mode == "unmanaged config" {
					config += "sync:\n  unmanaged: [" + file + "]\n"
				}
				handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), config)
				if mode == "owned unavailable store" {
					for _, unavailable := range []bool{false, true} {
						if unavailable {
							handoffWrite(t, filepath.Join(main, ".gitignore"), ".claude/\n.codex/\n.qoder/\n")
						}
						cmd := exec.Command(binary, "sync")
						cmd.Dir = linked
						if output, err := cmd.CombinedOutput(); err != nil {
							t.Fatalf("sync with unavailable=%v: %v\n%s", unavailable, err, output)
						}
						content, err := os.ReadFile(filepath.Join(linked, file))
						if err != nil && (!unavailable || !os.IsNotExist(err)) {
							t.Fatal(err)
						}
						root := filepath.ToSlash(filepath.Join(main, ".agnostic-ai/local/handoffs"))
						if strings.Contains(string(content), root) == unavailable {
							t.Errorf("unavailable=%v: native access was not reconciled: %s", unavailable, content)
						}
					}
					return
				}
				if strings.HasPrefix(mode, "adopted ") {
					first := exec.Command(binary, "sync")
					first.Dir = linked
					if output, err := first.CombinedOutput(); err != nil {
						t.Fatalf("create owned handoff access: %v\n%s", err, output)
					}
					path := filepath.Join(linked, file)
					original := handoffRead(t, path)
					if !strings.Contains(original, filepath.ToSlash(filepath.Join(main, ".agnostic-ai/local/handoffs"))) {
						t.Fatal("first sync did not create handoff access")
					}
					if mode == "adopted escaped tracked config" {
						if target == "codex" {
							t.Skip("JSON escaping applies to Claude and Qoder")
						}
						original = strings.ReplaceAll(original, "/", `\/`)
						if !json.Valid([]byte(original)) {
							t.Fatal("escaped native settings are not valid JSON")
						}
						handoffWrite(t, path, original)
					}
					if strings.Contains(mode, "tracked config") {
						handoffGit(t, linked, "add", "-f", file)
					} else {
						destination := filepath.Join(t.TempDir(), "native settings")
						handoffWrite(t, destination, original)
						if err := os.Remove(path); err != nil {
							t.Fatal(err)
						}
						if err := os.Symlink(destination, path); err != nil {
							if runtime.GOOS == "windows" {
								t.Skipf("symlink unavailable: %v", err)
							}
							t.Fatal(err)
						}
					}
					for _, transition := range []string{"check", "sync", "unavailable store", "removal", "check removal"} {
						if transition == "unavailable store" {
							handoffWrite(t, filepath.Join(main, ".gitignore"), ".claude/\n.codex/\n.qoder/\n")
						}
						if transition == "removal" {
							handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), strings.Replace(config, "builtins: [handoff]", "builtins: []", 1))
						}
						args := []string{"sync"}
						if strings.HasPrefix(transition, "check") {
							args = append(args, "--check")
						}
						cmd := exec.Command(binary, args...)
						cmd.Dir = linked
						output, err := cmd.CombinedOutput()
						if !strings.HasPrefix(transition, "check") && err != nil {
							t.Errorf("%s: %v\n%s", transition, err, output)
						}
						if got, err := os.ReadFile(path); err != nil || string(got) != original {
							t.Fatalf("%s changed adopted native config: %v\n%s", transition, err, got)
						}
						if mode == "adopted symlink config" {
							if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSymlink == 0 {
								t.Fatalf("%s replaced native config symlink: %v", transition, err)
							}
						}
					}
					return
				}
				original := ""
				if mode == "handwritten config" || mode == "tracked config" || mode == "unmanaged config" {
					original = "{\"permissions\":{\"additionalDirectories\":[\"user-root\"]}}\n"
					if target == "codex" {
						original = "[sandbox_workspace_write]\nwritable_roots = [\"user-root\"]\n"
					}
					handoffWrite(t, filepath.Join(linked, file), original)
					if mode == "tracked config" {
						handoffGit(t, linked, "add", "-f", file)
					}
				}
				args := []string{"sync"}
				if mode == "preview" {
					args = append(args, "--dry-run")
				}
				cmd := exec.Command(binary, args...)
				cmd.Dir = linked
				output, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("sync: %v\n%s", err, output)
				}
				if mode == "unavailable store" && !strings.Contains(string(output), "use an ignored checkout store") {
					t.Errorf("unavailable store lacks fallback note: %s", output)
				}
				root := filepath.ToSlash(filepath.Join(main, ".agnostic-ai/local/handoffs"))
				content, readErr := os.ReadFile(filepath.Join(linked, file))
				if mode == "shared" {
					if readErr != nil || !strings.Contains(string(content), root) {
						t.Errorf("native settings do not grant shared handoff store %s: %v\n%s", root, readErr, content)
					}
					check := exec.Command(binary, "sync", "--check")
					check.Dir = linked
					if output, err := check.CombinedOutput(); err != nil {
						t.Fatalf("second sync differs: %v\n%s", err, output)
					}
					if target != "codex" {
						var doc map[string]any
						if err := json.Unmarshal(content, &doc); err != nil {
							t.Fatal(err)
						}
						permissions, ok := doc["permissions"].(map[string]any)
						if !ok {
							t.Fatal("native settings lack a permissions object")
						}
						directories, ok := permissions["additionalDirectories"].([]any)
						if !ok {
							t.Fatal("native settings lack additionalDirectories")
						}
						permissions["additionalDirectories"] = append(directories, "user-root")
						changed, err := json.Marshal(doc)
						if err != nil {
							t.Fatal(err)
						}
						handoffWrite(t, filepath.Join(linked, file), string(changed))
						handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), strings.Replace(config, "builtins: [handoff]", "builtins: []", 1))
						remove := exec.Command(binary, "sync")
						remove.Dir = linked
						if output, err := remove.CombinedOutput(); err != nil {
							t.Fatalf("drop handoff access: %v\n%s", err, output)
						}
						retained := handoffRead(t, filepath.Join(linked, file))
						if strings.Contains(retained, root) || !strings.Contains(retained, "user-root") {
							t.Errorf("dropping handoff access changed user directories: %s", retained)
						}
					}
				} else {
					if mode == "unavailable store" && handoffRead(t, filepath.FromSlash(root)) != "keep this file\n" {
						t.Error("sync changed the unavailable shared path")
					}
					if strings.Contains(string(content), root) {
						t.Errorf("unsafe or preview native settings contain shared path: %s", content)
					}
					if original != "" && string(content) != original {
						t.Errorf("preserved native config changed: %q", content)
					}
					if _, err := os.Stat(root); mode != "unavailable store" && !os.IsNotExist(err) {
						t.Errorf("unsafe or preview sync created shared store: %v", err)
					}
				}
			})
		}
	}
}
