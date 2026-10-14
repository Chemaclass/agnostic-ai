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
	binary := handoffBuildCLI(t)
	for _, target := range []string{"codex", "qoder", "claude"} {
		for _, mode := range []string{"shared", "unignored store", "unavailable store", "handwritten config", "tracked config", "unmanaged config", "preview", "adopted tracked config", "adopted symlink config", "owned unavailable store", "adopted escaped tracked config", "adopted missing symlink config", "adopted directory symlink config", "tracked MCP without handoff", "committed MCP without handoff"} {
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
					original = string(content)
					handoffGit(t, linked, "add", "-f", file)
					handoffWrite(t, filepath.Join(linked, "agnostic-ai.yaml"), strings.Replace(config, "[handoff]", "[]", 1))
					run("sync")
					if got := handoffRead(t, path); got != original {
						t.Errorf("removing builtin changed adopted config: %s", got)
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
	if !strings.Contains(string(output), "glob characters") {
		t.Errorf("missing fallback note: %s", output)
	}
	if _, err := os.Stat(filepath.Join(main, ".agnostic-ai/local/handoffs")); !os.IsNotExist(err) {
		t.Errorf("unsafe path created shared store: %v", err)
	}
}
