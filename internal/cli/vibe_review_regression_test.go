package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"io"

	"github.com/chemaclass/agnostic-ai/internal/adapters/vibe"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportVibe_UnsafeNativeHookNameWritesNothing(t *testing.T) {
	for _, name := range []string{"../../victim", `..\..\victim`, `C:\outside\victim`, "/outside/victim"} {
		t.Run(name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [vibe]\n")
			sentinel := filepath.Join(dir, "victim.yaml")
			writeFile(t, sentinel, "sentinel\n")
			writeFile(t, ".vibe/hooks.toml", fmt.Sprintf("[[hooks]]\nname=%q\ntype='pre_tool'\ncommand='check'\n", name))
			cmd := NewRootCmd("test")
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs([]string{"import", "vibe"})
			if err := cmd.Execute(); err == nil {
				t.Error("unsafe native name accepted")
			}
			if got := readGlobalTest(t, sentinel); got != "sentinel\n" {
				t.Fatal("native name overwrote sentinel")
			}
			if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai")); !os.IsNotExist(err) {
				t.Error("invalid native name wrote source files or directories")
			}
		})
	}
}
func TestImportGlobal_VibeUnsafeHookDryRunKeepsSentinel(t *testing.T) {
	home, source := globalAgentTestHome(t)
	sentinel := filepath.Join(home, "victim.yaml")
	mustWriteGlobalTest(t, sentinel, "sentinel\n")
	relative, err := filepath.Rel(os.TempDir(), strings.TrimSuffix(sentinel, ".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	name := "../../" + filepath.ToSlash(relative)
	mustWriteGlobalTest(t, filepath.Join(home, ".vibe/hooks.toml"), fmt.Sprintf("[[hooks]]\nname=%q\ntype='pre_tool'\ncommand='check'\n", name))
	if _, _, err := runImportGlobalTest("vibe", "--dry-run"); err == nil {
		t.Error("unsafe global hook name accepted")
	}
	if got := readGlobalTest(t, sentinel); got != "sentinel\n" {
		t.Fatal("global dry run overwrote sentinel")
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Error("global dry run wrote portable source")
	}
}

func TestImportGlobal_VibeMCPUsesSharedCollisionRegistry(t *testing.T) {
	for _, order := range [][]string{{"codex", "vibe"}, {"vibe", "codex"}} {
		for _, scenario := range []string{"different", "equivalent", "case-collision"} {
			t.Run(strings.Join(order, "-")+"/"+scenario, func(t *testing.T) {
				home, source := globalAgentTestHome(t)
				codexCommand, vibeCommand, vibeName := "codex-server", "vibe-server", "docs"
				if scenario != "different" {
					vibeCommand = codexCommand
				}
				if scenario == "case-collision" {
					vibeName = "DOCS"
				}
				mustWriteGlobalTest(t, filepath.Join(home, ".codex/config.toml"), fmt.Sprintf("[mcp_servers.docs]\ncommand=%q\n", codexCommand))
				mustWriteGlobalTest(t, filepath.Join(home, ".vibe/config.toml"), fmt.Sprintf("[[mcp_servers]]\nname=%q\ntransport='stdio'\ncommand=%q\n", vibeName, vibeCommand))
				_, warnings, err := runImportGlobalTest(order...)
				if err != nil {
					t.Fatal(err)
				}
				conflict := strings.Contains(warnings, "differently")
				if conflict != (scenario != "equivalent") {
					t.Errorf("wrong collision diagnostic: %s", warnings)
				}
				entries, err := os.ReadDir(filepath.Join(source, "mcps"))
				if err != nil {
					t.Fatal(err)
				}
				if len(entries) != 1 {
					t.Fatalf("wanted one case-insensitive output, got %v", entries)
				}
				got := readGlobalTest(t, filepath.Join(source, "mcps", entries[0].Name()))
				wantCommand := codexCommand
				if order[0] == "vibe" {
					wantCommand = vibeCommand
				}
				if !strings.Contains(got, wantCommand) {
					t.Errorf("first target did not win: %s", got)
				}
				if scenario == "equivalent" {
					if _, warnings, err := runGlobalAgentTest("--only", "codex,vibe"); err != nil {
						t.Fatalf("equivalent imports cannot sync: %v %s", err, warnings)
					}
				}
			})
		}
	}
}

func TestSyncGlobal_VibeScalarEditsPreserveArraySeparatorOwnership(t *testing.T) {
	for _, update := range []bool{false, true} {
		t.Run(fmt.Sprint(update), func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := filepath.Join(home, ".vibe/config.toml")
			original := "key=1\n"
			mustWriteGlobalTest(t, path, original)
			model := filepath.Join(source, "settings/model.yaml")
			mcp := filepath.Join(source, "mcps/docs.yaml")
			mustWriteGlobalTest(t, model, "model: old-model\n")
			mustWriteGlobalTest(t, mcp, "name: docs\ncommand: node\n")
			if _, w, err := runGlobalAgentTest("--only", "vibe"); err != nil {
				t.Fatalf("add: %v %s", err, w)
			}
			if update {
				mustWriteGlobalTest(t, model, "model: new-model\n")
				if _, w, err := runGlobalAgentTest("--only", "vibe"); err != nil {
					t.Fatalf("model-only update: %v %s", err, w)
				}
			}
			for _, p := range []string{model, mcp} {
				if err := os.Remove(p); err != nil {
					t.Fatal(err)
				}
			}
			if _, w, err := runGlobalAgentTest("--only", "vibe"); err != nil {
				t.Fatalf("remove: %v %s", err, w)
			}
			if got := readGlobalTest(t, path); got != original {
				t.Fatalf("separator restoration: got %q want %q", got, original)
			}
		})
	}
}

func TestTOMLArray_UnnamedTimeoutUpdateIsAnOwnedTransition(t *testing.T) {
	old := map[string]any{"event": "before", "command": "guard", "timeout": int64(10)}
	data := []byte("[[hooks]]\nevent='before'\ncommand='guard'\ntimeout=10\n")
	want := map[string]map[string]any{"source-guard": {"event": "before", "command": "guard", "timeout": 20}}
	updated, _, err := mergeTOMLArrayRecords("config.toml", data, "hooks", want, map[string]any{"source-guard": old}, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(updated.data, []byte("timeout = 20")) {
		t.Fatalf("owned timeout not changed: %s", updated.data)
	}
	handEdited := bytes.Replace(data, []byte("timeout=10"), []byte("timeout=30"), 1)
	if _, _, err := mergeTOMLArrayRecords("config.toml", handEdited, "hooks", want, map[string]any{"source-guard": old}, nil, true); err == nil {
		t.Fatal("preexisting user timeout edit was not protected")
	}
}

func TestImportVibe_PostAgentStrictFalseRoundTripsFresh(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [vibe]\n")
	writeFile(t, ".vibe/hooks.toml", "[[hooks]]\nname='complete'\ntype='post_agent'\ncommand='check'\nstrict=false\n")
	execCLI(t, "import", "vibe")
	if err := os.Remove(filepath.Join(dir, ".vibe/hooks.toml")); err != nil {
		t.Fatal(err)
	}
	execCLI(t, "sync")
	if got := readGlobalTest(t, filepath.Join(dir, ".vibe/hooks.toml")); !strings.Contains(got, "strict = false") {
		t.Fatalf("false native default lost: %s", got)
	}
	execCLI(t, "sync", "--check")
}
func TestImportGlobal_VibePostAgentStrictFalseRoundTripsFresh(t *testing.T) {
	home, _ := globalAgentTestHome(t)
	path := filepath.Join(home, ".vibe/hooks.toml")
	mustWriteGlobalTest(t, path, "[[hooks]]\nname='complete'\ntype='post_agent'\ncommand='check'\nstrict=false\n")
	if _, _, err := runImportGlobalTest("vibe"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, w, err := runGlobalAgentTest("--only", "vibe"); err != nil {
		t.Fatalf("sync: %v %s", err, w)
	}
	if got := readGlobalTest(t, path); !strings.Contains(got, "strict = false") {
		t.Fatalf("false native default lost: %s", got)
	}
	if _, _, err := runGlobalAgentTest("--only", "vibe", "--check"); err != nil {
		t.Fatal(err)
	}
}

func TestSyncGlobal_VibeCollectsWindowsHookVariant(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "scripts/guard.sh"), "posix body\n")
	mustWriteGlobalTest(t, filepath.Join(source, "scripts/guard.ps1"), "windows body\n")
	hook := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "pre_tool", "command": "sh .agnostic-ai/scripts/guard.sh", "commandWindows": "powershell -File .agnostic-ai/scripts/guard.ps1"}}
	writes, _, err := buildGlobalWritesForPlatform(home, source, []string{"vibe"}, nil, spec.Bundle{Hooks: []spec.Entry{hook}}, globalState{Version: globalStateVersion}, nil, io.Discard, "warn", "windows")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range writes {
		if w.path == filepath.Join(home, ".vibe/hooks/guard.ps1") {
			found = true
			if string(w.data) != "windows body\n" {
				t.Fatal("wrong Windows script")
			}
		}
	}
	if !found {
		t.Fatal("global commandWindows references a script missing from its plan")
	}
	for _, w := range writes {
		if w.path == filepath.Join(home, ".vibe/hooks.toml") {
			doc, err := decodeTOMLDocument(w.path, w.data)
			if err != nil {
				t.Fatal(err)
			}
			records, err := nativeTOMLRecords(w.path, "hooks", doc["hooks"])
			if err != nil {
				t.Fatal(err)
			}
			command, _ := records[0]["command"].(string)
			declared := vibe.DeclaredHookCommand(command)
			if !strings.Contains(declared, "guard.ps1") || strings.Contains(declared, "guard.sh") {
				t.Fatalf("forced Windows plan selected wrong command: %s", declared)
			}
		}
	}
}

func TestSyncGlobal_VibeSeparatorProofKeepsUserScalarChanges(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".vibe/config.toml")
	mustWriteGlobalTest(t, path, "key=1\n")
	model := filepath.Join(source, "settings/model.yaml")
	mcp := filepath.Join(source, "mcps/docs.yaml")
	mustWriteGlobalTest(t, model, "model: old-model\n")
	mustWriteGlobalTest(t, mcp, "name: docs\ncommand: node\n")
	if _, _, err := runGlobalAgentTest("--only", "vibe"); err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(readGlobalTest(t, path), "key=1", "key=2", 1)
	mustWriteGlobalTest(t, path, text)
	for _, p := range []string{model, mcp} {
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := runGlobalAgentTest("--only", "vibe"); err != nil {
		t.Fatal(err)
	}
	if got := readGlobalTest(t, path); got != "key=2\n" {
		t.Fatalf("owned separator cleanup changed user scalar: %q", got)
	}
}

func TestSyncGlobal_VibeWindowsVariantRejectsUnsafeArguments(t *testing.T) {
	home, source := globalAgentTestHome(t)
	hook := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "pre_tool", "command": "python", "commandWindows": "powershell", "args": []any{"a'b"}}}
	if _, _, err := buildGlobalWritesForPlatform(home, source, []string{"vibe"}, nil, spec.Bundle{Hooks: []spec.Entry{hook}}, globalState{Version: globalStateVersion}, nil, io.Discard, "warn", "windows"); err == nil {
		t.Fatal("selected Windows command ignored unsafe arguments")
	}
	if _, err := os.Stat(filepath.Join(home, ".vibe")); !os.IsNotExist(err) {
		t.Fatal("failed native Windows plan wrote configuration")
	}
}
