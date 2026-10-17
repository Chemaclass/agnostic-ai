package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestHandoffAccess_SharedStoreJoinsNativeWriteRoots(t *testing.T) {
	binary := handoffBuildCLI(t)
	for _, target := range []string{"codex", "qoder", "claude", "factory"} {
		for _, mode := range []string{"shared", "unignored store", "unavailable store", "handwritten config", "tracked config", "unmanaged config", "preview", "adopted tracked config", "adopted symlink config", "owned unavailable store", "adopted escaped tracked config", "adopted missing symlink config", "adopted directory symlink config", "tracked MCP without handoff", "committed MCP without handoff", "handoff off"} {
			t.Run(target+"/"+mode, func(t *testing.T) {
				t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
				main, linked := t.TempDir(), filepath.Join(t.TempDir(), "linked")
				physicalMain, err := filepath.EvalSymlinks(main)
				if err != nil {
					t.Fatal(err)
				}
				storeRoot := filepath.ToSlash(filepath.Join(physicalMain, ".agnostic-ai/local/handoffs"))
				handoffInitGit(t, main)
				if mode == "unignored store" {
					handoffWrite(t, filepath.Join(main, ".gitignore"), ".claude/\n.codex/\n.qoder/\n.factory/\n")
				}
				if mode == "unavailable store" {
					handoffWrite(t, filepath.Join(main, ".agnostic-ai/local/handoffs"), "keep this file\n")
				}
				handoffGit(t, main, "worktree", "add", "-b", "handoff-access", linked)
				file := map[string]string{"codex": ".codex/config.toml", "qoder": ".qoder/settings.json", "claude": ".claude/settings.local.json", "factory": ".factory/settings.local.json"}[target]
				config := "version: 1\nbuiltins: [handoff]\ntargets: [" + target + "]\ngitignore:\n  enabled: true\n"
				if mode == "unmanaged config" {
					config += "sync:\n  unmanaged: [" + file + "]\n"
				}
				if strings.Contains(mode, "MCP without handoff") {
					if target != "codex" {
						t.Skip("Codex owns its whole native MCP config")
					}
					config = "version: 1\nbuiltins: []\ntargets: [codex]\ngitignore:\n  enabled: false\n"
					if mode == "committed MCP without handoff" {
						config = "version: 1\nbuiltins: []\ntargets: [codex]\ngitignore:\n  enabled: true\n  commit: [mcps]\n"
					}
					handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), config)
					spec := filepath.Join(linked, ".agnostic-ai/mcps/service.yaml")
					handoffWrite(t, spec, "name: service\ncommand: original-server\n")
					first := exec.Command(binary, "sync")
					first.Dir = linked
					if output, err := first.CombinedOutput(); err != nil {
						t.Fatalf("create tracked MCP config: %v\n%s", err, output)
					}
					path := filepath.Join(linked, file)
					if !strings.Contains(handoffRead(t, path), "original-server") {
						t.Fatal("first sync did not write the MCP server")
					}
					handoffGit(t, linked, "add", "-f", file)
					handoffWrite(t, spec, "name: service\ncommand: updated-server\n")
					second := exec.Command(binary, "sync")
					second.Dir = linked
					if output, err := second.CombinedOutput(); err != nil {
						t.Fatalf("update tracked MCP config: %v\n%s", err, output)
					}
					if content := handoffRead(t, path); !strings.Contains(content, "updated-server") || strings.Contains(content, "original-server") {
						t.Errorf("handoff guard suppressed ordinary MCP update: %s", content)
					}
					return
				}
				handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), config)
				if mode == "owned unavailable store" {
					for _, unavailable := range []bool{false, true} {
						if unavailable {
							handoffWrite(t, filepath.Join(main, ".gitignore"), ".claude/\n.codex/\n.qoder/\n.factory/\n")
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
						root := storeRoot
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
					linkDestination := ""
					if !strings.Contains(original, storeRoot) {
						t.Fatal("first sync did not create handoff access")
					}
					if mode == "adopted escaped tracked config" {
						if target == "codex" {
							t.Skip("JSON escaping applies to the JSON settings files")
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
						linkDestination = destination
						handoffWrite(t, destination, original)
						if err := os.Remove(path); err != nil {
							t.Fatal(err)
						}
						switch mode {
						case "adopted missing symlink config":
							if err := os.Remove(destination); err != nil {
								t.Fatal(err)
							}
						case "adopted directory symlink config":
							if err := os.Remove(destination); err != nil {
								t.Fatal(err)
							}
							handoffWrite(t, filepath.Join(destination, "keep.txt"), "private directory\n")
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
							handoffWrite(t, filepath.Join(main, ".gitignore"), ".claude/\n.codex/\n.qoder/\n.factory/\n")
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
						unreadable := mode == "adopted missing symlink config" || mode == "adopted directory symlink config"
						if !unreadable {
							if got, err := os.ReadFile(path); err != nil || string(got) != original {
								t.Fatalf("%s changed adopted native config: %v\n%s", transition, err, got)
							}
						}
						if strings.Contains(mode, "symlink config") {
							if got, err := os.Readlink(path); err != nil || got != linkDestination {
								t.Fatalf("%s changed native config link target: %q, %v", transition, got, err)
							}
							if mode == "adopted missing symlink config" {
								if _, err := os.Stat(linkDestination); !os.IsNotExist(err) {
									t.Fatalf("%s created missing native config target: %v", transition, err)
								}
							} else if mode == "adopted directory symlink config" && handoffRead(t, filepath.Join(linkDestination, "keep.txt")) != "private directory\n" {
								t.Fatalf("%s changed the native config target directory", transition)
							}
							if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSymlink == 0 {
								t.Fatalf("%s replaced native config symlink: %v", transition, err)
							}
						}
					}
					return
				}
				if mode == "handoff off" {
					path := filepath.Join(linked, file)
					for _, step := range []string{"on", "off", "check"} {
						if step == "off" {
							handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), strings.Replace(config, "builtins: [handoff]", "builtins: []", 1))
						}
						args := []string{"sync"}
						if step == "check" {
							args = append(args, "--check")
						}
						cmd := exec.Command(binary, args...)
						cmd.Dir = linked
						if output, err := cmd.CombinedOutput(); err != nil {
							t.Fatalf("%s: %v\n%s", step, err, output)
						}
						if step == "on" && !strings.Contains(handoffRead(t, path), storeRoot) {
							t.Fatal("first sync did not grant the shared store")
						}
						if _, err := os.Stat(path); step != "on" && !os.IsNotExist(err) {
							t.Errorf("%s: a file that held only the grant stays: %v", step, err)
						}
					}
					return
				}
				original := ""
				if mode == "handwritten config" || mode == "tracked config" || mode == "unmanaged config" {
					original = "{\"permissions\":{\"additionalDirectories\":[\"user-root\"]}}\n"
					if target == "factory" {
						original = "{\"sandbox\":{\"filesystem\":{\"allowWrite\":[\"user-root\"]}}}\n"
					}
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
				root := storeRoot
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
						listPath := []string{"permissions", "additionalDirectories"}
						if target == "factory" {
							listPath = []string{"sandbox", "filesystem", "allowWrite"}
						}
						parent := doc
						for _, key := range listPath[:len(listPath)-1] {
							child, ok := parent[key].(map[string]any)
							if !ok {
								t.Fatalf("native settings lack %s", key)
							}
							parent = child
						}
						last := listPath[len(listPath)-1]
						directories, ok := parent[last].([]any)
						if !ok {
							t.Fatalf("native settings lack %s", last)
						}
						parent[last] = append(directories, "user-root")
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

func TestHandoffAccess_QoderHandoffOffWithoutMemoryRemovesSettings(t *testing.T) {
	binary := handoffBuildCLI(t)
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	main, linked := t.TempDir(), filepath.Join(t.TempDir(), "linked")
	physicalMain, err := filepath.EvalSymlinks(main)
	if err != nil {
		t.Fatal(err)
	}
	store := filepath.ToSlash(filepath.Join(physicalMain, ".agnostic-ai/local/handoffs"))
	handoffInitGit(t, main)
	handoffGit(t, main, "worktree", "add", "-b", "handoff-access", linked)
	config := "version: 1\nbuiltins: [handoff]\ntargets: [qoder]\ngitignore:\n  enabled: true\n"
	handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), config)
	handoffWrite(t, filepath.Join(linked, "agnostic-ai.local.yaml"), "memory:\n  personal: repo\n")
	path := filepath.Join(linked, ".qoder/settings.json")
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = linked
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, output)
		}
	}
	run("sync")
	if !strings.Contains(handoffRead(t, path), store) {
		t.Fatalf("first sync did not grant %s", store)
	}
	handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), strings.Replace(config, "builtins: [handoff]", "builtins: []", 1))
	run("sync")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("a file that held only the grant stays: %v", err)
	}
	run("sync", "--check")
}

func TestHandoffAccess_OpenCodeAndDevinKeepMemoryAndUserPermissions(t *testing.T) {
	binary := handoffBuildCLI(t)
	for _, target := range []string{"opencode", "windsurf"} {
		for _, builtin := range []string{"handoff", "handoff-hook"} {
			t.Run(target+"/"+builtin, func(t *testing.T) {
				t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
				main, linked := t.TempDir(), filepath.Join(t.TempDir(), "linked")
				handoffInitGit(t, main)
				handoffGit(t, main, "worktree", "add", "-b", "handoff-access", linked)
				physicalMain, err := filepath.EvalSymlinks(main)
				if err != nil {
					t.Fatal(err)
				}
				store := filepath.ToSlash(filepath.Join(physicalMain, ".agnostic-ai/local/handoffs"))
				memory := ""
				handoffWrite(t, filepath.Join(linked, "agnostic-ai.local.yaml"), "memory:\n  personal: repo\n")
				file := map[string]string{"opencode": "opencode.json", "windsurf": ".devin/config.json"}[target]
				config := "version: 1\nbuiltins: [memory]\ntargets: [" + target + "]\ngitignore:\n  enabled: true\n"
				sync := func(check bool) {
					t.Helper()
					args := []string{"sync"}
					if check {
						args = append(args, "--check")
					}
					cmd := exec.Command(binary, args...)
					cmd.Dir = linked
					if output, err := cmd.CombinedOutput(); err != nil {
						t.Fatalf("%v: %v\n%s", args, err, output)
					}
				}
				handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), config)
				sync(false)
				path := filepath.Join(linked, file)
				var doc map[string]any
				if err := json.Unmarshal([]byte(handoffRead(t, path)), &doc); err != nil {
					t.Fatal(err)
				}
				if target == "opencode" {
					permission := doc["permission"].(map[string]any)
					for pattern := range permission["external_directory"].(map[string]any) {
						memory = strings.TrimSuffix(pattern, "/**")
					}
					permission["bash"] = "deny"
					permission["external_directory"].(map[string]any)["user-root/**"] = "ask"
				} else {
					permission := doc["permissions"].(map[string]any)
					memory = strings.TrimSuffix(strings.TrimPrefix(permission["allow"].([]any)[0].(string), "Write("), "/**)")
					permission["deny"] = []any{"Exec(rm)"}
					permission["allow"] = append(permission["allow"].([]any), "Write(user-root/**)")
				}
				data, err := json.Marshal(doc)
				if err != nil {
					t.Fatal(err)
				}
				handoffWrite(t, path, string(data))
				for _, enabled := range []bool{false, true, false} {
					next := config
					if enabled {
						next = strings.Replace(config, "[memory]", "[memory, "+builtin+"]", 1)
					}
					handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), next)
					sync(false)
					sync(true)
					content := handoffRead(t, path)
					if err := json.Unmarshal([]byte(content), &doc); err != nil {
						t.Fatal(err)
					}
					if target == "opencode" {
						permission := doc["permission"].(map[string]any)
						rules := permission["external_directory"].(map[string]any)
						if rules[memory+"/**"] != "allow" || rules["user-root/**"] != "ask" || permission["bash"] != "deny" {
							t.Errorf("memory or user permissions changed: %s", content)
						}
						if (rules[store+"/**"] == "allow") != enabled {
							t.Errorf("handoff enabled=%v: %s", enabled, content)
						}
					} else {
						permission := doc["permissions"].(map[string]any)
						allow, _ := permission["allow"].([]any)
						contains := func(want string) bool {
							for _, rule := range allow {
								if rule == want {
									return true
								}
							}
							return false
						}
						if !contains("Write("+memory+"/**)") || !contains("Write(user-root/**)") || !strings.Contains(content, "Exec(rm)") {
							t.Errorf("memory or user permissions changed: %s", content)
						}
						if contains("Write("+store+"/**)") != enabled {
							t.Errorf("handoff enabled=%v: %s", enabled, content)
						}
					}
				}
			})
		}
	}
}

func TestHandoffAccess_OpenCodeAndDevinProtectNativeConfig(t *testing.T) {
	repository, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	binary := handoffBuildCLI(t)
	for _, target := range []string{"opencode", "windsurf"} {
		for _, mode := range []string{"shared", "handwritten", "tracked", "unmanaged", "unignored", "dry run", "adopted tracked"} {
			t.Run(target+"/"+mode, func(t *testing.T) {
				t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
				main, linked := t.TempDir(), filepath.Join(t.TempDir(), "linked")
				handoffInitGit(t, main)
				if mode == "unignored" {
					handoffWrite(t, filepath.Join(main, ".gitignore"), "opencode.json\n.devin/\n")
				}
				handoffGit(t, main, "worktree", "add", "-b", "handoff-access", linked)
				physicalMain, err := filepath.EvalSymlinks(main)
				if err != nil {
					t.Fatal(err)
				}
				store := filepath.ToSlash(filepath.Join(physicalMain, ".agnostic-ai/local/handoffs"))
				file := map[string]string{"opencode": "opencode.json", "windsurf": ".devin/config.json"}[target]
				path := filepath.Join(linked, file)
				config := "version: 1\nbuiltins: [handoff]\ntargets: [" + target + "]\ngitignore:\n  enabled: true\n"
				if mode == "unmanaged" {
					config += "sync:\n  unmanaged: [" + file + "]\n"
				}
				handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), config)
				original := ""
				if mode == "handwritten" || mode == "tracked" || mode == "unmanaged" {
					original = "{\"user_option\":true}\n"
					handoffWrite(t, path, original)
					if mode == "tracked" {
						handoffGit(t, linked, "add", "-f", file)
					}
				}
				run := func(args ...string) {
					t.Helper()
					cmd := exec.Command(binary, args...)
					cmd.Dir = linked
					if output, err := cmd.CombinedOutput(); err != nil {
						t.Fatalf("%v: %v\n%s", args, err, output)
					}
				}
				if mode == "dry run" {
					run("sync", "--dry-run")
				} else {
					run("sync")
				}
				content, err := os.ReadFile(path)
				granted := mode == "shared" || mode == "adopted tracked"
				if granted {
					if err != nil || !strings.Contains(string(content), store) {
						t.Fatalf("missing handoff grant: %v\n%s", err, content)
					}
					normalized := strings.ReplaceAll(string(content), store, "<main>/.agnostic-ai/local/handoffs")
					expected := handoffRead(t, filepath.Join(repository, "tests/integration/fixtures/handoff-permissions", target+".json"))
					if normalized != expected {
						t.Errorf("native handoff permissions differ from golden:\n%s", normalized)
					}
					run("sync", "--check")
				} else {
					if strings.Contains(string(content), store) {
						t.Errorf("unsafe or preview grant: %s", content)
					}
					if original != "" && string(content) != original {
						t.Errorf("native config changed: %s", content)
					}
					if _, err := os.Stat(store); !os.IsNotExist(err) {
						t.Errorf("unsafe or preview created store: %v", err)
					}
				}
				if mode == "adopted tracked" {
					handoffGit(t, linked, "add", "-f", file)
					handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), strings.Replace(config, "[handoff]", "[]", 1))
					run("sync")
					// Sync created this file for the grant alone, so it leaves
					// with the built-in.
					if got, err := os.ReadFile(path); err == nil {
						t.Errorf("removing builtin left the file sync created: %s", got)
					}
				}
			})
		}
	}
}

func TestHandoffAccess_OpenCodePreservesEditedGrant(t *testing.T) {
	binary := handoffBuildCLI(t)
	for _, action := range []string{"deny", "ask"} {
		t.Run(action, func(t *testing.T) {
			t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
			main, linked := t.TempDir(), filepath.Join(t.TempDir(), "linked")
			handoffInitGit(t, main)
			handoffGit(t, main, "worktree", "add", "-b", "handoff-access", linked)
			physicalMain, err := filepath.EvalSymlinks(main)
			if err != nil {
				t.Fatal(err)
			}
			pattern := filepath.ToSlash(filepath.Join(physicalMain, ".agnostic-ai/local/handoffs")) + "/**"
			config := "version: 1\nbuiltins: [handoff]\ntargets: [opencode]\ngitignore:\n  enabled: true\n"
			handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), config)
			run := func(args ...string) {
				t.Helper()
				cmd := exec.Command(binary, args...)
				cmd.Dir = linked
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%v: %v\n%s", args, err, output)
				}
			}
			run("sync")
			path := filepath.Join(linked, "opencode.json")
			var doc map[string]any
			if err := json.Unmarshal([]byte(handoffRead(t, path)), &doc); err != nil {
				t.Fatal(err)
			}
			rules := doc["permission"].(map[string]any)["external_directory"].(map[string]any)
			if rules[pattern] != "allow" {
				t.Fatalf("first sync did not generate grant: %v", rules)
			}
			rules[pattern] = action
			data, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			handoffWrite(t, path, string(data))
			for _, enabled := range []bool{true, false} {
				if !enabled {
					handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), strings.Replace(config, "[handoff]", "[]", 1))
				}
				run("sync")
				run("sync", "--check")
				if err := json.Unmarshal([]byte(handoffRead(t, path)), &doc); err != nil {
					t.Fatal(err)
				}
				rules = doc["permission"].(map[string]any)["external_directory"].(map[string]any)
				if rules[pattern] != action {
					t.Errorf("handoff enabled=%v: edited rule = %v, want %s", enabled, rules[pattern], action)
				}
				var state struct {
					Merged map[string]struct {
						Keys []struct {
							Path []string `json:"path"`
						} `json:"keys"`
					} `json:"merged"`
				}
				if err := json.Unmarshal([]byte(handoffRead(t, filepath.Join(linked, ".agnostic-ai/.sync-state"))), &state); err != nil {
					t.Fatal(err)
				}
				for _, claim := range state.Merged["opencode.json"].Keys {
					if len(claim.Path) == 3 && claim.Path[0] == "permission" && claim.Path[1] == "external_directory" && claim.Path[2] == pattern {
						t.Errorf("handoff enabled=%v: edited rule still owned: %v", enabled, claim.Path)
					}
				}
			}
		})
	}
}

func TestHandoffAccess_OpenCodePreservesSpecRestriction(t *testing.T) {
	binary := handoffBuildCLI(t)
	for _, action := range []string{"deny", "ask"} {
		t.Run(action, func(t *testing.T) {
			t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
			main, linked := t.TempDir(), filepath.Join(t.TempDir(), "linked")
			handoffInitGit(t, main)
			handoffGit(t, main, "worktree", "add", "-b", "handoff-access", linked)
			physicalMain, err := filepath.EvalSymlinks(main)
			if err != nil {
				t.Fatal(err)
			}
			pattern := filepath.ToSlash(filepath.Join(physicalMain, ".agnostic-ai/local/handoffs")) + "/**"
			quoted, err := json.Marshal(pattern)
			if err != nil {
				t.Fatal(err)
			}
			config := "version: 1\nbuiltins: [memory, handoff]\ntargets: [opencode]\ngitignore:\n  enabled: true\n"
			handoffWrite(t, filepath.Join(linked, "agnostic-ai.local.yaml"), "memory:\n  personal: repo\n")
			handoffWrite(t, filepath.Join(linked, ".agnostic-ai/settings/restriction.yaml"), "name: restriction\nx-opencode:\n  permission:\n    external_directory:\n      '*': ask\n      "+string(quoted)+": "+action+"\n")
			for _, enabled := range []bool{true, false} {
				next := config
				if !enabled {
					next = strings.Replace(config, "[memory, handoff]", "[memory]", 1)
				}
				handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), next)
				for _, args := range [][]string{{"sync"}, {"sync", "--check"}} {
					cmd := exec.Command(binary, args...)
					cmd.Dir = linked
					if output, err := cmd.CombinedOutput(); err != nil {
						t.Fatalf("%v: %v\n%s", args, err, output)
					}
				}
				content := handoffRead(t, filepath.Join(linked, "opencode.json"))
				var doc struct {
					Permission struct {
						ExternalDirectory map[string]string `json:"external_directory"`
					} `json:"permission"`
				}
				if err := json.Unmarshal([]byte(content), &doc); err != nil {
					t.Fatal(err)
				}
				rules := doc.Permission.ExternalDirectory
				if rules[pattern] != action || rules["*"] != "ask" {
					t.Errorf("handoff enabled=%v: spec restriction changed: %s", enabled, content)
				}
				memoryAllowed := false
				for key, value := range rules {
					if key != pattern && strings.HasSuffix(key, "/**") && value == "allow" {
						memoryAllowed = true
					}
				}
				if !memoryAllowed {
					t.Errorf("memory grant lost: %s", content)
				}
			}
		})
	}
}

func TestHandoffAccess_RefusesWildcardMainCheckoutGrant(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows filenames cannot contain literal wildcard characters")
	}
	binary := handoffBuildCLI(t)
	for _, target := range []string{"opencode", "windsurf"} {
		t.Run(target, func(t *testing.T) {
			t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
			main, linked := filepath.Join(t.TempDir(), "main*"), filepath.Join(t.TempDir(), "linked")
			if err := os.Mkdir(main, 0o700); err != nil {
				t.Fatal(err)
			}
			handoffInitGit(t, main)
			handoffGit(t, main, "worktree", "add", "-b", "handoff-access", linked)
			handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), "version: 1\nbuiltins: [handoff]\ntargets: ["+target+"]\ngitignore:\n  enabled: true\n")
			cmd := exec.Command(binary, "sync")
			cmd.Dir = linked
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("sync: %v\n%s", err, output)
			}
			file := map[string]string{"opencode": "opencode.json", "windsurf": ".devin/config.json"}[target]
			content, err := os.ReadFile(filepath.Join(linked, file))
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if strings.Contains(string(content), "main*/.agnostic-ai/local/handoffs") {
				t.Errorf("literal checkout became broad native glob: %s", content)
			}
			if !strings.Contains(string(output), "handoff") || !strings.Contains(string(output), "checkout store") {
				t.Errorf("missing safe fallback note: %s", output)
			}
			if _, err := os.Stat(filepath.Join(main, ".agnostic-ai/local/handoffs")); !os.IsNotExist(err) {
				t.Errorf("unsafe glob grant created shared store: %v", err)
			}
		})
	}
}

func handoffBuildCLI(t *testing.T) string {
	t.Helper()
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
	return binary
}

func TestHandoffAccess_OpenCodeAndDevinStillMergeSpecsIntoProtectedConfig(t *testing.T) {
	binary := handoffBuildCLI(t)
	for _, target := range []string{"opencode", "windsurf"} {
		for _, place := range []string{"main checkout", "linked worktree"} {
			for _, mode := range []string{"handwritten", "tracked"} {
				t.Run(target+"/"+place+"/"+mode, func(t *testing.T) {
					t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
					main := t.TempDir()
					handoffInitGit(t, main)
					dir := main
					if place == "linked worktree" {
						dir = filepath.Join(t.TempDir(), "linked")
						handoffGit(t, main, "worktree", "add", "-b", "handoff-access", dir)
					}
					file := map[string]string{"opencode": "opencode.json", "windsurf": ".devin/config.json"}[target]
					path := filepath.Join(dir, file)
					handoffWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\nbuiltins: [handoff]\ntargets: ["+target+"]\ngitignore:\n  enabled: true\n")
					handoffWrite(t, filepath.Join(dir, ".agnostic-ai/mcps/github.yaml"), "name: github\ntype: http\nurl: https://example.com/mcp/\n")
					handoffWrite(t, filepath.Join(dir, ".agnostic-ai/settings/perms.yaml"), "name: perms\npermissions:\n  deny: [Read(.env)]\n")
					handoffWrite(t, path, "{\"user_option\":true}\n")
					if mode == "tracked" {
						handoffGit(t, dir, "add", "-f", file)
					}
					cmd := exec.Command(binary, "sync")
					cmd.Dir = dir
					if output, err := cmd.CombinedOutput(); err != nil {
						t.Fatalf("sync: %v\n%s", err, output)
					}
					content := handoffRead(t, path)
					want := []string{`"user_option"`, `.env`}
					if target == "opencode" {
						want = append(want, `"mcp"`, `"github"`)
					}
					for _, text := range want {
						if !strings.Contains(content, text) {
							t.Errorf("missing %s: %s", text, content)
						}
					}
					if strings.Contains(content, ".agnostic-ai/local/handoffs") {
						t.Errorf("grant written into protected config: %s", content)
					}
				})
			}
		}
	}
}

func TestHandoffAccess_OpenCodeKeepsLaterUserRuleAfterMemoryGrant(t *testing.T) {
	binary := handoffBuildCLI(t)
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	dir := t.TempDir()
	handoffInitGit(t, dir)
	handoffWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\nbuiltins: [memory]\ntargets: [opencode]\ngitignore:\n  enabled: true\n")
	handoffWrite(t, filepath.Join(dir, "agnostic-ai.local.yaml"), "memory:\n  personal: repo\n")
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, output)
		}
	}
	run("sync")
	path := filepath.Join(dir, "opencode.json")
	var doc struct {
		Permission struct {
			ExternalDirectory map[string]string `json:"external_directory"`
		} `json:"permission"`
	}
	if err := json.Unmarshal([]byte(handoffRead(t, path)), &doc); err != nil {
		t.Fatal(err)
	}
	pattern := ""
	for key, value := range doc.Permission.ExternalDirectory {
		if value == "allow" {
			pattern = key
		}
	}
	if pattern == "" {
		t.Fatalf("no memory grant: %s", handoffRead(t, path))
	}
	quoted, err := json.Marshal(pattern)
	if err != nil {
		t.Fatal(err)
	}
	content := handoffRead(t, path)
	grant := string(quoted) + ": \"allow\""
	if !strings.Contains(content, grant) {
		t.Fatalf("unexpected layout: %s", content)
	}
	handoffWrite(t, path, strings.Replace(content, grant, grant+",\n      \"/**\": \"deny\"", 1))
	run("sync")
	run("sync", "--check")
	content = handoffRead(t, path)
	if !strings.Contains(content, `"/**": "deny"`) || strings.Index(content, `"/**"`) < strings.Index(content, string(quoted)) {
		t.Errorf("later deny no longer follows the memory grant: %s", content)
	}
}

func TestHandoffAccess_UnwritableConfigNoteNamesNativeEntry(t *testing.T) {
	binary := handoffBuildCLI(t)
	for target, want := range map[string]string{
		"opencode": `/.agnostic-ai/local/handoffs/**": "allow" to permission.external_directory`,
		"windsurf": `Write(`,
	} {
		t.Run(target, func(t *testing.T) {
			t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
			main, linked := t.TempDir(), filepath.Join(t.TempDir(), "linked")
			handoffInitGit(t, main)
			handoffGit(t, main, "worktree", "add", "-b", "handoff-access", linked)
			file := map[string]string{"opencode": "opencode.json", "windsurf": ".devin/config.json"}[target]
			handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), "version: 1\nbuiltins: [handoff]\ntargets: ["+target+"]\ngitignore:\n  enabled: true\n")
			handoffWrite(t, filepath.Join(linked, file), "{\"user_option\":true}\n")
			cmd := exec.Command(binary, "sync")
			cmd.Dir = linked
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("sync: %v\n%s", err, output)
			}
			if !strings.Contains(string(output), want) || target == "windsurf" && !strings.Contains(string(output), `/.agnostic-ai/local/handoffs/**) to permissions.allow`) {
				t.Errorf("note does not name the native entry: %s", output)
			}
		})
	}
}

func TestHandoffAccess_RefusesParenthesisInDevinCheckoutPath(t *testing.T) {
	binary := handoffBuildCLI(t)
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	main, linked := filepath.Join(t.TempDir(), "main(1)"), filepath.Join(t.TempDir(), "linked")
	if err := os.Mkdir(main, 0o700); err != nil {
		t.Fatal(err)
	}
	handoffInitGit(t, main)
	handoffGit(t, main, "worktree", "add", "-b", "handoff-access", linked)
	handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), "version: 1\nbuiltins: [handoff]\ntargets: [windsurf]\ngitignore:\n  enabled: true\n")
	cmd := exec.Command(binary, "sync")
	cmd.Dir = linked
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, output)
	}
	content, err := os.ReadFile(filepath.Join(linked, ".devin/config.json"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "main(1)") {
		t.Errorf("parenthesis path became a Write rule: %s", content)
	}
	if !strings.Contains(string(output), "pattern characters") {
		t.Errorf("missing fallback note: %s", output)
	}
	if _, err := os.Stat(filepath.Join(main, ".agnostic-ai/local/handoffs")); !os.IsNotExist(err) {
		t.Errorf("unsafe path created shared store: %v", err)
	}
}

func handoffLinkedSpecs(t *testing.T, dir, target, url, deny string) string {
	t.Helper()
	file := map[string]string{"opencode": "opencode.json", "windsurf": ".devin/config.json"}[target]
	handoffWrite(t, filepath.Join(dir, ".agnostic-ai/mcps/github.yaml"), "name: github\ntype: http\nurl: "+url+"\n")
	handoffWrite(t, filepath.Join(dir, ".agnostic-ai/settings/perms.yaml"), "name: perms\npermissions:\n  deny: ["+deny+"]\n")
	return file
}

func TestHandoffAccess_HandwrittenConfigConvergesInLinkedWorktree(t *testing.T) {
	binary := handoffBuildCLI(t)
	for _, target := range []string{"opencode", "windsurf"} {
		t.Run(target, func(t *testing.T) {
			t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
			main, linked := t.TempDir(), filepath.Join(t.TempDir(), "linked")
			handoffInitGit(t, main)
			handoffGit(t, main, "worktree", "add", "-b", "handoff-access", linked)
			handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), "version: 1\nbuiltins: [handoff]\ntargets: ["+target+"]\ngitignore:\n  enabled: true\n")
			file := handoffLinkedSpecs(t, linked, target, "https://example.com/mcp/", "Read(.env)")
			handoffWrite(t, filepath.Join(linked, file), "{\"user_option\":true}\n")
			run := func(args ...string) {
				t.Helper()
				cmd := exec.Command(binary, args...)
				cmd.Dir = linked
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%v: %v\n%s", args, err, output)
				}
			}
			run("sync")
			first := handoffRead(t, filepath.Join(linked, file))
			run("sync", "--check")
			run("sync")
			if second := handoffRead(t, filepath.Join(linked, file)); second != first || strings.Contains(second, ".agnostic-ai/local/handoffs") {
				t.Errorf("second sync changed the config or added a grant:\n%s\n---\n%s", first, second)
			}
		})
	}
}

func TestHandoffAccess_CommittedGrantStillTakesSpecChanges(t *testing.T) {
	binary := handoffBuildCLI(t)
	for _, target := range []string{"opencode", "windsurf"} {
		t.Run(target, func(t *testing.T) {
			t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
			main, linked := t.TempDir(), filepath.Join(t.TempDir(), "linked")
			handoffInitGit(t, main)
			handoffGit(t, main, "worktree", "add", "-b", "handoff-access", linked)
			config := "version: 1\nbuiltins: [handoff]\ntargets: [" + target + "]\ngitignore:\n  enabled: true\n"
			handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), config)
			file := handoffLinkedSpecs(t, linked, target, "https://example.com/one/", "Read(.env)")
			run := func(args ...string) {
				t.Helper()
				cmd := exec.Command(binary, args...)
				cmd.Dir = linked
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%v: %v\n%s", args, err, output)
				}
			}
			run("sync")
			path := filepath.Join(linked, file)
			if !strings.Contains(handoffRead(t, path), ".agnostic-ai/local/handoffs") {
				t.Fatalf("no grant generated: %s", handoffRead(t, path))
			}
			handoffGit(t, linked, "add", "-f", file)
			handoffLinkedSpecs(t, linked, target, "https://example.com/two/", "Read(.secret)")
			run("sync")
			run("sync", "--check")
			want := map[string]string{"opencode": "https://example.com/two/", "windsurf": "Read(.secret)"}[target]
			if !strings.Contains(handoffRead(t, path), want) {
				t.Errorf("spec change did not reach the committed config: %s", handoffRead(t, path))
			}
			if !strings.Contains(handoffRead(t, path), ".agnostic-ai/local/handoffs") {
				t.Errorf("grant sync wrote left the committed config: %s", handoffRead(t, path))
			}
			handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), strings.Replace(config, "[handoff]", "[]", 1))
			handoffLinkedSpecs(t, linked, target, "https://example.com/three/", "Read(.third)")
			run("sync")
			content := handoffRead(t, path)
			if strings.Contains(content, ".agnostic-ai/local/handoffs") || !strings.Contains(content, map[string]string{"opencode": "https://example.com/three/", "windsurf": "Read(.third)"}[target]) {
				t.Errorf("turning handoff off did not release the grant and keep merging: %s", content)
			}
		})
	}
}

func handoffUserGrant(t *testing.T, path, target, store string) {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(handoffRead(t, path)), &doc); err != nil {
		t.Fatal(err)
	}
	if target == "opencode" {
		permission, _ := doc["permission"].(map[string]any)
		if permission == nil {
			permission = map[string]any{}
			doc["permission"] = permission
		}
		rules, _ := permission["external_directory"].(map[string]any)
		if rules == nil {
			rules = map[string]any{}
			permission["external_directory"] = rules
		}
		rules[store+"/**"] = "allow"
	} else {
		permission, _ := doc["permissions"].(map[string]any)
		if permission == nil {
			permission = map[string]any{}
			doc["permissions"] = permission
		}
		allow, _ := permission["allow"].([]any)
		permission["allow"] = append(allow, "Write("+store+"/**)")
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	handoffWrite(t, path, string(data))
}

func TestHandoffAccess_RefusedGrantAddedByHandStays(t *testing.T) {
	binary := handoffBuildCLI(t)
	for _, target := range []string{"opencode", "windsurf"} {
		for _, mode := range []string{"handwritten", "committed"} {
			t.Run(target+"/"+mode, func(t *testing.T) {
				t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
				main, linked := t.TempDir(), filepath.Join(t.TempDir(), "linked")
				handoffInitGit(t, main)
				handoffGit(t, main, "worktree", "add", "-b", "handoff-access", linked)
				physicalMain, err := filepath.EvalSymlinks(main)
				if err != nil {
					t.Fatal(err)
				}
				store := filepath.ToSlash(filepath.Join(physicalMain, ".agnostic-ai/local/handoffs"))
				handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), "version: 1\nbuiltins: [handoff]\ntargets: ["+target+"]\ngitignore:\n  enabled: true\n")
				handoffWrite(t, filepath.Join(linked, ".agnostic-ai/settings/perms.yaml"), "name: perms\npermissions:\n  deny: [\"Bash(rm:*)\"]\n  allow: [\"Read(*)\"]\n")
				file := map[string]string{"opencode": "opencode.json", "windsurf": ".devin/config.json"}[target]
				path := filepath.Join(linked, file)
				run := func(args ...string) string {
					t.Helper()
					cmd := exec.Command(binary, args...)
					cmd.Dir = linked
					output, err := cmd.CombinedOutput()
					if err != nil {
						t.Fatalf("%v: %v\n%s", args, err, output)
					}
					return string(output)
				}
				if mode == "handwritten" {
					handoffWrite(t, path, "{\"user_option\":true}\n")
					run("sync")
					handoffUserGrant(t, path, target, store)
				} else {
					run("sync")
					handoffGit(t, linked, "add", "-f", file)
				}
				for i := 0; i < 2; i++ {
					output := run("sync")
					if strings.Contains(output, "stays as configured") {
						t.Errorf("sync %d repeats the note for a grant already there: %s", i+1, output)
					}
					run("sync", "--check")
					content := handoffRead(t, path)
					if !strings.Contains(content, store+"/**") {
						t.Errorf("sync %d dropped the grant: %s", i+1, content)
					}
					if mode == "handwritten" && !strings.Contains(content, "user_option") {
						t.Errorf("sync %d dropped user content: %s", i+1, content)
					}
				}
			})
		}
	}
}

func TestHandoffAccess_GrantAddedByHandSurvivesCheckAndHandoffOff(t *testing.T) {
	binary := handoffBuildCLI(t)
	for _, target := range []string{"opencode", "windsurf"} {
		for _, spec := range []bool{false, true} {
			name := target + "/no permissions spec"
			if spec {
				name = target + "/permissions spec"
			}
			t.Run(name, func(t *testing.T) {
				t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
				main, linked := t.TempDir(), filepath.Join(t.TempDir(), "linked")
				handoffInitGit(t, main)
				handoffGit(t, main, "worktree", "add", "-b", "handoff-access", linked)
				physicalMain, err := filepath.EvalSymlinks(main)
				if err != nil {
					t.Fatal(err)
				}
				store := filepath.ToSlash(filepath.Join(physicalMain, ".agnostic-ai/local/handoffs"))
				config := "version: 1\nbuiltins: [handoff]\ntargets: [" + target + "]\ngitignore:\n  enabled: true\n"
				handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), config)
				if spec {
					handoffWrite(t, filepath.Join(linked, ".agnostic-ai/settings/perms.yaml"), "name: perms\npermissions:\n  deny: [\"Bash(rm:*)\"]\n  allow: [\"Read(*)\"]\n")
				}
				file := map[string]string{"opencode": "opencode.json", "windsurf": ".devin/config.json"}[target]
				path := filepath.Join(linked, file)
				run := func(args ...string) string {
					t.Helper()
					cmd := exec.Command(binary, args...)
					cmd.Dir = linked
					output, _ := cmd.CombinedOutput()
					return string(output)
				}
				handoffWrite(t, path, "{\"user_option\":true}\n")
				run("sync")
				handoffUserGrant(t, path, target, store)
				if output := run("sync", "--check"); strings.Contains(output, "edited locally") || strings.Contains(output, "saves each as") {
					t.Errorf("check promises a backup sync does not make: %s", output)
				} else if spec && !strings.Contains(output, "keeps the keys it did not write and resets the ones it writes") {
					t.Errorf("check does not say what sync does to a merged file: %s", output)
				}
				run("sync")
				if _, err := os.Stat(path + ".bak"); err == nil {
					t.Error("sync saved a .bak for a merged config")
				}
				if output := run("sync", "--check"); strings.Contains(output, "drift") {
					t.Errorf("check still reports drift after sync: %s", output)
				}
				if content := handoffRead(t, path); !strings.Contains(content, store+"/**") {
					t.Fatalf("sync dropped the grant added by hand: %s", content)
				}
				handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), strings.Replace(config, "[handoff]", "[]", 1))
				output := run("sync")
				content := handoffRead(t, path)
				if !strings.Contains(content, store+"/**") {
					t.Errorf("turning handoff off dropped the grant added by hand (note: %q): %s", output, content)
				}
				if !strings.Contains(content, "user_option") {
					t.Errorf("turning handoff off dropped user content: %s", content)
				}
			})
		}
	}
}

func TestHandoffAccess_EveryGrantAddedByHandSurvives(t *testing.T) {
	binary := handoffBuildCLI(t)
	for _, target := range []string{"opencode", "windsurf"} {
		for _, spec := range []bool{false, true} {
			for _, state := range []string{"handoff on", "handoff off", "main checkout"} {
				t.Run(target+"/spec="+strconv.FormatBool(spec)+"/"+state, func(t *testing.T) {
					t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
					main := t.TempDir()
					handoffInitGit(t, main)
					dir := main
					if state != "main checkout" {
						dir = filepath.Join(t.TempDir(), "linked")
						handoffGit(t, main, "worktree", "add", "-b", "handoff-access", dir)
					}
					physicalMain, err := filepath.EvalSymlinks(main)
					if err != nil {
						t.Fatal(err)
					}
					stores := []string{"/old/main/.agnostic-ai/local/handoffs", "/zz/other/.agnostic-ai/local/handoffs"}
					if state != "main checkout" {
						stores = append(stores, filepath.ToSlash(filepath.Join(physicalMain, ".agnostic-ai/local/handoffs")))
					}
					config := "version: 1\nbuiltins: [handoff]\ntargets: [" + target + "]\ngitignore:\n  enabled: true\n"
					handoffWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), config)
					if spec {
						handoffWrite(t, filepath.Join(dir, ".agnostic-ai/settings/perms.yaml"), "name: perms\npermissions:\n  deny: [\"Bash(rm:*)\"]\n  allow: [\"Read(*)\"]\n")
					}
					file := map[string]string{"opencode": "opencode.json", "windsurf": ".devin/config.json"}[target]
					path := filepath.Join(dir, file)
					run := func(args ...string) {
						t.Helper()
						cmd := exec.Command(binary, args...)
						cmd.Dir = dir
						if output, err := cmd.CombinedOutput(); err != nil && args[len(args)-1] != "--check" {
							t.Fatalf("%v: %v\n%s", args, err, output)
						}
					}
					handoffWrite(t, path, "{\"user_option\":true}\n")
					run("sync")
					for _, store := range stores {
						handoffUserGrant(t, path, target, store)
					}
					denied := stores[1] + "/**"
					if target == "opencode" {
						var doc map[string]any
						if err := json.Unmarshal([]byte(handoffRead(t, path)), &doc); err != nil {
							t.Fatal(err)
						}
						doc["permission"].(map[string]any)["external_directory"].(map[string]any)[denied] = "deny"
						data, err := json.Marshal(doc)
						if err != nil {
							t.Fatal(err)
						}
						handoffWrite(t, path, string(data))
					}
					if state == "handoff off" {
						handoffWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), strings.Replace(config, "[handoff]", "[]", 1))
					}
					for i := 0; i < 2; i++ {
						run("sync")
						content := handoffRead(t, path)
						for _, store := range stores {
							if !strings.Contains(content, store+"/**") {
								t.Errorf("sync %d dropped the grant for %s: %s", i+1, store, content)
							}
						}
						if target == "opencode" {
							var doc struct {
								Permission struct {
									ExternalDirectory map[string]string `json:"external_directory"`
								} `json:"permission"`
							}
							if err := json.Unmarshal([]byte(content), &doc); err != nil {
								t.Fatal(err)
							}
							if got := doc.Permission.ExternalDirectory[denied]; got != "deny" {
								t.Errorf("sync %d changed %s to %q: %s", i+1, denied, got, content)
							}
						}
					}
				})
			}
		}
	}
}

func TestHandoffAccess_UpgradedStateWithoutMergedRecordSettlesInOneSync(t *testing.T) {
	binary := handoffBuildCLI(t)
	for _, target := range []string{"opencode", "windsurf"} {
		t.Run(target, func(t *testing.T) {
			t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
			main, linked := t.TempDir(), filepath.Join(t.TempDir(), "linked")
			handoffInitGit(t, main)
			handoffGit(t, main, "worktree", "add", "-b", "handoff-access", linked)
			config := "version: 1\nbuiltins: [handoff]\ntargets: [" + target + "]\ngitignore:\n  enabled: true\n"
			handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), strings.Replace(config, "[handoff]", "[]", 1))
			file := handoffLinkedSpecs(t, linked, target, "https://example.com/mcp/", "Read(.env)")
			run := func(args ...string) {
				t.Helper()
				cmd := exec.Command(binary, args...)
				cmd.Dir = linked
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%v: %v\n%s", args, err, output)
				}
			}
			run("sync")
			statePath := filepath.Join(linked, ".agnostic-ai/.sync-state")
			var state map[string]json.RawMessage
			if err := json.Unmarshal([]byte(handoffRead(t, statePath)), &state); err != nil {
				t.Fatal(err)
			}
			var merged map[string]json.RawMessage
			if err := json.Unmarshal(state["merged"], &merged); err != nil {
				t.Fatal(err)
			}
			delete(merged, file)
			state["merged"], _ = json.Marshal(merged)
			data, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			handoffWrite(t, statePath, string(data))
			handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), config)
			run("sync")
			first := handoffRead(t, filepath.Join(linked, file))
			run("sync", "--check")
			run("sync")
			if second := handoffRead(t, filepath.Join(linked, file)); second != first || !strings.Contains(first, ".agnostic-ai/local/handoffs") {
				t.Errorf("first sync after the upgrade did not settle:\n%s\n---\n%s", first, second)
			}
		})
	}
}

func TestHandoffAccess_TrackedConfigNoteAsksToUntrack(t *testing.T) {
	binary := handoffBuildCLI(t)
	for _, target := range []string{"opencode", "windsurf"} {
		t.Run(target, func(t *testing.T) {
			t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
			main, linked := t.TempDir(), filepath.Join(t.TempDir(), "linked")
			handoffInitGit(t, main)
			handoffGit(t, main, "worktree", "add", "-b", "handoff-access", linked)
			file := map[string]string{"opencode": "opencode.json", "windsurf": ".devin/config.json"}[target]
			handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), "version: 1\nbuiltins: [handoff]\ntargets: ["+target+"]\ngitignore:\n  enabled: true\n")
			handoffWrite(t, filepath.Join(linked, file), "{\"user_option\":true}\n")
			handoffGit(t, linked, "add", "-f", file)
			cmd := exec.Command(binary, "sync")
			cmd.Dir = linked
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("sync: %v\n%s", err, output)
			}
			if !strings.Contains(string(output), "git rm --cached "+file) || strings.Contains(string(output), "add \"") || strings.Contains(string(output), "Write(") {
				t.Errorf("note does not ask to untrack the file: %s", output)
			}
		})
	}
}

func TestHandoffAccess_DevinKeepsHandAddedDenyNextToSpecDeny(t *testing.T) {
	binary := handoffBuildCLI(t)
	for _, place := range []string{"main checkout", "linked worktree"} {
		t.Run(place, func(t *testing.T) {
			t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
			main := t.TempDir()
			handoffInitGit(t, main)
			dir := main
			if place == "linked worktree" {
				dir = filepath.Join(t.TempDir(), "linked")
				handoffGit(t, main, "worktree", "add", "-b", "devin-deny", dir)
			}
			physicalMain, err := filepath.EvalSymlinks(main)
			if err != nil {
				t.Fatal(err)
			}
			store := filepath.ToSlash(filepath.Join(physicalMain, ".agnostic-ai/local/handoffs")) + "/**"
			handoffWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\nbuiltins: [handoff]\ntargets: [windsurf]\ngitignore:\n  enabled: true\n")
			specPath := filepath.Join(dir, ".agnostic-ai/settings/perms.yaml")
			handoffWrite(t, specPath, "name: perms\npermissions:\n  deny: [\"Bash(rm:*)\"]\n")
			path := filepath.Join(dir, ".devin/config.json")
			handoffWrite(t, path, "{\"permissions\":{\"deny\":[\"Exec(curl)\",\"Write("+store+")\"]}}\n")
			run := func(args ...string) {
				t.Helper()
				cmd := exec.Command(binary, args...)
				cmd.Dir = dir
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%v: %v\n%s", args, err, output)
				}
			}
			denies := func() []string {
				t.Helper()
				var doc struct {
					Permissions struct {
						Deny []string `json:"deny"`
					} `json:"permissions"`
				}
				if err := json.Unmarshal([]byte(handoffRead(t, path)), &doc); err != nil {
					t.Fatal(err)
				}
				return doc.Permissions.Deny
			}
			run("sync")
			run("sync", "--check")
			got := denies()
			if len(got) != 3 || got[0] != "Exec(curl)" || got[1] != "Write("+store+")" || got[2] != "Exec(rm)" {
				t.Fatalf("deny = %v, want both hand entries then the spec's", got)
			}
			handoffWrite(t, specPath, "name: perms\npermissions:\n  deny: [\"Bash(sudo:*)\"]\n")
			run("sync")
			run("sync", "--check")
			got = denies()
			if len(got) != 3 || got[2] != "Exec(sudo)" {
				t.Fatalf("deny after a spec change = %v, want the hand entries and Exec(sudo)", got)
			}
			if err := os.Remove(specPath); err != nil {
				t.Fatal(err)
			}
			run("sync")
			run("sync", "--check")
			got = denies()
			if len(got) != 2 || got[0] != "Exec(curl)" || got[1] != "Write("+store+")" {
				t.Errorf("deny after the spec left = %v, want only the hand entries", got)
			}
		})
	}
}

func TestHandoffAccess_FactoryGrantKeepsSandboxSettings(t *testing.T) {
	binary := handoffBuildCLI(t)
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	main, linked := t.TempDir(), filepath.Join(t.TempDir(), "linked")
	physicalMain, err := filepath.EvalSymlinks(main)
	if err != nil {
		t.Fatal(err)
	}
	store := filepath.ToSlash(filepath.Join(physicalMain, ".agnostic-ai/local/handoffs"))
	handoffInitGit(t, main)
	handoffGit(t, main, "worktree", "add", "-b", "handoff-access", linked)
	config := "version: 1\nbuiltins: [handoff, handoff-hook]\ntargets: [factory]\ngitignore:\n  enabled: true\n"
	handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), config)
	path := filepath.Join(linked, ".factory/settings.local.json")
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = linked
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, output)
		}
		return string(output)
	}
	run("sync")
	created := handoffRead(t, path)
	if !strings.Contains(created, store) {
		t.Fatalf("first sync did not grant %s: %s", store, created)
	}
	handoffWrite(t, path, strings.Replace(created, "\"sandbox\": {", "\"model\": \"user-model\",\n  \"sandbox\": {\n    \"mode\": \"per-command\",", 1))
	handoffWrite(t, path, strings.Replace(handoffRead(t, path), "\"filesystem\": {", "\"filesystem\": {\n      \"denyWrite\": [\"/tmp/locks\"],", 1))
	run("sync")
	var doc struct {
		Model   string `json:"model"`
		Sandbox struct {
			Enabled    *bool  `json:"enabled"`
			Mode       string `json:"mode"`
			Filesystem struct {
				AllowWrite []string `json:"allowWrite"`
				DenyWrite  []string `json:"denyWrite"`
			} `json:"filesystem"`
		} `json:"sandbox"`
	}
	if err := json.Unmarshal([]byte(handoffRead(t, path)), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Sandbox.Filesystem.AllowWrite) != 1 || doc.Sandbox.Filesystem.AllowWrite[0] != store {
		t.Errorf("allowWrite does not grant %s: %v", store, doc.Sandbox.Filesystem.AllowWrite)
	}
	if doc.Sandbox.Enabled != nil {
		t.Error("sync set sandbox.enabled")
	}
	if doc.Model != "user-model" || doc.Sandbox.Mode != "per-command" || len(doc.Sandbox.Filesystem.DenyWrite) != 1 {
		t.Errorf("sync changed the user's settings: %+v", doc)
	}
	if _, err := os.Stat(filepath.Join(linked, ".factory/settings.json")); !os.IsNotExist(err) {
		t.Errorf("sync wrote the shared settings file: %v", err)
	}
	ignored := exec.Command("git", "check-ignore", "-q", ".factory/settings.local.json")
	ignored.Dir = linked
	if err := ignored.Run(); err != nil {
		t.Errorf("settings.local.json is not ignored: %v", err)
	}
	run("sync", "--check")
	handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), strings.Replace(config, "builtins: [handoff, handoff-hook]", "builtins: []", 1))
	run("sync")
	if content := handoffRead(t, path); strings.Contains(content, store) || !strings.Contains(content, "/tmp/locks") || !strings.Contains(content, "user-model") {
		t.Errorf("dropping handoff did not remove only the grant: %s", content)
	}
	ignoredOff := exec.Command("git", "check-ignore", "-q", ".factory/settings.local.json")
	ignoredOff.Dir = linked
	if err := ignoredOff.Run(); err != nil {
		t.Errorf("settings.local.json with user keys is not ignored after handoff turns off: %v", err)
	}
	run("sync", "--check")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), config)
	run("sync")
	handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), strings.Replace(config, "builtins: [handoff, handoff-hook]", "builtins: []", 1))
	run("sync")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("a file that held only the grant stays: %v", err)
	}
	run("sync", "--check")
}
