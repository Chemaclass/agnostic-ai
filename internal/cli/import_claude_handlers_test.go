package cli

import (
	"encoding/json"
	"path/filepath"
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
{"type":"prompt","prompt":"Allow read-only commands.","model":"example-model","once":true,"continueOnBlock":true},
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

func TestImportClaude_KeepsOnFailureBlockAsFailClosed(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	const native = `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[
{"type":"http","url":"https://example.test/check","onFailure":"block"},
{"type":"command","command":"guard.sh","onFailure":"block"}
]}]}}`
	writeFile(t, ".claude/settings.json", native)
	execCLI(t, "import", "claude")
	execCLI(t, "sync", "-t", "claude")
	want, got := importedClaudeHandlers(t, native), importedClaudeHandlers(t, readFile(t, ".claude/settings.json"))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round-trip handlers = %#v, want %#v", got, want)
	}
}

func TestImportClaude_KeepsOnFailureOnTheHandlerThatSetsIt(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	writeFile(t, ".claude/settings.json", `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[
{"type":"command","command":"guard.sh","onFailure":"block"},
{"type":"command","command":"audit-log.sh"}
]}]}}`)
	execCLI(t, "import", "claude")
	execCLI(t, "sync", "-t", "claude")

	var doc struct {
		Hooks map[string][]struct {
			Hooks []map[string]any `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(readFile(t, ".claude/settings.json")), &doc); err != nil {
		t.Fatal(err)
	}
	onFailure := map[string]any{}
	for _, group := range doc.Hooks["PreToolUse"] {
		for _, h := range group.Hooks {
			onFailure[h["command"].(string)] = h["onFailure"]
		}
	}
	if onFailure["guard.sh"] != "block" || onFailure["audit-log.sh"] != nil {
		t.Errorf("onFailure by command = %v, want only guard.sh to block", onFailure)
	}
}

func TestImportClaude_KeepsEachCommandHandlersOwnSettings(t *testing.T) {
	cases := map[string]string{
		"timeout": `{"type":"command","command":"a.sh","timeout":10},{"type":"command","command":"b.sh"}`,
		"shell":   `{"type":"command","command":"a.sh","shell":"powershell"},{"type":"command","command":"b.sh"}`,
		"async":   `{"type":"command","command":"a.sh","async":true},{"type":"command","command":"b.sh"}`,
		"same":    `{"type":"command","command":"a.sh","timeout":10},{"type":"command","command":"b.sh","timeout":10}`,
	}
	for name, handlers := range cases {
		t.Run(name, func(t *testing.T) {
			testutil.TempCwd(t)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
			native := `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[` + handlers + `]}]}}`
			writeFile(t, ".claude/settings.json", native)
			execCLI(t, "import", "claude")
			execCLI(t, "sync", "-t", "claude")
			want, got := claudePreToolUseGroups(t, native), claudePreToolUseGroups(t, readFile(t, ".claude/settings.json"))
			if !reflect.DeepEqual(got, want) {
				t.Errorf("PreToolUse groups = %#v, want %#v", got, want)
			}
		})
	}
}

func TestImportClaude_KeepsAgreeingCommandHandlersInOneSpec(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	writeFile(t, ".claude/settings.json", `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[
{"type":"command","command":"a.sh","timeout":10},
{"type":"command","command":"b.sh","timeout":10}
]}]}}`)
	execCLI(t, "import", "claude")
	specs, err := filepath.Glob(".agnostic-ai/hooks/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 {
		t.Errorf("hook specs = %v, want one spec holding both commands", specs)
	}
}

func TestImportClaude_KeepsOnFailureOnMcpToolAndPromptHandlersAsWritten(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	const native = `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[
{"type":"mcp_tool","server":"checks","tool":"verify","onFailure":"block"},
{"type":"prompt","prompt":"Allow read-only commands.","onFailure":"block"}
]}]}}`
	writeFile(t, ".claude/settings.json", native)
	execCLI(t, "import", "claude")
	execCLI(t, "sync", "-t", "claude")
	want, got := claudePreToolUseGroups(t, native), claudePreToolUseGroups(t, readFile(t, ".claude/settings.json"))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("PreToolUse groups = %#v, want %#v", got, want)
	}
}

func TestImportClaude_KeepsNonBlockOnFailureAsWritten(t *testing.T) {
	cases := map[string]string{
		"command":  `{"type":"command","command":"guard.sh","onFailure":"allow"}`,
		"http":     `{"type":"http","url":"https://example.test/check","onFailure":"allow"}`,
		"mcp_tool": `{"type":"mcp_tool","server":"checks","tool":"verify","onFailure":"allow"}`,
	}
	for name, handler := range cases {
		t.Run(name, func(t *testing.T) {
			testutil.TempCwd(t)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
			native := `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[` + handler + `]}]}}`
			writeFile(t, ".claude/settings.json", native)
			execCLI(t, "import", "claude")
			execCLI(t, "sync", "-t", "claude")
			want, got := claudePreToolUseGroups(t, native), claudePreToolUseGroups(t, readFile(t, ".claude/settings.json"))
			if !reflect.DeepEqual(got, want) {
				t.Errorf("PreToolUse groups = %#v, want %#v", got, want)
			}
		})
	}
}

func TestSyncClaude_McpToolSpecWithFailClosedFromAnOlderImportKeepsOneGroup(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	const native = `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[
{"type":"mcp_tool","server":"checks","tool":"verify","onFailure":"block"}
]}]}}`
	writeFile(t, ".claude/settings.json", native)
	writeFile(t, ".agnostic-ai/hooks/pretooluse-bash-checks-verify.yaml", "name: pretooluse-bash-checks-verify\ndescription: Calls checks/verify.\nevent: PreToolUse\nmatcher: Bash\ntarget: claude\ntype: mcp_tool\nserver: checks\ntool: verify\nfailClosed: true\n")
	execCLI(t, "sync", "-t", "claude")
	want, got := claudePreToolUseGroups(t, native), claudePreToolUseGroups(t, readFile(t, ".claude/settings.json"))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("PreToolUse groups = %#v, want %#v", got, want)
	}
}

func claudePreToolUseGroups(t *testing.T, data string) []any {
	t.Helper()
	var doc struct {
		Hooks map[string][]any `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(data), &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Hooks["PreToolUse"]
}

func TestImportClaude_IgnoredOnFailureBlockRoundTrips(t *testing.T) {
	cases := []struct {
		name   string
		native string
	}{
		{"stop", `{"hooks":{"Stop":[{"matcher":"","hooks":[{"type":"command","command":"guard.sh","onFailure":"block"}]}]}}`},
		{"async", `{"hooks":{"PostToolUse":[{"matcher":"","hooks":[{"type":"command","command":"guard.sh","async":true,"onFailure":"block"}]}]}}`},
		{"async rewake", `{"hooks":{"PostToolUse":[{"matcher":"","hooks":[{"type":"command","command":"guard.sh","asyncRewake":true,"onFailure":"block"}]}]}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			testutil.TempCwd(t)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
			writeFile(t, ".claude/settings.json", c.native)
			execCLI(t, "import", "claude")
			execCLI(t, "sync", "-t", "claude")
			var want, got map[string]any
			if err := json.Unmarshal([]byte(c.native), &want); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(readFile(t, ".claude/settings.json")), &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got["hooks"], want["hooks"]) {
				t.Errorf("round-trip hooks = %#v, want %#v", got["hooks"], want["hooks"])
			}
			execCLI(t, "sync", "--check", "-t", "claude")
			execCLI(t, "lint")
		})
	}
}
