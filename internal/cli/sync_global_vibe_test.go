package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestSyncGlobal_VibeComposesNativeArraysAndSettings(t *testing.T) {
	home, source := globalAgentTestHome(t)
	root := filepath.Join(home, "custom-vibe")
	t.Setenv("VIBE_HOME", root)
	config := filepath.Join(root, "config.toml")
	hooks := filepath.Join(root, "hooks.toml")
	original := "# user\n[ui]\ncolor='blue'\n"
	mustWriteGlobalTest(t, config, original)
	if err := os.Chmod(config, 0o600); err != nil {
		t.Fatal(err)
	}
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "model.yaml"), "model:\n  vibe: devstral\n")
	mustWriteGlobalTest(t, filepath.Join(source, "mcps", "docs.yaml"), "name: docs\ntype: stdio\ncommand: node\n")
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\ntarget: vibe\nevent: pre_tool\ncommand: echo '{}'\n")
	if _, warnings, err := runGlobalAgentTest("--only", "vibe"); err != nil {
		t.Fatalf("sync: %v %s", err, warnings)
	}
	got := readGlobalTest(t, config)
	for _, want := range []string{`active_model = "devstral"`, `[[mcp_servers]]`, original} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
	if !strings.Contains(readGlobalTest(t, hooks), `type = "pre_tool"`) {
		t.Fatal("missing native hook")
	}
	info, err := os.Stat(config)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatal("private config mode widened")
	}
	for _, f := range []string{"settings/model.yaml", "mcps/docs.yaml", "hooks/guard.yaml"} {
		if err := os.Remove(filepath.Join(source, f)); err != nil {
			t.Fatal(err)
		}
	}
	if _, warnings, err := runGlobalAgentTest("--only", "vibe"); err != nil {
		t.Fatalf("remove: %v %s", err, warnings)
	}
	if got := readGlobalTest(t, config); got != original {
		t.Errorf("config not restored: got %q want %q", got, original)
	}
	if _, err := os.Stat(hooks); !os.IsNotExist(err) {
		t.Fatalf("created hook file remains: %v", err)
	}
}

func TestSyncGlobal_VibeDryRunNoopAndRootMoveProtectUserEdits(t *testing.T) {
	home, source := globalAgentTestHome(t)
	root := filepath.Join(home, "vibe-one")
	t.Setenv("VIBE_HOME", root)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "model.yaml"), "model: devstral\n")
	mustWriteGlobalTest(t, filepath.Join(source, "mcps", "docs.yaml"), "name: docs\ncommand: node\n")
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\ntarget: vibe\nevent: pre_tool\ncommand: echo '{}'\n")
	if _, _, err := runGlobalAgentTest("--only", "vibe", "--dry-run"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("dry run wrote files")
	}
	if _, _, err := runGlobalAgentTest("--only", "vibe"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "config.toml")
	before := readGlobalTest(t, path)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("created config mode = %v", info.Mode().Perm())
	}
	if _, _, err := runGlobalAgentTest("--only", "vibe", "--check"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGlobalAgentTest("--only", "vibe"); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(after.ModTime()) || readGlobalTest(t, path) != before {
		t.Fatal("no-op rewrote config")
	}
	edited := strings.Replace(before, `command = "node"`, `command = "edited"`, 1)
	mustWriteGlobalTest(t, path, edited)
	moved := filepath.Join(home, "vibe-two")
	t.Setenv("VIBE_HOME", moved)
	if _, _, err := runGlobalAgentTest("--only", "vibe"); err == nil {
		t.Fatal("root move must protect edited old MCP record")
	}
	if got := readGlobalTest(t, path); got != edited {
		t.Fatal("root move changed user edits")
	}
	if _, err := os.Stat(filepath.Join(moved, "config.toml")); !os.IsNotExist(err) {
		t.Fatal("failed plan wrote new root")
	}
}

func TestSyncGlobal_VibeInstructionsAgentsSkillsFollowHome(t *testing.T) {
	home, source := globalAgentTestHome(t)
	root := filepath.Join(home, "native-root")
	t.Setenv("VIBE_HOME", root)
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "Personal instructions.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "rules", "style.md"), "---\nname: style\n---\nPersonal rule.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Reviews\ncan: [read]\n---\nReview changes.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "check", "SKILL.md"), "---\nname: check\ndescription: Checks\n---\nCheck changes.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "check", "asset.txt"), "asset\n")
	if _, warnings, err := runGlobalAgentTest("--only", "vibe"); err != nil {
		t.Fatalf("%v %s", err, warnings)
	}
	for path, want := range map[string]string{"AGENTS.md": "Personal instructions.", "agents/reviewer.toml": `enabled_tools = ["read_file"]`, "skills/check/SKILL.md": "Check changes.", "skills/check/asset.txt": "asset\n"} {
		if got := readGlobalTest(t, filepath.Join(root, path)); !strings.Contains(got, want) {
			t.Errorf("%s: missing %q: %s", path, want, got)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".vibe")); !os.IsNotExist(err) {
		t.Fatal("default root used despite VIBE_HOME")
	}
}

func TestImportGlobal_VibeAdoptsUserEntriesAndExcludesOwnedRecords(t *testing.T) {
	home, source := globalAgentTestHome(t)
	root := filepath.Join(home, "vibe")
	t.Setenv("VIBE_HOME", root)
	config := "[[mcp_servers]]\nname='docs'\ntransport='http'\nurl='https://example.test/mcp'\n[mcp_servers.auth]\ntype='static'\napi_key_env='DOCS_TOKEN'\n"
	hooks := "# personal hook\n[[hooks]]\nname='guard'\ntype='pre_tool'\ncommand=\"echo '{}'\"\nstrict=false\n"
	mustWriteGlobalTest(t, filepath.Join(root, "config.toml"), config)
	mustWriteGlobalTest(t, filepath.Join(root, "hooks.toml"), hooks)
	if _, warnings, err := runImportGlobalTest("vibe"); err != nil {
		t.Fatalf("import: %v %s", err, warnings)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "vibe"); err != nil {
		t.Fatalf("adopt: %v %s", err, warnings)
	}
	if got := readGlobalTest(t, filepath.Join(root, "config.toml")); got != config {
		t.Errorf("adoption rewrote MCP: %s", got)
	}
	if got := readGlobalTest(t, filepath.Join(root, "hooks.toml")); got != hooks {
		t.Errorf("adoption rewrote hooks: %s", got)
	}
	for _, path := range []string{"mcps/docs.yaml", "hooks/guard.yaml"} {
		if err := os.Remove(filepath.Join(source, path)); err != nil {
			t.Fatal(err)
		}
	}
	if _, warnings, err := runImportGlobalTest("vibe"); err != nil {
		t.Fatalf("import owned: %v %s", err, warnings)
	}
	for _, path := range []string{"mcps/docs.yaml", "hooks/guard.yaml"} {
		if _, err := os.Stat(filepath.Join(source, path)); !os.IsNotExist(err) {
			t.Errorf("owned entry imported again: %s", path)
		}
	}
}

func TestSyncGlobal_VibePlansOneConfigWriteAndRejectsConcurrentEdit(t *testing.T) {
	home, source := globalAgentTestHome(t)
	root := filepath.Join(home, "vibe")
	t.Setenv("VIBE_HOME", root)
	path := filepath.Join(root, "config.toml")
	original := "# personal\n"
	mustWriteGlobalTest(t, path, original)
	bundle := spec.NewBundle([]spec.Entry{{Kind: spec.KindSettings, Name: "model", Meta: map[string]any{"model": "devstral"}}, {Kind: spec.KindMCP, Name: "docs", Meta: map[string]any{"command": "node"}}})
	writes, _, err := buildGlobalWrites(home, source, []string{"vibe"}, nil, bundle, globalState{Version: globalStateVersion}, nil, io.Discard, "warn")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, w := range writes {
		if w.path == path {
			count++
			if w.planned == nil || string(w.planned.data) != original {
				t.Fatal("missing original disk guard")
			}
			for _, want := range []string{"active_model", "mcp_servers"} {
				if !strings.Contains(string(w.data), want) {
					t.Errorf("combined plan lacks %s", want)
				}
			}
		}
	}
	if count != 1 {
		t.Fatalf("planned %d config writes", count)
	}
	edited := "# changed while sync ran\n"
	mustWriteGlobalTest(t, path, edited)
	if _, err := applyGlobalChanges(writes, nil, false, nil); err == nil || !strings.Contains(err.Error(), "changed while sync ran") {
		t.Fatalf("stale plan accepted: %v", err)
	}
	if got := readGlobalTest(t, path); got != edited {
		t.Fatal("concurrent user edit replaced")
	}
}
