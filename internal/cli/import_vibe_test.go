package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportVibe_HandwrittenNativeRoundTrip(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [vibe]\n")
	writeFile(t, "AGENTS.md", "Keep edits small.\n")
	writeFile(t, ".vibe/agents/reviewer.toml", "agent_type='subagent'\ndescription='Review diffs'\ninstructions='Review the diff.'\nenabled_tools=['read_file']\n[tools.read_file]\npermission='always'\n")
	writeFile(t, ".vibe/skills/check/SKILL.md", "---\nname: check\ndescription: Check edits\ndisable-model-invocation: true\nuser-invocable: true\n---\nRun checks.\n")
	writeFile(t, ".vibe/skills/check/reference.txt", "asset stays\n")
	writeFile(t, ".vibe/config.toml", "active_model='devstral'\nenable_mouse_capture=false\n[[mcp_servers]]\nname='docs'\ntransport='streamable-http'\nurl='https://example.test/mcp'\n[mcp_servers.auth]\ntype='static'\napi_key_env='DOCS_TOKEN'\n[[mcp_servers]]\nname='local'\ntransport='stdio'\ncommand=['node','server.js']\n[mcp_servers.env]\nNODE_ENV='production'\n")
	writeFile(t, ".vibe/hooks.toml", "[[hooks]]\nname='guard'\ntype='pre_tool'\ncommand=\"sh .vibe/hooks/guard.sh\"\nmatch='bash'\nstrict=true\n")
	writeFile(t, ".vibe/hooks/guard.sh", "#!/bin/sh\nprintf '%s\\n' '{\"decision\":\"deny\"}'\n")
	execCLI(t, "import", "vibe")
	for _, path := range []string{".vibe/agents/reviewer.toml", ".vibe/skills/check/SKILL.md", ".vibe/skills/check/reference.txt", ".vibe/config.toml", ".vibe/hooks.toml", ".vibe/hooks/guard.sh", "AGENTS.md"} {
		if err := os.Remove(filepath.Join(dir, path)); err != nil {
			t.Fatal(err)
		}
	}
	execCLI(t, "sync", "-t", "vibe")
	for path, wants := range map[string][]string{".vibe/agents/reviewer.toml": {"subagent", "read_file", "permission = \"always\""}, ".vibe/skills/check/SKILL.md": {"disable-model-invocation: true", "user-invocable: true"}, ".vibe/skills/check/reference.txt": {"asset stays"}, ".vibe/config.toml": {"enable_mouse_capture = false", "api_key_env = \"DOCS_TOKEN\"", "production", "server.js"}, ".vibe/hooks.toml": {"strict = true", "pre_tool"}, ".vibe/hooks/guard.sh": {"decision"}, "AGENTS.md": {"Keep edits small"}} {
		data := readFile(t, filepath.Join(dir, path))
		for _, want := range wants {
			if !strings.Contains(data, want) {
				t.Errorf("%s missing %q: %s", path, want, data)
			}
		}
		if filepath.Ext(path) == ".toml" {
			if _, err := toml.Decode(data, new(map[string]any)); err != nil {
				t.Fatal(err)
			}
		}
	}
	first := snapshotEmitted(t, dir)
	execCLI(t, "sync", "-t", "vibe")
	assertEmittedEqual(t, first, snapshotEmitted(t, dir))
	execCLI(t, "sync", "--check", "-t", "vibe")
	execCLI(t, "lint")
}

func TestImportVibe_RejectsNativeSettingsCredentialWithoutPrintingIt(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [vibe]\n")
	secret := strings.Repeat("x", 32)
	writeFile(t, ".vibe/config.toml", "active_model='devstral'\n[[providers]]\nname='private'\napi_key='"+secret+"'\n")
	cmd := NewRootCmd("test")
	cmd.SetArgs([]string{"import", "vibe"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("native provider credential must stop import")
	}
	if strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "literal credential") {
		t.Fatalf("unsafe or unclear error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai/settings/vibe.yaml")); !os.IsNotExist(err) {
		t.Fatal("credential reached a source settings file")
	}
}
