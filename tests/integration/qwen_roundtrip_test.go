package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestQwenNativeProject_BuiltBinaryImportSyncCheck(t *testing.T) {
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "agnostic-ai")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = repo
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	dir := testutil.TempCwd(t)
	files := map[string]string{
		"agnostic-ai.yaml":                    "version: 1\ntargets: [qwen]\n",
		"QWEN.md":                             "# Project\n\nKeep changes small.\n",
		"backend/QWEN.md":                     "Use database transactions.\n",
		".qwen/agents/review.md":              "---\nname: review\ndescription: Review\ntools: [read_file, unknown_restriction]\n---\nReview the change.\n",
		".qwen/skills/check/SKILL.md":         "---\nname: check\ndescription: Check\nuser-invocable: false\n---\nRun scripts/check.sh.\n",
		".qwen/skills/check/scripts/check.sh": "#!/bin/sh\necho checked\n",
		".qwen/commands/git/commit.md":        "---\ndescription: Commit\n---\nCommit {{args}}\n",
		".qwen/settings.json":                 `{"custom":{"keep":true},"model":{"name":"custom-model","temperature":0.2},"mcpServers":{"http":{"httpUrl":"https://example.test/mcp"},"sse":{"url":"https://example.test/events"},"local":{"command":"runner"}},"hooks":{"enabled":true,"PreToolUse":[{"hooks":[{"type":"command","command":"echo checked","timeout":7}]}]}}`,
		".qwenignore":                         ".env\n",
	}
	for path, body := range files {
		out := filepath.Join(dir, path)
		must(t, os.MkdirAll(filepath.Dir(out), 0755))
		must(t, os.WriteFile(out, []byte(body), 0644))
	}
	for _, args := range [][]string{{"import", "qwen"}, {"sync"}, {"lint"}, {"sync", "--check"}} {
		command := exec.Command(binary, args...)
		command.Dir = dir
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, output)
		}
	}
	for path, want := range map[string]string{"QWEN.md": "Keep changes small.", "backend/QWEN.md": "Use database transactions.", ".qwen/agents/review.md": "unknown_restriction", ".qwen/skills/check/SKILL.md": "user-invocable: false", ".qwen/skills/check/scripts/check.sh": "echo checked", ".qwen/commands/git/commit.md": "Commit {{args}}", ".qwenignore": ".env"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), want) {
			t.Errorf("%s missing %q: %s", path, want, data)
		}
	}
	data, err := os.ReadFile(".qwen/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	var native map[string]any
	if err := json.Unmarshal(data, &native); err != nil {
		t.Fatal(err)
	}
	servers := native["mcpServers"].(map[string]any)
	if servers["http"].(map[string]any)["httpUrl"] != "https://example.test/mcp" || servers["sse"].(map[string]any)["url"] != "https://example.test/events" {
		t.Errorf("native transports: %s", data)
	}
	handler := native["hooks"].(map[string]any)["PreToolUse"].([]any)[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if handler["timeout"] != float64(7) || native["custom"] == nil {
		t.Errorf("native hook/settings: %s", data)
	}
}
