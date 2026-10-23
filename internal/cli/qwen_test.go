package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestQwen_ImportSyncNativeProject(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [qwen]\n")
	files := map[string]string{
		"QWEN.md":                             "# Team instructions\n\nKeep changes small.\n",
		"backend/QWEN.md":                     "Use database transactions.\n",
		".qwen/agents/reviewer.md":            "---\nname: reviewer\ndescription: Review\ntools: [read_file, mystery_tool]\napprovalMode: plan\nmcpServers:\n  local:\n    command: local-mcp\n---\nReview carefully.\n",
		".qwen/skills/check/SKILL.md":         "---\nname: check\ndescription: Check\nuser-invocable: false\npaths: [backend/**]\n---\nRun scripts/check.sh.\n",
		".qwen/skills/check/scripts/check.sh": "#!/bin/sh\necho checked\n",
		".qwen/commands/git/commit.md":        "---\ndescription: Commit\n---\nCommit {{args}} !{git status} @{README.md}\n",
		".qwen/commands/deploy.md":            "---\ndescription: Ship\n---\nShip {{args}}\n",
		".qwen/settings.json":                 `{"model":{"name":"my-model","temperature":0.2},"permissions":{"deny":["Read(.env)"]},"custom":{"nested":{"keep":true}},"mcpServers":{"http":{"httpUrl":"https://example.test/mcp","trust":false,"oauth":{"enabled":false},"headers":{"Authorization":"Bearer ${TOKEN}"}},"sse":{"url":"https://example.test/events"},"local":{"command":"runner","args":["--quiet"]}},"hooks":{"enabled":true,"notifications":false,"PreToolUse":[{"matcher":"edit","sequential":true,"hooks":[{"type":"command","command":"sh .qwen/hooks/guard.sh","timeout":7,"async":false},{"type":"http","url":"https://example.test/guard","timeout":3},{"type":"prompt","prompt":"Check $ARGUMENTS","timeout":4}]}]}}`,
		".qwen/hooks/guard.sh":                "#!/bin/sh\necho guarded\n",
		".qwenignore":                         ".env\n",
	}
	for path, body := range files {
		writeFile(t, filepath.Join(dir, path), body)
	}
	for _, args := range [][]string{{"import", "qwen"}, {"sync"}, {"sync", "--check"}, {"lint"}} {
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	for path, body := range files {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(path, "settings.json") {
			var want, have map[string]any
			if err := json.Unmarshal([]byte(body), &want); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(got, &have); err != nil {
				t.Fatal(err)
			}
			groups := have["hooks"].(map[string]any)["PreToolUse"].([]any)
			delete(groups[0].(map[string]any)["hooks"].([]any)[0].(map[string]any), "env")
			if !sameSetting(want, have) {
				t.Errorf("settings changed: %s", got)
			}
		} else if path == "QWEN.md" || path == "backend/QWEN.md" {
			if !strings.Contains(string(got), strings.TrimSpace(body)) {
				t.Errorf("%s lost content: %s", path, got)
			}
		} else if strings.Contains(path, "scripts/") || strings.HasSuffix(path, ".sh") {
			if string(got) != body {
				t.Errorf("asset changed: %s", got)
			}
		} else {
			want, err := spec.ParseMarkdownBytes(spec.KindAgent, []byte(body))
			if err != nil {
				t.Fatal(err)
			}
			have, err := spec.ParseMarkdownBytes(spec.KindAgent, got)
			if err != nil {
				t.Fatal(err)
			}
			for key, value := range want.Meta {
				if !sameSetting(value, have.Meta[key]) {
					t.Errorf("%s lost %s: %#v want %#v", path, key, have.Meta[key], value)
				}
			}
			if strings.TrimSpace(header.Strip(have.Body)) != strings.TrimSpace(want.Body) {
				t.Errorf("%s body=%q", path, have.Body)
			}

		}
	}
}

func TestQwen_GlobalOwnershipAndRoot(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "created", true: "existing"}[existing], func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			native := filepath.Join(t.TempDir(), "qwen-home")
			t.Setenv("QWEN_HOME", native)
			path := filepath.Join(native, "settings.json")
			original := "{\n  \"model\": {\"temperature\": 0.2},\n  \"custom\": true\n}\n"
			if existing {
				mustWriteGlobalTest(t, path, original)
				if err := os.Chmod(path, 0600); err != nil {
					t.Fatal(err)
				}
			}
			specs := map[string]string{"settings/default.yaml": "model:\n  qwen: my-model\npermissions:\n  deny: [write, 'shell(git push *)']\n", "hooks/guard.yaml": "event: PreToolUse\ncommand: guard\nargs: ['two words']\ntimeout: 7\ntarget: qwen\n", "mcps/http.yaml": "name: http\ntype: http\nurl: https://example.test/mcp\n"}
			for rel, body := range specs {
				mustWriteGlobalTest(t, filepath.Join(source, rel), body)
			}
			if _, warnings, err := runGlobalAgentTest("--only", "qwen"); err != nil {
				t.Fatalf("sync: %v\n%s", err, warnings)
			}
			got := readGlobalJSON(t, path)
			handler := firstGlobalHandler(t, got, "PreToolUse")
			deny := got["permissions"].(map[string]any)["deny"].([]any)
			if !slices.Contains(deny, any("Bash(git push *)")) {
				t.Errorf("scoped permission lost: %#v", deny)
			}
			if handler["timeout"] != float64(7) || handler["command"] != "guard 'two words'" {
				t.Errorf("handler=%#v", handler)
			}
			if got["mcpServers"].(map[string]any)["http"].(map[string]any)["httpUrl"] != "https://example.test/mcp" {
				t.Errorf("servers=%#v", got)
			}
			if existing {
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm() != 0600 {
					t.Errorf("private mode=%v", info.Mode())
				}
			}
			if _, err := os.Stat(filepath.Join(home, ".qwen/settings.json")); !os.IsNotExist(err) {
				t.Errorf("default root was written: %v", err)
			}
			for rel := range specs {
				if err := os.Remove(filepath.Join(source, rel)); err != nil {
					t.Fatal(err)
				}
			}
			if _, warnings, err := runGlobalAgentTest("--only", "qwen"); err != nil {
				t.Fatalf("remove: %v\n%s", err, warnings)
			}
			if existing {
				if got := readGlobalTest(t, path); got != original {
					t.Errorf("restore=%q want=%q", got, original)
				}
			} else if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("created file survived: %v", err)
			}
			if _, _, err := runGlobalAgentTest("--only", "qwen", "--check"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestQwen_GlobalNativeTreesAndImport(t *testing.T) {
	_, source := globalAgentTestHome(t)
	native := filepath.Join(t.TempDir(), "qwen")
	t.Setenv("QWEN_HOME", native)
	for path, body := range map[string]string{"AGNOSTIC_AI.md": "# Agreements\nKeep changes small.\n", "agents/reviewer.md": "---\nname: reviewer\ndescription: Review\ncan: [read]\n---\nReview carefully.\n", "skills/check/SKILL.md": "---\nname: check\ndescription: Check\n---\nRun scripts/check.sh.\n", "skills/check/scripts/check.sh": "#!/bin/sh\necho checked\n"} {
		mustWriteGlobalTest(t, filepath.Join(source, path), body)
	}
	settings := filepath.Join(native, "settings.json")
	original := `{"custom":true,"model":{"name":"native-model","temperature":0.3},"mcpServers":{"http":{"httpUrl":"https://example.test/mcp"},"sse":{"url":"https://example.test/events"}}}`
	mustWriteGlobalTest(t, settings, original)
	if _, warnings, err := runImportGlobalTest("qwen"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "qwen"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, settings); got != original {
		t.Errorf("native settings changed: %s", got)
	}
	for path, want := range map[string]string{"QWEN.md": "Keep changes small", "agents/reviewer.md": "read_file", "skills/check/SKILL.md": "Run scripts/check.sh.", "skills/check/scripts/check.sh": "echo checked"} {
		got := readGlobalTest(t, filepath.Join(native, path))
		if !strings.Contains(got, want) {
			t.Errorf("%s missing %q: %s", path, want, got)
		}
	}
	if _, _, err := runGlobalAgentTest("--only", "qwen", "--check"); err != nil {
		t.Fatal(err)
	}
}

func TestQwen_PermissionScopesReachNativeSettings(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [qwen]\non-unsupported: error\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/settings/permissions.yaml"), "permissions:\n  deny: ['shell(git push *)', 'read(.env)']\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".qwen/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Bash(git push *)", "Read(.env)"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("missing %q: %s", want, data)
		}
	}
}
