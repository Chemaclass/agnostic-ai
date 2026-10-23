package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportPi_NativeRoundTrip(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [pi]\n")
	for path, body := range map[string]string{
		"AGENTS.md":                       "# Project\n\nUse small changes.\n",
		"backend/AGENTS.md":               "Use explicit errors.\n",
		".pi/skills/review/SKILL.md":      "---\nname: review\ndescription: Review the patch\ncompatibility: pi\nallowed-tools: Read\ndisable-model-invocation: true\n---\n\nReview the patch.\n",
		".pi/skills/review/reference.txt": "reference bytes\n",
		".pi/prompts/check.md":            "---\ndescription: Check a file\nargument-hint: <path>\n---\n\nCheck $ARGUMENTS.\n",
		".pi/mcp.json":                    "{\"mcpServers\":{\"local\":{\"command\":\"server\",\"cwd\":\"tools\",\"enabled\":false,\"exposure\":\"direct\"},\"remote\":{\"type\":\"http\",\"url\":\"https://example.com/mcp\",\"headers\":{\"Authorization\":\"${TOKEN}\"}}}}\n",
		".pi/settings.json":               "{\"defaultModel\":\"model-a\",\"defaultThinkingLevel\":\"high\",\"theme\":\"light\"}\n",
	} {
		writeFile(t, filepath.Join(dir, path), body)
	}
	execCLI(t, "import", "pi")
	mcp := readFile(t, filepath.Join(dir, ".agnostic-ai/mcps/local.yaml"))
	if !strings.Contains(mcp, "disabled: true") || !strings.Contains(mcp, "x-pi:") {
		t.Errorf("native MCP not normalized: %s", mcp)
	}
	settings := readFile(t, filepath.Join(dir, ".agnostic-ai/settings/pi.yaml"))
	if !strings.Contains(settings, "model: model-a") || !strings.Contains(settings, "effort: high") {
		t.Errorf("settings not promoted: %s", settings)
	}
	execCLI(t, "sync", "-t", "pi")
	for path, want := range map[string]string{"AGENTS.md": "Use small changes.", "backend/AGENTS.md": "Use explicit errors.", ".pi/skills/review/SKILL.md": "compatibility: pi", ".pi/skills/review/reference.txt": "reference bytes\n", ".pi/prompts/check.md": "Check $ARGUMENTS.", ".pi/settings.json": "\"theme\": \"light\""} {
		if got := readFile(t, filepath.Join(dir, path)); !strings.Contains(got, want) {
			t.Errorf("%s missing %q: %s", path, want, got)
		}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, ".pi/mcp.json"))), &m); err != nil {
		t.Fatal(err)
	}
	local := m["mcpServers"].(map[string]any)["local"].(map[string]any)
	if local["enabled"] != false || local["cwd"] != "tools" || local["exposure"] != "direct" {
		t.Errorf("server changed: %v", local)
	}
	first := readFile(t, filepath.Join(dir, ".pi/prompts/check.md"))
	execCLI(t, "sync", "--check", "-t", "pi")
	execCLI(t, "lint")
	execCLI(t, "sync", "-t", "pi")
	if got := readFile(t, filepath.Join(dir, ".pi/prompts/check.md")); got != first {
		t.Errorf("prompt drift:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".pi/trust.json")); !os.IsNotExist(err) {
		t.Errorf("trust file: %v", err)
	}
}

func TestSyncGlobal_PiOwnsNativeKeysAndRestoresUserBytes(t *testing.T) {
	home, source := globalAgentTestHome(t)
	root := filepath.Join(home, "custom-pi")
	t.Setenv("PI_CODING_AGENT_DIR", root)
	settings := filepath.Join(root, "settings.json")
	mcp := filepath.Join(root, "mcp.json")
	beforeSettings := "{\n\t\"theme\": \"light\"\n}\n"
	beforeMCP := "{\n\t\"mcpServers\": {\n\t\t\"mine\": {\"command\": \"mine\"}\n\t}\n}\n"
	mustWriteGlobalTest(t, settings, beforeSettings)
	mustWriteGlobalTest(t, mcp, beforeMCP)
	settingsSpec := filepath.Join(source, "settings/pi.yaml")
	mcpSpec := filepath.Join(source, "mcps/docs.yaml")
	mustWriteGlobalTest(t, settingsSpec, "model: model-a\neffort: high\n")
	mustWriteGlobalTest(t, mcpSpec, "name: docs\ncommand: docs-mcp\ndisabled: true\n")
	if _, warn, err := runGlobalAgentTest("--only", "pi"); err != nil {
		t.Fatalf("sync: %v %s", err, warn)
	}
	if got := readGlobalTest(t, settings); !strings.Contains(got, "\"defaultModel\": \"model-a\"") || !strings.Contains(got, "\t\"theme\": \"light\"") {
		t.Errorf("settings: %s", got)
	}
	if got := readGlobalTest(t, mcp); !strings.Contains(got, "\"enabled\": false") || strings.Contains(got, "disabled") {
		t.Errorf("mcp: %s", got)
	}
	if _, _, err := runGlobalAgentTest("--only", "pi", "--check"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(settingsSpec); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(mcpSpec); err != nil {
		t.Fatal(err)
	}
	if _, warn, err := runGlobalAgentTest("--only", "pi"); err != nil {
		t.Fatalf("remove: %v %s", err, warn)
	}
	if got := readGlobalTest(t, settings); got != beforeSettings {
		t.Errorf("settings bytes changed: %q", got)
	}
	if got := readGlobalTest(t, mcp); got != beforeMCP {
		t.Errorf("mcp bytes changed: %q", got)
	}
}

func TestImportGlobal_PiAdoptsNativeFilesWithoutChangingBytes(t *testing.T) {
	home, source := globalAgentTestHome(t)
	root := filepath.Join(home, ".pi/agent")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	native := map[string]string{
		"settings.json": "{\n\t\"theme\":\"light\",\n\t\"defaultModel\":\"model-a\",\n\t\"defaultThinkingLevel\":\"high\"\n}\n",
		"mcp.json":      "{\n\t\"mcpServers\":{\n\t\t\"docs\":{\"command\":\"docs-mcp\",\"enabled\":false,\"timeout\":40,\"exposure\":\"direct\"},\n\t\t\"web\":{\"url\":\"https://example.com/mcp\",\"type\":\"streamable-http\",\"headers\":{\"Authorization\":\"${TOKEN}\"},\"enabled\":true}\n\t}\n}\n",
	}
	for path, body := range native {
		mustWriteGlobalTest(t, filepath.Join(root, path), body)
	}
	if _, warnings, err := runImportGlobalTest("pi"); err != nil {
		t.Fatalf("import: %v %s", err, warnings)
	}
	if got := readGlobalTest(t, filepath.Join(source, "mcps/docs.yaml")); !strings.Contains(got, "disabled: true") {
		t.Errorf("missing inversion: %s", got)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "pi"); err != nil {
		t.Fatalf("sync: %v %s", err, warnings)
	}
	for path, want := range native {
		if got := readGlobalTest(t, filepath.Join(root, path)); got != want {
			t.Errorf("%s bytes changed:\n%s", path, got)
		}
	}
	if _, _, err := runGlobalAgentTest("--only", "pi", "--check"); err != nil {
		t.Fatal(err)
	}
}

func TestSyncGlobal_PiCreatedFilesAreReleasedAndPrivateFilesStayPrivate(t *testing.T) {
	home, source := globalAgentTestHome(t)
	root := filepath.Join(home, "pi-agent")
	t.Setenv("PI_CODING_AGENT_DIR", root)
	specs := map[string]string{
		"rules/style.md":         "---\nname: style\n---\n\nUse plain names.\n",
		"skills/review/SKILL.md": "---\nname: review\ndescription: Review patches\ndisable-model-invocation: true\n---\n\nReview.\n",
		"mcps/docs.yaml":         "name: docs\ncommand: docs-mcp\n",
		"settings/pi.yaml":       "model: model-a\neffort: high\nx-pi:\n  defaultProjectTrust: allow\n",
	}
	for path, body := range specs {
		mustWriteGlobalTest(t, filepath.Join(source, path), body)
	}
	private := filepath.Join(root, "settings.json")
	before := "{\"theme\":\"light\"}\n"
	mustWriteGlobalTest(t, private, before)
	if err := os.Chmod(private, 0600); err != nil {
		t.Fatal(err)
	}
	if _, warn, err := runGlobalAgentTest("--only", "pi"); err != nil {
		t.Fatalf("sync: %v %s", err, warn)
	}
	for path, want := range map[string]string{"AGENTS.md": "Use plain names.", "skills/review/SKILL.md": "disable-model-invocation: true", "mcp.json": "docs-mcp"} {
		if got := readGlobalTest(t, filepath.Join(root, path)); !strings.Contains(got, want) {
			t.Errorf("%s: %s", path, got)
		}
	}
	if got := readGlobalTest(t, private); strings.Contains(got, "defaultProjectTrust") {
		t.Errorf("sync changed trust: %s", got)
	}
	info, err := os.Stat(private)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Errorf("private mode: %o", info.Mode().Perm())
	}
	for path := range specs {
		if err := os.Remove(filepath.Join(source, path)); err != nil {
			t.Fatal(err)
		}
	}
	if _, warn, err := runGlobalAgentTest("--only", "pi"); err != nil {
		t.Fatalf("release: %v %s", err, warn)
	}
	if got := readGlobalTest(t, private); got != before {
		t.Errorf("user settings bytes: %q", got)
	}
	for _, path := range []string{"AGENTS.md", "skills/review/SKILL.md", "mcp.json"} {
		if _, err := os.Stat(filepath.Join(root, path)); !os.IsNotExist(err) {
			t.Errorf("created %s not removed: %v", path, err)
		}
	}
}

func TestSyncPi_SharedInstructionsKeepBothTargetsAndScopes(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [pi, codex]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/AGNOSTIC_AI.md"), "Project owned instructions.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/rules/style.md"), "---\nname: style\n---\n\nUse plain names.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/rules/backend/errors.md"), "---\nname: errors\nscope: backend\n---\n\nReturn contextual errors.\n")
	execCLI(t, "sync")
	root := readFile(t, filepath.Join(dir, "AGENTS.md"))
	if strings.Count(root, "Use plain names.") != 1 || !strings.Contains(root, "Project owned instructions.") {
		t.Errorf("shared instructions: %s", root)
	}
	if got := readFile(t, filepath.Join(dir, "backend/AGENTS.md")); strings.Count(got, "Return contextual errors.") != 1 {
		t.Errorf("scoped instructions: %s", got)
	}
	execCLI(t, "sync", "--check")
}

func TestImportPi_ReadsConfiguredNativeDirectories(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [pi]\noutputs:\n  pi:\n    dir: .custom-pi\n    conf-file: native-settings.json\n")
	writeFile(t, filepath.Join(dir, ".custom-pi/prompts/check.md"), "---\ndescription: Check changes\n---\n\nCheck $1.\n")
	writeFile(t, filepath.Join(dir, "native-settings.json"), "{\"defaultModel\":\"model-a\"}\n")
	execCLI(t, "import", "pi")
	if got := readFile(t, filepath.Join(dir, ".agnostic-ai/commands/check.md")); !strings.Contains(got, "Check $1.") {
		t.Errorf("prompt not imported: %s", got)
	}
	if got := readFile(t, filepath.Join(dir, ".agnostic-ai/settings/pi.yaml")); !strings.Contains(got, "model: model-a") {
		t.Errorf("settings not imported: %s", got)
	}
	execCLI(t, "sync", "-t", "pi")
	execCLI(t, "sync", "--check", "-t", "pi")
}

func TestSyncGlobal_PiInvalidMCPPreservesManagedFiles(t *testing.T) {
	home, source := globalAgentTestHome(t)
	root := filepath.Join(home, ".pi/agent")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	mustWriteGlobalTest(t, filepath.Join(source, "mcps/docs.yaml"), "name: docs\ncommand: docs-mcp\n")
	settingsSpec := filepath.Join(source, "settings/pi.yaml")
	mustWriteGlobalTest(t, settingsSpec, "model: model-a\n")
	if _, warn, err := runGlobalAgentTest("--only", "pi"); err != nil {
		t.Fatalf("initial sync: %v %s", err, warn)
	}
	before := map[string]string{}
	for _, path := range []string{filepath.Join(root, "mcp.json"), filepath.Join(root, "settings.json"), filepath.Join(source, "state/global.json")} {
		before[path] = readGlobalTest(t, path)
	}
	mustWriteGlobalTest(t, filepath.Join(source, "mcps/bad.yaml"), "name: bad\ntype: sse\nurl: https://example.com/sse\n")
	mustWriteGlobalTest(t, settingsSpec, "model: changed\n")
	if _, _, err := runGlobalAgentTest("--only", "pi"); err == nil || !strings.Contains(err.Error(), "SSE") {
		t.Errorf("expected SSE rendering error, got %v", err)
	}
	for path, want := range before {
		if got := readGlobalTest(t, path); got != want {
			t.Errorf("invalid server changed %s:\ngot %s\nwant %s", path, got, want)
		}
	}
}

func TestImportPi_ControlOnlyOverridesRoundTripAndLint(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [pi, codex]\n")
	writeFile(t, filepath.Join(dir, ".pi/mcp.json"), "{\"mcpServers\":{\"disabled\":{\"enabled\":false},\"hidden\":{\"exposure\":\"hidden\"},\"limited\":{\"toolExposure\":{\"read_*\":\"direct\",\"delete_*\":\"hidden\"}}}}\n")
	execCLI(t, "import", "pi")
	for _, name := range []string{"disabled", "hidden", "limited"} {
		got := readFile(t, filepath.Join(dir, ".agnostic-ai/mcps", name+".yaml"))
		if !strings.Contains(got, "pi") {
			t.Errorf("unscoped override: %s", got)
		}
	}
	execCLI(t, "sync")
	execCLI(t, "sync", "--check")
	execCLI(t, "lint")
	got := readFile(t, filepath.Join(dir, ".pi/mcp.json"))
	for _, want := range []string{"\"enabled\": false", "\"exposure\": \"hidden\"", "\"read_*\": \"direct\""} {
		if !strings.Contains(got, want) {
			t.Errorf("missing control %s: %s", want, got)
		}
	}
}
