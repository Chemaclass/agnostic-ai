package cli

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestSyncUser_GrokSharesConfigAndRestoresBytes(t *testing.T) {
	home, source := userAgentTestHome(t)
	t.Setenv("GROK_HOME", "")
	path := filepath.Join(home, ".grok", "config.toml")
	before := "# hand written\n[plugins]\nenabled = true\n[models]\n# preserve this\nother = 'keep'\n"
	writeFile(t, path, before)
	model := filepath.Join(source, "settings", "grok.yaml")
	mcp := filepath.Join(source, "mcps", "docs.yaml")
	writeFile(t, model, "model: grok-code\n")
	writeFile(t, mcp, "name: docs\ncommand: server\ndisabled: true\n")
	if _, warnings, err := runUserAgentTest("--only", "grok"); err != nil {
		t.Fatalf("%v %s", err, warnings)
	}
	got := readFile(t, path)
	for _, want := range []string{"default = \"grok-code\"", "enabled = false", "# preserve this", "other = 'keep'"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
	if _, _, err := runUserAgentTest("--only", "grok", "--check"); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{model, mcp} {
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := runUserAgentTest("--only", "grok"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != before {
		t.Errorf("did not restore exact bytes: %q", got)
	}
}
func TestSyncUser_GrokCustomRootAndNativeHookImport(t *testing.T) {
	home, source := userAgentTestHome(t)
	root := filepath.Join(home, "native-grok")
	t.Setenv("GROK_HOME", root)
	path := filepath.Join(root, "hooks", "guard.json")
	native := `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo safe","env":{"MODE":"safe"},"timeout":7}]}]}}`
	writeFile(t, path, native)
	writeFile(t, filepath.Join(root, "config.toml"), "[models]\ndefault = \"grok-code\"\n[mcp_servers.docs]\ncommand = \"server\"\nenabled = false\n")
	if _, warnings, err := runImportUserTest("grok"); err != nil {
		t.Fatalf("%v %s", err, warnings)
	}
	if _, err := os.Stat(filepath.Join(source, "hooks", "guard.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runUserAgentTest("--only", "grok"); err != nil {
		t.Fatalf("%v %s", err, warnings)
	}
	if got := readFile(t, path); got != native {
		t.Fatalf("native hook changed %q", got)
	}
	if _, _, err := runUserAgentTest("--only", "grok", "--check"); err != nil {
		t.Fatal(err)
	}
}

func TestSyncUser_GrokNewConfigIsPrivateAndRemoved(t *testing.T) {
	home, source := userAgentTestHome(t)
	t.Setenv("GROK_HOME", "")
	model := filepath.Join(source, "settings", "model.yaml")
	writeFile(t, model, "model: grok-code\n")
	if _, _, err := runUserAgentTest("--only", "grok"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".grok", "config.toml")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Errorf("mode %o", info.Mode().Perm())
	}
	if err := os.Remove(model); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runUserAgentTest("--only", "grok"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("created config remains: %v", err)
	}
}

func TestSyncUser_GrokPlansPrivateConfig(t *testing.T) {
	home, source := userAgentTestHome(t)
	t.Setenv("GROK_HOME", "")
	bundle := spec.Bundle{
		Settings: []spec.Entry{{Kind: spec.KindSettings, Name: "model", Meta: map[string]any{"model": "grok-code"}}},
		MCPs:     []spec.Entry{{Kind: spec.KindMCP, Name: "docs", Meta: map[string]any{"command": "server"}}},
	}
	writes, _, err := buildUserWrites(home, source, []string{"grok"}, nil, bundle, userState{Version: userStateVersion}, nil, io.Discard, "")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".grok", "config.toml")
	count := 0
	for _, write := range writes {
		if write.path != path {
			continue
		}
		count++
		if write.mode != 0600 || write.planned == nil {
			t.Error("combined config must plan a private write with an original snapshot")
		}
		if data := string(write.data); !strings.Contains(data, "grok-code") || !strings.Contains(data, "mcp_servers.docs") {
			t.Error("combined config plan lost the model or server")
		}
	}
	if count != 1 {
		t.Errorf("combined config writes = %d, want 1", count)
	}
}

func TestSyncUser_GrokCombinedConfigStopsOnConcurrentEdit(t *testing.T) {
	home, source := userAgentTestHome(t)
	t.Setenv("GROK_HOME", "")
	path := filepath.Join(home, ".grok", "config.toml")
	before := "# keep\n[plugins]\nx = 1\n"
	writeFile(t, path, before)
	bundle := spec.Bundle{Settings: []spec.Entry{{Kind: spec.KindSettings, Name: "model", Meta: map[string]any{"model": "grok-code"}}}, MCPs: []spec.Entry{{Kind: spec.KindMCP, Name: "docs", Meta: map[string]any{"command": "server"}}}}
	writes, _, err := buildUserWrites(home, source, []string{"grok"}, nil, bundle, userState{Version: userStateVersion}, nil, io.Discard, "")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, write := range writes {
		if write.path == path {
			count++
			if write.planned == nil || string(write.planned.data) != before {
				t.Fatal("combined write lost original snapshot")
			}
		}
	}
	if count != 1 {
		t.Fatalf("config writes %d", count)
	}
	changed := before + "# external write\n"
	writeFile(t, path, changed)
	if _, err := applyUserChanges(writes, nil, false, nil); err == nil || !strings.Contains(err.Error(), "changed while sync ran") {
		t.Fatalf("got %v", err)
	}
	if got := readFile(t, path); got != changed {
		t.Errorf("external write lost %q", got)
	}
}

func TestSyncUser_GrokRootMoveReleasesOldConfig(t *testing.T) {
	home, source := userAgentTestHome(t)
	oldRoot := filepath.Join(home, "old-grok")
	newRoot := filepath.Join(home, "new-grok")
	t.Setenv("GROK_HOME", oldRoot)
	before := "# keep old root\n[plugins]\nx = 1\n"
	path := filepath.Join(oldRoot, "config.toml")
	writeFile(t, path, before)
	writeFile(t, filepath.Join(source, "settings", "model.yaml"), "model: grok-code\n")
	writeFile(t, filepath.Join(source, "mcps", "docs.yaml"), "name: docs\ncommand: server\n")
	if _, _, err := runUserAgentTest("--only", "grok"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GROK_HOME", newRoot)
	if _, _, err := runUserAgentTest("--only", "grok"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != before {
		t.Errorf("old config changed %q", got)
	}
	got := readFile(t, filepath.Join(newRoot, "config.toml"))
	if !strings.Contains(got, "grok-code") || !strings.Contains(got, "mcp_servers.docs") {
		t.Errorf("new config missing owned fields %s", got)
	}
}

func TestSyncUser_GrokImportsRemoteTransportDefaults(t *testing.T) {
	home, source := userAgentTestHome(t)
	t.Setenv("GROK_HOME", "")
	path := filepath.Join(home, ".grok/config.toml")
	native := "[mcp_servers.http]\nurl = \"https://example.com/mcp\"\n[mcp_servers.sse]\nurl = \"https://example.com/sse\"\n"
	writeFile(t, path, native)
	if _, warnings, err := runImportUserTest("grok"); err != nil {
		t.Fatalf("%v %s", err, warnings)
	}
	for _, name := range []string{"http", "sse"} {
		if _, err := os.Stat(filepath.Join(source, "mcps", name+".yaml")); err != nil {
			t.Fatal(err)
		}
	}
	if _, warnings, err := runUserAgentTest("--only", "grok"); err != nil {
		t.Fatalf("%v %s", err, warnings)
	}
	if got := readFile(t, path); got != native {
		t.Fatalf("native transport changed %q", got)
	}
}
func TestSyncUser_GrokSkipsHookCredentials(t *testing.T) {
	home, source := userAgentTestHome(t)
	t.Setenv("GROK_HOME", "")
	path := filepath.Join(home, ".grok/hooks/guard.json")
	native := `{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"echo safe","env":{"API_TOKEN":"private-token-value"}}]}]}}`
	writeFile(t, path, native)
	out, warnings, err := runImportUserTest("grok")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out+warnings, "private-token-value") {
		t.Fatal("credential leaked into output")
	}
	if _, err := os.Stat(filepath.Join(source, "hooks/guard.yaml")); !os.IsNotExist(err) {
		t.Fatalf("credential imported %v", err)
	}
	if got := readFile(t, path); got != native {
		t.Fatal("native credential hook changed")
	}
}
func TestSyncUser_GrokImportKeepsExistingCasefoldMCPName(t *testing.T) {
	home, source := userAgentTestHome(t)
	t.Setenv("GROK_HOME", "")
	writeFile(t, filepath.Join(source, "mcps/docs.yaml"), "name: Docs\ncommand: existing\n")
	writeFile(t, filepath.Join(home, ".grok/config.toml"), "[mcp_servers.docs]\ncommand = \"different\"\n")
	if _, _, err := runImportUserTest("grok"); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(source, "mcps/docs.yaml"))
	if !strings.Contains(got, "existing") {
		t.Fatalf("existing casefold name changed %s", got)
	}
}
func TestSyncUser_GrokAdoptsEquivalentNativeMCPDefaults(t *testing.T) {
	home, source := userAgentTestHome(t)
	t.Setenv("GROK_HOME", "")
	path := filepath.Join(home, ".grok/config.toml")
	before := "# keep native spellings\n[mcp_servers.local]\ncommand = 'server'\n[mcp_servers.remote]\ntype = 'streamable-http'\nurl = 'https://example.com/mcp'\n"
	writeFile(t, path, before)
	writeFile(t, filepath.Join(source, "mcps/local.yaml"), "name: local\ncommand: server\ndisabled: false\n")
	writeFile(t, filepath.Join(source, "mcps/remote.yaml"), "name: remote\ntype: http\nurl: https://example.com/mcp\n")
	if _, warnings, err := runUserAgentTest("--only", "grok"); err != nil {
		t.Fatalf("%v %s", err, warnings)
	}
	if got := readFile(t, path); got != before {
		t.Errorf("changed native default bytes %q", got)
	}
}
func TestSyncUser_GrokRawHookStripsProjectRootFromSharedScript(t *testing.T) {
	home, source := userAgentTestHome(t)
	root := filepath.Join(home, "native-grok")
	t.Setenv("GROK_HOME", root)
	writeFile(t, filepath.Join(source, "scripts/guard.sh"), "#!/bin/sh\necho safe\n")
	writeFile(t, filepath.Join(source, "hooks/guard.yaml"), "name: guard\nevent: PreToolUse\ntarget: grok\nx-grok:\n  hooks:\n    PreToolUse:\n      - hooks:\n          - type: command\n            command: '$CLAUDE_PROJECT_DIR/.agnostic-ai/scripts/guard.sh'\n")
	if _, warnings, err := runUserAgentTest("--only", "grok"); err != nil {
		t.Fatalf("%v %s", err, warnings)
	}
	got := readFile(t, filepath.Join(root, "hooks/guard.json"))
	if strings.Contains(got, "CLAUDE_PROJECT_DIR") || !strings.Contains(got, filepath.ToSlash(filepath.Join(root, "hook-scripts/guard.sh"))) {
		t.Fatalf("wrong user script command %s", got)
	}
}
func TestSyncUser_GrokRejectsUnsupportedNativeTransport(t *testing.T) {
	home, source := userAgentTestHome(t)
	t.Setenv("GROK_HOME", "")
	writeFile(t, filepath.Join(source, "mcps/docs.yaml"), "name: docs\ntype: http\nurl: https://example.com/mcp\nx-grok:\n  type: unsupported\n")
	if _, _, err := runUserAgentTest("--only", "grok"); err == nil || !strings.Contains(err.Error(), "unsupported transport") {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".grok/config.toml")); !os.IsNotExist(err) {
		t.Fatalf("wrote invalid native config %v", err)
	}
}
