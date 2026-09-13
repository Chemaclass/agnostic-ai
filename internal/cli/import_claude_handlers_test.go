package cli

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportClaude_PreservesMixedNativeHookHandlers(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	const native = `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[
{"type":"http","url":"https://example.test/check","headers":{"Authorization":"Bearer $TOKEN"},"allowedEnvVars":["TOKEN"],"timeout":20,"statusMessage":"Checking request","if":"Bash(git *)"},
{"type":"mcp_tool","server":"checks","tool":"verify","input":{"path":"${tool_input.file_path}"}},
{"type":"prompt","prompt":"Allow read-only commands.","model":"example-model","once":true},
{"type":"command","command":"echo","args":["checked"],"async":true}
]}]}}`
	writeFile(t, ".claude/settings.json", native)
	execCLI(t, "import", "claude")
	execCLI(t, "sync", "-t", "claude")
	want, got := importedClaudeHandlers(t, native), importedClaudeHandlers(t, readFile(t, ".claude/settings.json"))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round-trip handlers = %#v, want %#v", got, want)
	}
}

func importedClaudeHandlers(t *testing.T, data string) map[string]map[string]any {
	t.Helper()
	var doc struct {
		Hooks map[string][]struct {
			Hooks []map[string]any `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(data), &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]any{}
	for _, group := range doc.Hooks["PreToolUse"] {
		for _, handler := range group.Hooks {
			kind, _ := handler["type"].(string)
			out[kind] = handler
		}
	}
	return out
}
