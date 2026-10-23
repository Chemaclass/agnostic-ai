package cli

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestCodebuddy_SourceAuditMCPJSONCAndInferenceRecreateNativeFields(t *testing.T) {
	for _, tc := range []struct{ name, server, kind string }{
		{"stdio", `"command":"docs-server","cwd":"backend","description":"Native description"`, "stdio"},
		{"http", `"url":"https://example.com/mcp","description":"Native description"`, "http"},
		{"empty-type", `"type":"","command":"docs-server","description":"Native description"`, "stdio"},
		{"cwd-reference", `"command":"docs-server","cwd":"${PROJECT_ROOT}/backend","description":"Native description"`, "stdio"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.TempCwd(t)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codebuddy]\n")
			writeFile(t, "mcp.json", "{\n // native JSONC\n \"mcpServers\": {\"docs\": {"+tc.server+",},},\n}\n")
			if out, err := runRoot(t, "import", "codebuddy"); err != nil {
				t.Fatalf("import: %v\n%s", err, out)
			}
			if err := os.Rename("mcp.json", filepath.Join(t.TempDir(), "original")); err != nil {
				t.Fatal(err)
			}
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codebuddy]\noutputs:\n  codebuddy:\n    root-mcp-file: .mcp.json\n")
			mustSync(t)
			servers, err := readJSONMapAt(".mcp.json", "mcpServers")
			if err != nil {
				t.Fatal(err)
			}
			got, _ := servers["docs"].(map[string]any)
			if got["type"] != tc.kind || got["description"] != "Native description" || tc.name == "stdio" && got["cwd"] != "backend" {
				t.Fatalf("native fields lost: %v", got)
			}
			if tc.name == "cwd-reference" && got["cwd"] != "${PROJECT_ROOT}/backend" {
				t.Fatalf("native cwd reference changed: %v", got)
			}
			if out, err := runRoot(t, "sync", "--check"); err != nil {
				t.Fatalf("check: %v\n%s", err, out)
			}
			if out, err := runRoot(t, "lint"); err != nil {
				t.Fatalf("lint: %v\n%s", err, out)
			}
		})
	}
}

func TestCodebuddy_SourceAuditManualOnlyAgentSurvivesFreshImport(t *testing.T) {
	for _, flag := range []string{"true", "false"} {
		t.Run(flag, func(t *testing.T) {
			testutil.TempCwd(t)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codebuddy]\n")
			writeFile(t, ".codebuddy/agents/reviewer.md", "---\nname: reviewer\ndescription: Review\ntools: Read\ndisable-model-invocation: "+flag+"\n---\nReview.\n")
			if out, err := runRoot(t, "import", "codebuddy"); err != nil {
				t.Fatalf("import: %v\n%s", err, out)
			}
			if err := os.Rename(".codebuddy", filepath.Join(t.TempDir(), "original")); err != nil {
				t.Fatal(err)
			}
			mustSync(t)
			data, err := os.ReadFile(".codebuddy/agents/reviewer.md")
			if err != nil {
				t.Fatal(err)
			}
			meta, _ := splitMdcFrontmatter(data)
			if got, ok := meta["disable-model-invocation"]; !ok || got != (flag == "true") {
				t.Fatalf("manual-only flag lost: %s", data)
			}
			if out, err := runRoot(t, "sync", "--check"); err != nil {
				t.Fatalf("check: %v\n%s", err, out)
			}
		})
	}
}

func TestCodebuddy_SourceAuditSkillPatternsRemainRestrictive(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codebuddy]\n")
	writeFile(t, ".agnostic-ai/skills/review/SKILL.md", "---\nname: review\ndescription: Review\nallowed-tools: [Read, 'Bash(git:*)', 'Edit(src/**/*.go)']\n---\nReview.\n")
	mustSync(t)
	data, err := os.ReadFile(".codebuddy/skills/review/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	meta, _ := splitMdcFrontmatter(data)
	if meta["allowed-tools"] != "Read, Bash(git:*), Edit(src/**/*.go)" {
		t.Fatalf("scope changed: %s", data)
	}
}

func TestCodebuddy_SourceAuditSkillPatternsRejectInvalidMappings(t *testing.T) {
	for _, tool := range []string{"Bash()", "Bash(git:*", "Delete(src/**)", "mcp__github__get*"} {
		t.Run(tool, func(t *testing.T) {
			testutil.TempCwd(t)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codebuddy]\n")
			writeFile(t, ".agnostic-ai/skills/review/SKILL.md", "---\nname: review\ndescription: Review\nallowed-tools: ['"+tool+"']\n---\nReview.\n")
			if _, err := runRoot(t, "sync"); err == nil {
				t.Fatal("invalid restriction accepted")
			}
			if _, err := os.Stat(".codebuddy/skills/review/SKILL.md"); !os.IsNotExist(err) {
				t.Fatal("invalid restriction emitted")
			}
		})
	}
}

func TestCodebuddy_SourceAuditThirdLegacyJSONCOwnershipRestoresBytes(t *testing.T) {
	home, source := globalAgentTestHome(t)
	native := filepath.Join(home, ".codebuddy.json")
	original := "{\n // account and project state\n \"nativeControl\": true,\n \"projects\": {\"/project\": {\"native\":true}},\n \"mcpServers\": {\"mine\": {\"command\":\"mine-server\",},},\n}\n"
	mustWriteGlobalTest(t, native, original)
	if err := os.Chmod(native, 0600); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runImportGlobalTest("codebuddy"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, filepath.Join(source, "mcps/mine.yaml")); !strings.Contains(got, "mine-server") {
		t.Fatal("third legacy server was not imported")
	}
	added := filepath.Join(source, "mcps/docs.yaml")
	mustWriteGlobalTest(t, added, globalDocsMCP)
	if _, warnings, err := runGlobalAgentTest("--only", "codebuddy"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, native); !strings.Contains(got, "docs-mcp") || !strings.Contains(got, "account and project state") {
		t.Fatal("server or comments lost")
	}
	for _, path := range []string{".codebuddy/.mcp.json", ".codebuddy/mcp.json"} {
		if _, err := os.Stat(filepath.Join(home, path)); !os.IsNotExist(err) {
			t.Fatalf("legacy servers shadowed by %s", path)
		}
	}
	if _, warnings, err := runGlobalAgentTest("--only", "codebuddy", "--check"); err != nil {
		t.Fatalf("check: %v\n%s", err, warnings)
	}
	if err := os.Remove(added); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "codebuddy"); err != nil {
		t.Fatalf("remove: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, native); got != original {
		t.Fatalf("original bytes changed:\n%q\nwant:\n%q", got, original)
	}
	info, err := os.Stat(native)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Errorf("private mode changed: %v", info.Mode())
	}
}

func TestCodebuddy_SourceAuditMCPFirstExistingAndStaleProtection(t *testing.T) {
	for _, chosen := range []string{".codebuddy/.mcp.json", ".codebuddy/mcp.json", ".codebuddy.json"} {
		t.Run(chosen, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			paths := []string{".codebuddy/.mcp.json", ".codebuddy/mcp.json", ".codebuddy.json"}
			found := false
			for _, path := range paths {
				found = found || path == chosen
				if found {
					mustWriteGlobalTest(t, filepath.Join(home, path), "{\n // native\n \"mcpServers\": {}\n}\n")
				}
			}
			native := filepath.Join(home, chosen)
			if got := globalTargets["codebuddy"].mcpPath(home); got != native {
				t.Fatalf("selected %s, want %s", got, native)
			}
			bundle := spec.Bundle{MCPs: []spec.Entry{{Kind: spec.KindMCP, Name: "docs", Path: "mcps/docs.yaml", Meta: map[string]any{"command": "docs-mcp"}}}}
			writes, _, err := buildGlobalWrites(home, source, []string{"codebuddy"}, nil, bundle, globalState{Version: globalStateVersion}, nil, io.Discard, "")
			if err != nil {
				t.Fatal(err)
			}
			changed := "{\"mcpServers\":{},\"changedAfterRead\":true}\n"
			mustWriteGlobalTest(t, native, changed)
			if _, err := applyGlobalChanges(writes, nil, false, nil); err == nil || !strings.Contains(err.Error(), "changed while sync ran") {
				t.Fatalf("stale write accepted: %v", err)
			}
			if got := readGlobalTest(t, native); got != changed {
				t.Fatal("stale user edit overwritten")
			}
		})
	}
}

func TestCodebuddy_SourceAuditRejectsInvalidNativeMCPWithoutWritingSpecs(t *testing.T) {
	for _, tc := range []struct{ name, server, field string }{
		{"type-value", `"type":"unsupported-type-fixture","command":"docs-server"`, "type"},
		{"type-shape", `"type":42,"command":"docs-server"`, "type"},
		{"cwd-shape", `"command":"docs-server","cwd":42`, "cwd"},
		{"remote-cwd", `"url":"https://example.com/mcp","cwd":"private-cwd-fixture"`, "cwd"},
		{"disabled", `"command":"docs-server","disabled":true`, "disabled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.TempCwd(t)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codebuddy]\n")
			original := "{\"mcpServers\":{\"docs\":{" + tc.server + "}}}\n"
			writeFile(t, ".mcp.json", original)
			_, err := runRoot(t, "import", "codebuddy")
			if err == nil || !strings.Contains(err.Error(), ".mcp.json") || !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("invalid native field accepted: %v", err)
			}
			if strings.Contains(err.Error(), "fixture") {
				t.Fatalf("native value leaked: %v", err)
			}
			if _, err := os.Stat(".agnostic-ai/mcps/docs.yaml"); !os.IsNotExist(err) {
				t.Fatal("invalid native server imported")
			}
			data, err := os.ReadFile(".mcp.json")
			if err != nil || string(data) != original {
				t.Fatalf("native input changed: %v", err)
			}
		})
	}
}

func TestCodebuddy_SourceAuditGlobalNativeFields(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agents/reviewer.md"), "---\nname: reviewer\ndescription: Review\ncan: [read]\nx-codebuddy:\n  disable-model-invocation: true\n---\nReview.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "mcps/docs.yaml"), "command: docs-server\ncwd: backend\ndescription: Native description\n")
	if _, warnings, err := runGlobalAgentTest("--only", "codebuddy"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, filepath.Join(home, ".codebuddy/agents/reviewer.md")); !strings.Contains(got, "disable-model-invocation: true") {
		t.Fatal("manual-only flag lost from global agent")
	}
	servers, err := readJSONMapAt(filepath.Join(home, ".codebuddy/.mcp.json"), "mcpServers")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := servers["docs"].(map[string]any)
	if got["cwd"] != "backend" || got["description"] != "Native description" || got["type"] != "stdio" {
		t.Fatalf("global native fields lost: %v", got)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "codebuddy", "--check"); err != nil {
		t.Fatalf("check: %v\n%s", err, warnings)
	}
}
