package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestQwen_ImportRefusesRetainedNativeCredentials(t *testing.T) {
	const literal = "synthetic-qwen-private-value"
	for _, tc := range []struct{ name, path, body string }{
		{"agent MCP environment", ".qwen/agents/reviewer.md", "---\nname: reviewer\ndescription: Review\nmcpServers:\n  docs:\n    command: server\n    env:\n      API_TOKEN: " + literal + "\n---\nReview changes.\n"},
		{"agent MCP argument", ".qwen/agents/reviewer.md", "---\nname: reviewer\ndescription: Review\nmcpServers:\n  docs:\n    command: server\n    args: [--password, " + literal + "]\n---\nReview changes.\n"},
		{"agent hook environment", ".qwen/agents/reviewer.md", "---\nname: reviewer\ndescription: Review\nhooks:\n  PreToolUse:\n    - hooks:\n        - type: command\n          command: echo check\n          env:\n            API_TOKEN: " + literal + "\n---\nReview changes.\n"},
		{"skill hook environment", ".qwen/skills/check/SKILL.md", "---\nname: check\ndescription: Check\nhooks:\n  PreToolUse:\n    - hooks:\n        - type: command\n          command: echo check\n          env:\n            API_TOKEN: " + literal + "\n---\nCheck changes.\n"},
		{"settings authentication", ".qwen/settings.json", `{"security":{"auth":{"apiKey":"` + literal + `"}}}`},
		{"nested settings password", ".qwen/settings.json", `{"gateway":{"credentials":{"password":"` + literal + `"}}}`},
		{"settings hook environment", ".qwen/settings.json", `{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"echo check","env":{"API_TOKEN":"` + literal + `"}}]}]}}`},
		{"unresolved agent MCP reference", ".qwen/agents/reviewer.md", "---\nname: reviewer\ndescription: Review\nmcpServers:\n  docs:\n    command: server\n    env:\n      API_TOKEN: ${TEAM_API_KEY}\n---\nReview changes.\n"},
		{"unresolved agent MCP argument", ".qwen/agents/reviewer.md", "---\nname: reviewer\ndescription: Review\nmcpServers:\n  docs:\n    command: server\n    args: [--password, '${TEAM_API_KEY}']\n---\nReview changes.\n"},
		{"unresolved attached agent MCP argument", ".qwen/agents/reviewer.md", "---\nname: reviewer\ndescription: Review\nmcpServers:\n  docs:\n    command: server\n    args: ['--password=${TEAM_API_KEY}']\n---\nReview changes.\n"},
		{"defaulted settings reference", ".qwen/settings.json", `{"security":{"auth":{"apiKey":"${TEAM_API_KEY:-` + literal + `}"}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "targets: [qwen]\n")
			writeFile(t, filepath.Join(dir, tc.path), tc.body)
			output, err := runCLI(t, "import", "qwen")
			if err == nil {
				t.Errorf("import accepted unsafe retained native field")
			}
			if strings.Contains(output, literal) || err != nil && strings.Contains(err.Error(), literal) {
				t.Error("credential appeared in error output")
			}
			err = filepath.WalkDir(filepath.Join(dir, ".agnostic-ai"), func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}
				data, readErr := os.ReadFile(path)
				if readErr != nil {
					return readErr
				}
				if strings.Contains(string(data), literal) {
					t.Errorf("credential retained in %s", path)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := readFile(t, tc.path); got != tc.body {
				t.Error("native input changed")
			}
		})
	}
}

func TestQwen_ImportNativeSettingsReferencesSurviveFreshOutput(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "targets: [qwen]\n")
	const native = `{"security":{"auth":{"selectedType":"openai","useExternal":false,"apiKey":"${TEAM_API_KEY}"}},"modelProviders":{"openai":[{"id":"team-model","envKey":"TEAM_API_KEY"}]},"memory":{"mem0":{"baseUrl":"https://example.test/memory","envKey":"MEM0_API_KEY"}},"custom":{"keep":true},"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"echo check","env":{"API_TOKEN":"$TEAM_API_KEY"}}]}]}}`
	writeFile(t, filepath.Join(dir, ".qwen/settings.json"), native)
	if out, err := runCLI(t, "import", "qwen"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if err := os.Remove(".qwen/settings.json"); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"sync", "-t", "qwen"}, {"sync", "--check", "-t", "qwen"}, {"lint"}} {
		if out, err := runCLI(t, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	got := readFile(t, ".qwen/settings.json")
	for _, want := range []string{"${TEAM_API_KEY}", "$TEAM_API_KEY", `"selectedType": "openai"`, `"useExternal": false`, `"envKey": "TEAM_API_KEY"`, `"envKey": "MEM0_API_KEY"`, `"keep": true`} {
		if !strings.Contains(got, want) {
			t.Errorf("native output lost %s", want)
		}
	}
}
