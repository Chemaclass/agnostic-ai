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

func TestImportVibe_NotesAgentNamedMain(t *testing.T) {
	testutil.TempCwd(t)
	out := captureLogOut(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [vibe]\n")
	writeFile(t, ".vibe/agents/main.toml", "agent_type='subagent'\ndescription='Main reviewer'\ninstructions='Review.'\n")
	execCLI(t, "import", "vibe")
	for _, want := range []string{".vibe/agents/main.toml", "reserves the name main", "rename"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("import note lacks %q:\n%s", want, out.String())
		}
	}
	cmd := NewRootCmd("test")
	cmd.SetArgs([]string{"sync", "-t", "vibe"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "reserves the name main") {
		t.Fatalf("sync must stop on the reserved agent name: %v", err)
	}
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

func TestSyncVibe_UnmappedToolCoverageStopsBeforeWriting(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "targets: [vibe]\ncoverage:\n  fail-on-notes: true\n")
	writeFile(t, ".agnostic-ai/agents/reader.md", "---\nname: reader\ndescription: Read source files.\ntools: [Read, Glob]\n---\nReview requested files.\n")
	cmd := NewRootCmd("test")
	cmd.SetArgs([]string{"sync", "-t", "vibe"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "coverage.fail-on-notes") {
		t.Fatalf("unmapped tool bypassed coverage policy: %v", err)
	}
	if _, err := os.Stat(".vibe/agents/reader.toml"); !os.IsNotExist(err) {
		t.Fatalf("failed coverage check wrote the native agent: %v", err)
	}
}

func TestImportVibe_NativeSkillToolsStayScoped(t *testing.T) {
	for _, tools := range []string{"[read_file, ask_user_question]", "read_file", "[ask_user_question]"} {
		t.Run(tools, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [vibe]\nbuiltins: []\n")
			writeFile(t, ".vibe/skills/probe/SKILL.md", "---\nname: probe\ndescription: Inspect files.\nallowed-tools: "+tools+"\n---\nInspect files.\n")
			execCLI(t, "import", "vibe")
			imported := readFile(t, filepath.Join(dir, ".agnostic-ai/skills/probe/SKILL.md"))
			if !strings.Contains(imported, "x-vibe:") {
				t.Errorf("native tools must be scoped: %s", imported)
			}
			execCLI(t, "sync", "--only", "vibe")
			first := readFile(t, filepath.Join(dir, ".vibe/skills/probe/SKILL.md"))
			if !strings.Contains(first, "read_file") && !strings.Contains(first, "ask_user_question") {
				t.Errorf("native spelling lost: %s", first)
			}
			execCLI(t, "import", "vibe")
			execCLI(t, "sync", "--only", "vibe")
			if got := readFile(t, filepath.Join(dir, ".vibe/skills/probe/SKILL.md")); got != first {
				t.Errorf("repeat import changed output: %s", got)
			}
			execCLI(t, "sync", "--only", "vibe", "--check")
			execCLI(t, "lint")
		})
	}
}

func TestImportVibe_NativeSkillToolsPreserveSharedCapabilities(t *testing.T) {
	for _, tc := range []struct{ field, value string }{
		{"allowed-tools", "Read"},
		{"can", "read"},
		{"allowed-tools", "Grep"},
	} {
		t.Run(tc.field+tc.value, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [vibe, claude]\nbuiltins: []\n")
			source := ".agnostic-ai/skills/probe/SKILL.md"
			writeFile(t, source, "---\nname: probe\ndescription: Inspect files.\n"+tc.field+": ["+tc.value+"]\nx-vibe:\n  allowed-tools: [read_file]\n---\nInspect files.\n")
			writeFile(t, ".vibe/skills/probe/SKILL.md", "---\nname: probe\ndescription: Inspect files.\nallowed-tools: [ask_user_question]\n---\nInspect files.\n")
			execCLI(t, "import", "vibe", "--overwrite")
			imported := readFile(t, filepath.Join(dir, source))
			mapping, _, ok := frontmatterNode([]byte(imported))
			if !ok {
				t.Fatalf("invalid imported spec: %s", imported)
			}
			shared := mappingValue(mapping, tc.field)
			if shared == nil {
				t.Errorf("shared %s lost: %s", tc.field, imported)
			} else {
				var got []string
				if err := shared.Decode(&got); err != nil || len(got) != 1 || got[0] != tc.value {
					t.Errorf("shared %s changed: %s", tc.field, imported)
				}
			}
			native := mappingValue(mappingValue(mapping, "x-vibe"), "allowed-tools")
			var tools []string
			if native == nil {
				t.Errorf("native tools lost: %s", imported)
			} else if err := native.Decode(&tools); err != nil || len(tools) != 1 || tools[0] != "ask_user_question" {
				t.Errorf("native tools not replaced: %s", imported)
			}
			execCLI(t, "sync")
			execCLI(t, "import", "vibe")
			execCLI(t, "sync", "--check")
			execCLI(t, "lint")
		})
	}
}

func TestImportVibe_UnchangedNativeSkillToolsPreserveSharedCapabilities(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [vibe]\nbuiltins: []\n")
	source := ".agnostic-ai/skills/probe/SKILL.md"
	writeFile(t, source, "---\nname: probe\ndescription: Inspect files.\nallowed-tools: [Read]\nx-vibe:\n  allowed-tools: [read_file]\n---\nInspect files.\n")
	writeFile(t, ".vibe/skills/probe/SKILL.md", "---\nname: probe\ndescription: Inspect files.\nallowed-tools: [read_file]\n---\nInspect files.\n")
	execCLI(t, "import", "vibe")
	imported := readFile(t, filepath.Join(dir, source))
	mapping, _, ok := frontmatterNode([]byte(imported))
	if !ok {
		t.Fatalf("invalid imported spec: %s", imported)
	}
	shared := mappingValue(mapping, "allowed-tools")
	var got []string
	if shared == nil {
		t.Errorf("shared tools lost: %s", imported)
	} else if err := shared.Decode(&got); err != nil || len(got) != 1 || got[0] != "Read" {
		t.Errorf("shared tools changed: %s", imported)
	}
	execCLI(t, "sync", "--only", "vibe")
	execCLI(t, "import", "vibe")
	execCLI(t, "sync", "--only", "vibe", "--check")
	execCLI(t, "lint")
}

func TestImportVibe_NativeSkillBodyEditPreservesSharedTools(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [vibe, claude]\nbuiltins: []\n")
	source := ".agnostic-ai/skills/probe/SKILL.md"
	writeFile(t, source, "---\nname: probe\ndescription: Inspect files.\nallowed-tools: [Read]\n---\nInspect files.\n")
	execCLI(t, "sync")
	nativePath := filepath.Join(dir, ".vibe/skills/probe/SKILL.md")
	native := readFile(t, nativePath)
	writeFile(t, nativePath, strings.Replace(native, "\nInspect files.\n", "\nInspect updated files.\n", 1))
	execCLI(t, "import", "vibe", "--overwrite")
	imported := readFile(t, filepath.Join(dir, source))
	mapping, _, ok := frontmatterNode([]byte(imported))
	if !ok {
		t.Fatalf("invalid imported spec: %s", imported)
	}
	shared := mappingValue(mapping, "allowed-tools")
	var got []string
	if shared == nil {
		t.Errorf("shared tools lost after native body edit: %s", imported)
	} else if err := shared.Decode(&got); err != nil || len(got) != 1 || got[0] != "Read" {
		t.Errorf("shared tools changed: %s", imported)
	}
	if !strings.Contains(imported, "Inspect updated files.") {
		t.Errorf("native edit lost: %s", imported)
	}
	execCLI(t, "sync")
	claude := readFile(t, filepath.Join(dir, ".claude/skills/probe/SKILL.md"))
	if !strings.Contains(claude, "allowed-tools:") || !strings.Contains(claude, "Read") {
		t.Errorf("shared restriction lost on Claude: %s", claude)
	}
	execCLI(t, "sync", "--check")
	execCLI(t, "lint")
}
