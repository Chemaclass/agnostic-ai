package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncUser_PiRejectsNonObjectMCPWithoutWrites(t *testing.T) {
	for _, document := range []string{"null", "[]", "\"text\"", "42", "true", piBOM + "null", piBOM + "[]"} {
		t.Run(document, func(t *testing.T) {
			home, source := userAgentTestHome(t)
			t.Setenv("PI_CODING_AGENT_DIR", "")
			native := filepath.Join(home, ".pi/agent/mcp.json")
			settings := filepath.Join(home, ".pi/agent/settings.json")
			beforeSettings := "{\"theme\":\"light\"}\n"
			writeFile(t, native, document)
			writeFile(t, settings, beforeSettings)
			writeFile(t, filepath.Join(source, "mcps/tools.yaml"), piOrderedSpec)
			writeFile(t, filepath.Join(source, "settings/pi.yaml"), "model: model-a\n")
			var syncErr error
			var panicValue any
			func() { defer func() { panicValue = recover() }(); _, _, syncErr = runUserAgentTest("--only", "pi") }()
			if panicValue != nil {
				t.Errorf("sync panicked: %v", panicValue)
			}
			if syncErr == nil || !strings.Contains(syncErr.Error(), native) {
				t.Errorf("expected native-path parse error, got %v", syncErr)
			}
			if got := readFile(t, native); got != document {
				t.Errorf("native MCP changed: %s", got)
			}
			if got := readFile(t, settings); got != beforeSettings {
				t.Errorf("pending settings write escaped: %s", got)
			}
			if _, err := os.Stat(filepath.Join(source, "state/global.json")); !os.IsNotExist(err) {
				t.Errorf("invalid native document wrote state: %v", err)
			}
		})
	}
}

func TestSyncUser_PiAcceptsMissingOrEmptyMCPDocument(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "missing"
		if existing {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			home, source := userAgentTestHome(t)
			t.Setenv("PI_CODING_AGENT_DIR", "")
			native := filepath.Join(home, ".pi/agent/mcp.json")
			if existing {
				writeFile(t, native, "")
			}
			writeFile(t, filepath.Join(source, "mcps/tools.yaml"), piOrderedSpec)
			if _, warn, err := runUserAgentTest("--only", "pi"); err != nil {
				t.Fatalf("sync: %v %s", err, warn)
			}
			requirePiOrder(t, readFile(t, native))
			if _, _, err := runUserAgentTest("--only", "pi", "--check"); err != nil {
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
