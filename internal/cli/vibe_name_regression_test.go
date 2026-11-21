package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestSyncUser_VibeNativeAliasIdentity(t *testing.T) {
	for _, mode := range []string{"equal", "different", "duplicate", "empty"} {
		t.Run(mode, func(t *testing.T) {
			home, source := userAgentTestHome(t)
			path := filepath.Join(home, ".vibe/config.toml")
			original := "# user\nkey=1\n\n[[mcp_servers]]\nname='docs.api'\ntransport='stdio'\ncommand='node'\n"
			if mode == "different" {
				original += "args=['different']\n"
			}
			if mode == "duplicate" {
				original += "\n[[mcp_servers]]\nname='docs_api'\ncommand='node'\n"
			}
			if mode == "empty" {
				original = "[[mcp_servers]]\nname='---'\ncommand='node'\n"
			}
			writeFile(t, path, original)
			writeFile(t, filepath.Join(source, "mcps/docs_api.yaml"), "name: docs_api\ncommand: node\n")
			_, w, err := runUserAgentTest("--only", "vibe")
			if mode != "equal" {
				if err == nil {
					t.Errorf("invalid/conflicting native alias accepted: %s", w)
				}
				if got := readFile(t, path); got != original {
					t.Errorf("conflict changed file: %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("adopt: %v %s", err, w)
			}
			if got := readFile(t, path); got != original {
				t.Fatalf("adoption changed bytes: %q", got)
			}
			state, err := loadUserState(filepath.Join(source, "state/global.json"))
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := state.MCP["vibe"]["docs.api"]; !ok {
				t.Fatal("ownership did not retain physical name")
			}
			if _, w, err := runUserAgentTest("--only", "vibe", "--check"); err != nil {
				t.Fatalf("repeat: %v %s", err, w)
			}
			writeFile(t, filepath.Join(source, "mcps/docs_api.yaml"), "name: docs_api\ncommand: updated\n")
			if _, w, err := runUserAgentTest("--only", "vibe"); err != nil {
				t.Fatalf("owned alias update: %v %s", err, w)
			}
			doc, err := readVibeTOML(path)
			if err != nil {
				t.Fatal(err)
			}
			records, err := nativeTOMLRecords(path, "mcp_servers", doc["mcp_servers"])
			if err != nil {
				t.Fatal(err)
			}
			if len(records) != 1 || records[0]["name"] != "docs.api" || records[0]["command"] != "updated" {
				t.Fatalf("physical spelling/update: %v", records)
			}
		})
	}
}
func TestImportVibe_NativeMCPIdentityRejectsBeforeWrites(t *testing.T) {
	for _, userLevel := range []bool{false, true} {
		t.Run(strconv.FormatBool(userLevel), func(t *testing.T) {
			home, source := userAgentTestHome(t)
			body := "[[mcp_servers]]\nname='docs.api'\ncommand='node'\n\n[[mcp_servers]]\nname='docs_api'\ncommand='node'\n"
			if userLevel {
				writeFile(t, filepath.Join(home, ".vibe/config.toml"), body)
				if _, _, err := runImportUserTest("vibe"); err == nil {
					t.Error("duplicate identity imported")
				}
				if _, err := os.Stat(source); !os.IsNotExist(err) {
					t.Error("invalid user import wrote sources")
				}
			} else {
				writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [vibe]\n")
				writeFile(t, ".vibe/config.toml", body)
				cmd := NewRootCmd("test")
				cmd.SetArgs([]string{"import", "vibe"})
				if err := cmd.Execute(); err == nil {
					t.Error("duplicate identity imported")
				}
				if _, err := os.Stat(".agnostic-ai"); !os.IsNotExist(err) {
					t.Error("invalid project import wrote sources")
				}
			}
		})
	}
}

func TestSyncUser_VibeNativeIdentityFailurePreservesStateAndMode(t *testing.T) {
	for _, scenario := range []string{"desired-collision", "invalid-current-unrelated", "renamed", "renamed-removal", "renamed-root-move", "ambiguous-state", "key-disagreement"} {
		t.Run(scenario, func(t *testing.T) {
			home, source := userAgentTestHome(t)
			path := filepath.Join(home, ".vibe/config.toml")
			mcp := filepath.Join(source, "mcps/docs.api.yaml")
			writeFile(t, path, "# keep\nkey=1\n")
			writeFile(t, mcp, "name: docs.api\ncommand: node\n")
			if _, w, err := runUserAgentTest("--only", "vibe"); err != nil {
				t.Fatalf("setup: %v %s", err, w)
			}
			statePath := filepath.Join(source, "state/global.json")
			switch scenario {
			case "desired-collision":
				writeFile(t, filepath.Join(source, "mcps/docs_api.yaml"), "name: docs_api\ncommand: node\n")
			case "invalid-current-unrelated":
				if err := os.Remove(mcp); err != nil {
					t.Fatal(err)
				}
				writeFile(t, filepath.Join(source, "settings/model.yaml"), "model: devstral\n")
				writeFile(t, path, readFile(t, path)+"\n[[mcp_servers]]\nname='---'\ncommand='node'\n")
			case "renamed", "renamed-removal", "renamed-root-move":
				writeFile(t, path, strings.ReplaceAll(readFile(t, path), `"docs.api"`, `"docs_api"`))
				if scenario == "renamed-removal" {
					if err := os.Remove(mcp); err != nil {
						t.Fatal(err)
					}
				}
				if scenario == "renamed-root-move" {
					t.Setenv("VIBE_HOME", filepath.Join(home, "new-vibe"))
				}
			case "ambiguous-state", "key-disagreement":
				state, err := loadUserState(statePath)
				if err != nil {
					t.Fatal(err)
				}
				name := "docs_api"
				record := map[string]any{"name": name, "transport": "stdio", "command": "node"}
				if scenario == "key-disagreement" {
					record["name"] = "other"
				}
				state.MCP["vibe"][name] = record
				data, err := json.Marshal(state)
				if err != nil {
					t.Fatal(err)
				}
				writeFile(t, statePath, string(data))
			}
			if err := os.Chmod(path, 0o600); err != nil {
				t.Fatal(err)
			}
			original, stateBefore := readFile(t, path), readFile(t, statePath)
			if _, _, err := runUserAgentTest("--only", "vibe"); err == nil {
				t.Error("identity conflict accepted")
			}
			if got := readFile(t, path); got != original {
				t.Error("identity conflict wrote native bytes")
			}
			if got := readFile(t, statePath); got != stateBefore {
				t.Error("identity conflict changed ownership")
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
				t.Error("identity conflict changed privacy")
			}
			if scenario == "renamed-root-move" {
				if _, err := os.Stat(filepath.Join(home, "new-vibe/config.toml")); !os.IsNotExist(err) {
					t.Error("failed root move wrote new native file")
				}
			}
		})
	}
}
func TestSyncUser_VibeNativeRawIdentityRestoresOriginalBytes(t *testing.T) {
	for _, adopted := range []bool{false, true} {
		t.Run(strconv.FormatBool(adopted), func(t *testing.T) {
			home, source := userAgentTestHome(t)
			path := filepath.Join(home, ".vibe/config.toml")
			original := "# unrelated\nkey=1\n"
			native := original
			name := "docs.api"
			if adopted {
				name = "docs_api"
				native += "\n[[mcp_servers]]\nname='docs.api'\ntransport='stdio'\ncommand='node'\n"
			}
			writeFile(t, path, native)
			mcp := filepath.Join(source, "mcps/"+name+".yaml")
			writeFile(t, mcp, "name: "+name+"\ncommand: node\n")
			if _, w, err := runUserAgentTest("--only", "vibe"); err != nil {
				t.Fatalf("add: %v %s", err, w)
			}
			if adopted && readFile(t, path) != native {
				t.Fatal("adoption changed physical bytes")
			}
			if _, w, err := runUserAgentTest("--only", "vibe", "--check"); err != nil {
				t.Fatalf("reload check: %v %s", err, w)
			}
			if !adopted {
				if err := os.Remove(mcp); err != nil {
					t.Fatal(err)
				}
				mcp = filepath.Join(source, "mcps/docs_api.yaml")
			}
			writeFile(t, mcp, "name: docs_api\ncommand: updated\n")
			if _, w, err := runUserAgentTest("--only", "vibe"); err != nil {
				t.Fatalf("alias update: %v %s", err, w)
			}
			if _, w, err := runUserAgentTest("--only", "vibe", "--check"); err != nil {
				t.Fatalf("updated alias reload: %v %s", err, w)
			}
			state, err := loadUserState(filepath.Join(source, "state/global.json"))
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := state.MCP["vibe"]["docs.api"]; !ok {
				t.Fatal("physical ownership lost")
			}
			if !adopted {
				if _, ok := state.TOMLArrays[path+"\x00mcp_servers"]["docs.api"]; !ok {
					t.Fatal("layout keyed by something other than physical name")
				}
			}
			if err := os.Remove(mcp); err != nil {
				t.Fatal(err)
			}
			if _, w, err := runUserAgentTest("--only", "vibe"); err != nil {
				t.Fatalf("remove: %v %s", err, w)
			}
			want := original
			if adopted {
				want += "\n"
			}
			if got := readFile(t, path); got != want {
				t.Fatalf("unrelated bytes: got %q want %q", got, want)
			}
		})
	}
}
func TestImportUser_VibeHomeNativeAliasCoverageChecksBodies(t *testing.T) {
	for _, equal := range []bool{false, true} {
		t.Run(strconv.FormatBool(equal), func(t *testing.T) {
			home, source := userAgentTestHome(t)
			src := filepath.Join(source, "mcps/docs_api.yaml")
			specBody := "name: docs_api\ncommand: node\n"
			writeFile(t, src, specBody)
			command := "node"
			if !equal {
				command = "different"
			}
			path := filepath.Join(home, ".vibe/config.toml")
			original := fmt.Sprintf("# native\n[[mcp_servers]]\nname='docs.api'\ncommand=%q\ntransport='stdio'\n", command)
			writeFile(t, path, original)
			_, _, err := runImportUserTest("vibe")
			if equal {
				if err != nil {
					t.Fatal(err)
				}
				entries, err := os.ReadDir(filepath.Join(source, "mcps"))
				if err != nil {
					t.Fatal(err)
				}
				if len(entries) != 1 {
					t.Fatalf("alias imported duplicate source: %v", entries)
				}
				if _, w, err := runUserAgentTest("--only", "vibe"); err != nil {
					t.Fatalf("import alias adoption: %v %s", err, w)
				}
				if _, w, err := runUserAgentTest("--only", "vibe", "--check"); err != nil {
					t.Fatalf("import alias sync check: %v %s", err, w)
				}
			} else if err == nil {
				t.Error("different home alias covered without conflict")
			}
			if readFile(t, src) != specBody || readFile(t, path) != original {
				t.Error("import changed source or native bytes")
			}
			if _, err := os.Stat(filepath.Join(source, "mcps/docs.api.yaml")); !os.IsNotExist(err) {
				t.Error("import wrote a colliding second spec")
			}
		})
	}
}
func TestImportVibe_NoncanonicalNativeNameRoundTripsFresh(t *testing.T) {
	for _, userLevel := range []bool{false, true} {
		t.Run(strconv.FormatBool(userLevel), func(t *testing.T) {
			home, _ := userAgentTestHome(t)
			path := ".vibe/config.toml"
			if userLevel {
				path = filepath.Join(home, path)
			} else {
				writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [vibe]\n")
			}
			writeFile(t, path, "[[mcp_servers]]\nname='docs.api'\ncommand='node'\ntransport='stdio'\n")
			if userLevel {
				if _, _, err := runImportUserTest("vibe"); err != nil {
					t.Fatal(err)
				}
			} else {
				execCLI(t, "import", "vibe")
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if userLevel {
				if _, w, err := runUserAgentTest("--only", "vibe"); err != nil {
					t.Fatalf("fresh sync: %v %s", err, w)
				}
				if _, w, err := runUserAgentTest("--only", "vibe", "--check"); err != nil {
					t.Fatalf("check: %v %s", err, w)
				}
			} else {
				execCLI(t, "sync", "-t", "vibe")
				execCLI(t, "sync", "--check", "-t", "vibe")
				execCLI(t, "lint")
			}
			doc, err := readVibeTOML(path)
			if err != nil {
				t.Fatal(err)
			}
			records, err := nativeTOMLRecords(path, "mcp_servers", doc["mcp_servers"])
			if err != nil {
				t.Fatal(err)
			}
			if len(records) != 1 || records[0]["name"] != "docs.api" {
				t.Fatalf("native spelling lost: %v", records)
			}
		})
	}
}

func TestSyncUser_VibeExcludedNativeAliasDoesNotCollide(t *testing.T) {
	home, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "mcps/docs_api.yaml"), "name: docs_api\ncommand: node\nx-vibe:\n  name: ignored\n")
	writeFile(t, filepath.Join(source, "mcps/docs.api.yaml"), "name: docs.api\ncommand: node\ntarget-exclude: vibe\n")
	if _, w, err := runUserAgentTest("--only", "vibe"); err != nil {
		t.Fatalf("excluded alias: %v %s", err, w)
	}
	path := filepath.Join(home, ".vibe/config.toml")
	doc, err := readVibeTOML(path)
	if err != nil {
		t.Fatal(err)
	}
	records, err := nativeTOMLRecords(path, "mcp_servers", doc["mcp_servers"])
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0]["name"] != "docs_api" {
		t.Fatalf("effective native name/filter: %v", records)
	}
}
func TestImportVibe_InvalidNativeMCPNameRejectsBeforeSources(t *testing.T) {
	for _, name := range []string{"---", "../docs", "bad\\name"} {
		t.Run(name, func(t *testing.T) {
			userAgentTestHome(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [vibe]\n")
			writeFile(t, ".vibe/config.toml", fmt.Sprintf("[[mcp_servers]]\nname=%q\ncommand='node'\n", name))
			cmd := NewRootCmd("test")
			cmd.SetArgs([]string{"import", "vibe"})
			if err := cmd.Execute(); err == nil {
				t.Error("invalid name accepted")
			}
			if _, err := os.Stat(".agnostic-ai"); !os.IsNotExist(err) {
				t.Error("invalid name wrote source directory")
			}
		})
	}
}
