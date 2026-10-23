package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncGlobal_PiRejectsNonObjectMCPWithoutWrites(t *testing.T) {
	for _, document := range []string{"null", "[]", "\"text\"", "42", "true", piBOM + "null", piBOM + "[]"} {
		t.Run(document, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			t.Setenv("PI_CODING_AGENT_DIR", "")
			native := filepath.Join(home, ".pi/agent/mcp.json")
			settings := filepath.Join(home, ".pi/agent/settings.json")
			beforeSettings := "{\"theme\":\"light\"}\n"
			mustWriteGlobalTest(t, native, document)
			mustWriteGlobalTest(t, settings, beforeSettings)
			mustWriteGlobalTest(t, filepath.Join(source, "mcps/tools.yaml"), piOrderedSpec)
			mustWriteGlobalTest(t, filepath.Join(source, "settings/pi.yaml"), "model: model-a\n")
			var syncErr error
			var panicValue any
			func() { defer func() { panicValue = recover() }(); _, _, syncErr = runGlobalAgentTest("--only", "pi") }()
			if panicValue != nil {
				t.Errorf("sync panicked: %v", panicValue)
			}
			if syncErr == nil || !strings.Contains(syncErr.Error(), native) {
				t.Errorf("expected native-path parse error, got %v", syncErr)
			}
			if got := readGlobalTest(t, native); got != document {
				t.Errorf("native MCP changed: %s", got)
			}
			if got := readGlobalTest(t, settings); got != beforeSettings {
				t.Errorf("pending settings write escaped: %s", got)
			}
			if _, err := os.Stat(filepath.Join(source, "state/global.json")); !os.IsNotExist(err) {
				t.Errorf("invalid native document wrote state: %v", err)
			}
		})
	}
}

func TestSyncGlobal_PiAcceptsMissingOrEmptyMCPDocument(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "missing"
		if existing {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			t.Setenv("PI_CODING_AGENT_DIR", "")
			native := filepath.Join(home, ".pi/agent/mcp.json")
			if existing {
				mustWriteGlobalTest(t, native, "")
			}
			mustWriteGlobalTest(t, filepath.Join(source, "mcps/tools.yaml"), piOrderedSpec)
			if _, warn, err := runGlobalAgentTest("--only", "pi"); err != nil {
				t.Fatalf("sync: %v %s", err, warn)
			}
			requirePiOrder(t, readGlobalTest(t, native))
			if _, _, err := runGlobalAgentTest("--only", "pi", "--check"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPiMCPMap_StateWithoutPiRemainsEmpty(t *testing.T) {
	for _, document := range []string{`{}`, `{"mcp":null}`, `{"mcp":{}}`, `{"mcp":{"pi":null}}`} {
		got, err := piMCPMap("global.json", []byte(document), "mcp", "pi")
		if err != nil || len(got) != 0 {
			t.Errorf("%s: got %v %v", document, got, err)
		}
	}
}
