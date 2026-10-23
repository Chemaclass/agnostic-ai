package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const piOrderedSpec = "name: tools\ntargets: [pi]\ncommand: tools-mcp\nx-pi:\n  toolExposure:\n    write_*: hidden\n    '*': direct\n"
const piOrderedServer = `{"command":"tools-mcp","toolExposure":{"write_*":"hidden","*":"direct"}}`
const piReversedServer = `{"command":"tools-mcp","toolExposure":{"*":"direct","write_*":"hidden"}}`

func requirePiOrder(t *testing.T, data string) {
	t.Helper()
	first, second := strings.Index(data, "write_*"), strings.Index(data, "\"*\"")
	if second < 0 {
		second = strings.Index(data, "'*'")
	}
	if first < 0 || second < 0 || first > second {
		t.Errorf("wildcard precedence changed: %s", data)
	}
}

func TestPiExposure_ProjectImportToFreshOutput(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [pi]\n")
	native := filepath.Join(dir, ".pi/mcp.json")
	writeFile(t, native, `{"mcpServers":{"tools":`+piOrderedServer+`}}`)
	execCLI(t, "import", "pi")
	requirePiOrder(t, readFile(t, filepath.Join(dir, ".agnostic-ai/mcps/tools.yaml")))
	if err := os.Remove(native); err != nil {
		t.Fatal(err)
	}
	execCLI(t, "sync")
	requirePiOrder(t, readFile(t, native))
	execCLI(t, "sync", "--check")
	execCLI(t, "lint")
}

func TestPiExposure_GlobalStateAndFreshImportKeepPatternOrder(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("PI_CODING_AGENT_DIR", "")
	native := filepath.Join(home, ".pi/agent/mcp.json")
	mustWriteGlobalTest(t, native, `{"mcpServers":{"tools":`+piOrderedServer+`}}`)
	if _, warn, err := runImportGlobalTest("pi"); err != nil {
		t.Fatalf("import: %v %s", err, warn)
	}
	requirePiOrder(t, readGlobalTest(t, filepath.Join(source, "mcps/tools.yaml")))
	if err := os.Remove(native); err != nil {
		t.Fatal(err)
	}
	if _, warn, err := runGlobalAgentTest("--only", "pi"); err != nil {
		t.Fatalf("sync: %v %s", err, warn)
	}
	requirePiOrder(t, readGlobalTest(t, native))
	requirePiOrder(t, readGlobalTest(t, filepath.Join(source, "state/global.json")))
	if _, warn, err := runGlobalAgentTest("--only", "pi", "--check"); err != nil {
		t.Fatalf("check: %v %s", err, warn)
	}
}

func TestPiExposure_GlobalReorderedUserServerConflicts(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("PI_CODING_AGENT_DIR", "")
	native := filepath.Join(home, ".pi/agent/mcp.json")
	before := `{"mcpServers":{"tools":` + piReversedServer + `}}`
	mustWriteGlobalTest(t, native, before)
	mustWriteGlobalTest(t, filepath.Join(source, "mcps/tools.yaml"), piOrderedSpec)
	if _, _, err := runGlobalAgentTest("--only", "pi"); err == nil {
		t.Error("reversed wildcard order was silently adopted")
	}
	if got := readGlobalTest(t, native); got != before {
		t.Errorf("unapproved write: %s", got)
	}
}

func TestPiExposure_GlobalOwnedOrderUpdateRestoresUnrelatedBytes(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("PI_CODING_AGENT_DIR", "")
	native := filepath.Join(home, ".pi/agent/mcp.json")
	before := "{\n  // keep this comment\n  \"theme\": \"light\",\n  \"mcpServers\": {\n    \"mine\": {\"z\": 1, \"command\": \"mine\", \"a\": 2}\n  }\n}\n"
	mustWriteGlobalTest(t, native, before)
	if err := os.Chmod(native, 0600); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(source, "mcps/tools.yaml")
	mustWriteGlobalTest(t, specPath, piOrderedSpec)
	if _, warn, err := runGlobalAgentTest("--only", "pi"); err != nil {
		t.Fatalf("add: %v %s", err, warn)
	}
	requirePiOrder(t, readGlobalTest(t, native))
	mustWriteGlobalTest(t, specPath, strings.Replace(piOrderedSpec, "    write_*: hidden\n    '*': direct", "    '*': direct\n    write_*: hidden", 1))
	if _, warn, err := runGlobalAgentTest("--only", "pi"); err != nil {
		t.Fatalf("owned update: %v %s", err, warn)
	}
	if got := readGlobalTest(t, native); strings.Index(got, "\"*\"") > strings.Index(got, "write_*") {
		t.Errorf("owned order update lost: %s", got)
	}
	if _, warn, err := runGlobalAgentTest("--only", "pi", "--check"); err != nil {
		t.Fatalf("check after state reload: %v %s", err, warn)
	}
	if err := os.Remove(specPath); err != nil {
		t.Fatal(err)
	}
	if _, warn, err := runGlobalAgentTest("--only", "pi"); err != nil {
		t.Fatalf("remove: %v %s", err, warn)
	}
	if got := readGlobalTest(t, native); got != before {
		t.Errorf("unrelated bytes changed:\n%q\nwant %q", got, before)
	}
	info, err := os.Stat(native)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("private mode changed: %o", info.Mode().Perm())
	}
}

func TestPiExposure_GlobalUserReorderSurvivesRemovalAndRootMove(t *testing.T) {
	for _, operation := range []string{"remove", "move"} {
		t.Run(operation, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			t.Setenv("PI_CODING_AGENT_DIR", "")
			native := filepath.Join(home, ".pi/agent/mcp.json")
			specPath := filepath.Join(source, "mcps/tools.yaml")
			mustWriteGlobalTest(t, specPath, piOrderedSpec)
			if _, warn, err := runGlobalAgentTest("--only", "pi"); err != nil {
				t.Fatalf("initial: %v %s", err, warn)
			}
			edited := `{"mcpServers":{"tools":` + piReversedServer + `}}`
			mustWriteGlobalTest(t, native, edited)
			if _, _, err := runGlobalAgentTest("--only", "pi"); err == nil {
				t.Error("user reorder was silently overwritten or adopted")
			}
			if got := readGlobalTest(t, native); got != edited {
				t.Errorf("conflict changed user server: %s", got)
			}
			if operation == "remove" {
				if err := os.Remove(specPath); err != nil {
					t.Fatal(err)
				}
			} else {
				t.Setenv("PI_CODING_AGENT_DIR", filepath.Join(home, "moved-pi"))
			}
			if _, warn, err := runGlobalAgentTest("--only", "pi"); err != nil {
				t.Fatalf("%s: %v %s", operation, err, warn)
			}
			if got := readGlobalTest(t, native); got != edited {
				t.Errorf("%s deleted/reordered edited server: %s", operation, got)
			}
			if operation == "move" {
				requirePiOrder(t, readGlobalTest(t, filepath.Join(home, "moved-pi/mcp.json")))
			}
		})
	}
}

func TestPiExposure_GlobalEquivalentAdoptionAndSkippedTargetKeepRawOrder(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("PI_CODING_AGENT_DIR", "")
	native := filepath.Join(home, ".pi/agent/mcp.json")
	before := `{"mcpServers":{"tools":{"enabled":true,"toolExposure":{"write_*":"hidden","*":"direct"},"type":"stdio","command":"tools-mcp"}}}`
	mustWriteGlobalTest(t, native, before)
	mustWriteGlobalTest(t, filepath.Join(source, "mcps/tools.yaml"), piOrderedSpec)
	if _, warn, err := runGlobalAgentTest("--only", "pi"); err != nil {
		t.Fatalf("adopt: %v %s", err, warn)
	}
	if got := readGlobalTest(t, native); got != before {
		t.Errorf("adoption changed bytes: %s", got)
	}
	if _, warn, err := runGlobalAgentTest("--only", "codex"); err != nil {
		t.Fatalf("unselected target carry: %v %s", err, warn)
	}
	requirePiOrder(t, readGlobalTest(t, filepath.Join(source, "state/global.json")))
	if _, warn, err := runGlobalAgentTest("--only", "pi", "--check"); err != nil {
		t.Fatalf("reloaded state: %v %s", err, warn)
	}
}

func TestPiExposure_ProjectLedgerKeepsUserReorderWhenSpecRemoved(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [pi]\n")
	source := filepath.Join(dir, ".agnostic-ai/mcps/tools.yaml")
	writeFile(t, source, piOrderedSpec)
	execCLI(t, "sync")
	native := filepath.Join(dir, ".pi/mcp.json")
	edited := `{"mcpServers":{"tools":` + piReversedServer + `}}`
	writeFile(t, native, edited)
	command := NewRootCmd("test")
	command.SetArgs([]string{"sync", "--check"})
	if err := command.Execute(); err == nil {
		t.Error("check ignored wildcard order drift")
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	execCLI(t, "sync")
	if got := readFile(t, native); !strings.Contains(got, `"*":"direct","write_*":"hidden"`) {
		t.Errorf("project release removed user reorder: %s", got)
	}
}

func TestPiExposure_LocalLayerAppendsPatternsWithoutMovingBaseKeys(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [pi]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/mcps/tools.yaml"), piOrderedSpec)
	writeFile(t, filepath.Join(dir, ".agnostic-ai/local/mcps/tools.yaml"), "name: tools\nx-pi:\n  toolExposure:\n    write_*: hidden\n    read_*: deferred\n")
	execCLI(t, "sync")
	got := readFile(t, filepath.Join(dir, ".pi/mcp.json"))
	requirePiOrder(t, got)
	if strings.Index(got, "read_*") < strings.Index(got, "\"*\"") {
		t.Errorf("local key moved ahead of base catch-all: %s", got)
	}
	execCLI(t, "sync", "--check")
}

func TestPiExposure_OldRawStateKeepsActualOrderAndCannotDeleteUserReorder(t *testing.T) {
	dir := testutil.TempCwd(t)
	statePath := filepath.Join(dir, "global.json")
	mustWriteGlobalTest(t, statePath, `{"version":7,"files":[],"mcp":{"pi":{"mcpServers.tools":`+piOrderedServer+`}}}`)
	state, err := loadGlobalState(statePath)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	requirePiOrder(t, string(raw))
	native := filepath.Join(dir, "mcp.json")
	edited := `{"mcpServers":{"tools":` + piReversedServer + `}}`
	mustWriteGlobalTest(t, native, edited)
	merge, err := mergeGlobalMCP(native, globalTargets["pi"].mcp, []byte(edited), "pi", nil, state.MCP["pi"])
	if err != nil {
		t.Fatal(err)
	}
	if len(merge.changes) > 0 || merge.data != nil {
		t.Errorf("old state authorized removing reordered user server: %v %s", merge.changes, merge.data)
	}
}

func TestPiExposure_ExactKeysAreUnorderedAndWildcardOrderIsMeaningful(t *testing.T) {
	ordered, err := piMCPMap("a.json", []byte(`{"tools":{"command":"tools-mcp","toolExposure":{"write_safe":"deferred","write_*":"hidden","*":"direct","read?":"codemode"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	reordered, err := piMCPMap("b.json", []byte(`{"tools":{"command":"tools-mcp","toolExposure":{"read?":"codemode-deferred","write_*":"hidden","write_safe":"deferred","*":"direct"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !sameMCPServer("pi", ordered["tools"], reordered["tools"]) {
		t.Error("exact-key position or exposure alias changed meaning")
	}
	reversed, err := piMCPMap("c.json", []byte(`{"tools":{"command":"tools-mcp","toolExposure":{"write_safe":"deferred","*":"direct","write_*":"hidden","read?":"codemode"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if sameMCPServer("pi", ordered["tools"], reversed["tools"]) {
		t.Error("wildcard order treated as equivalent")
	}
}

func TestPiExposure_ProjectFirstSyncBacksUpDifferentPatternOrder(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [pi]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/mcps/tools.yaml"), piOrderedSpec)
	native := filepath.Join(dir, ".pi/mcp.json")
	before := `{"mcpServers":{"tools":` + piReversedServer + `}}`
	writeFile(t, native, before)
	execCLI(t, "sync")
	requirePiOrder(t, readFile(t, native))
	if got := readFile(t, native+".bak"); got != before {
		t.Errorf("order edit backup changed: %s", got)
	}
}
