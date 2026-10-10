package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestCredentialPreview_DryRunAndRenderHideLiteralMCPValues(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\noutputs:\n  claude:\n    mcp-file: native-config.txt\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "remote.yaml"), "name: remote\ntype: http\nurl: https://example.test/mcp\nheaders:\n  Authorization: !literal 'Bearer PREVIEW_SECRET_2021_A'\n  X-Opaque: !literal PREVIEW_SECRET_2021_B\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "local.yaml"), "name: local\ncommand: server\nenv:\n  OPAQUE: !literal PREVIEW_SECRET_2021_ENV\n")
	before := snapshotProject(t, dir)
	for _, args := range [][]string{{"sync", "--dry-run"}, {"render", ".agnostic-ai/mcps/remote.yaml"}, {"sync", "--dry-run", "--json"}} {
		printed := captureStdout(t, func() {
			out, err := runCLI(t, args...)
			if err != nil {
				t.Fatalf("preview command failed: %v", err)
			}
			if strings.Contains(out, "PREVIEW_SECRET_2021_") {
				t.Error("command preview leaked a credential")
			}
		})
		if strings.Contains(printed, "PREVIEW_SECRET_2021_") {
			t.Error("stdout preview leaked a credential")
		}
	}
	assertProjectUnchanged(t, before, snapshotProject(t, dir))
	captureStdout(t, func() {
		if _, err := runCLI(t, "sync"); err != nil {
			t.Fatal(err)
		}
	})
	data, err := os.ReadFile(filepath.Join(dir, "native-config.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "PREVIEW_SECRET_2021_A") || !strings.Contains(string(data), "PREVIEW_SECRET_2021_B") {
		t.Error("actual sync altered the accepted literal credential")
	}
	if adapters.ContentSum(string(data)) != fmt.Sprintf("%x", sha256.Sum256(data)) {
		t.Error("raw content fingerprint changed")
	}
	if readStateFile(dir).OutputSums["native-config.txt"] != adapters.ContentSum(string(data)) {
		t.Error("ownership ledger no longer fingerprints the actual output")
	}
	if _, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync check after write: %v", err)
	}
}

func TestCredentialPreview_GlobalDiffHidesSecretOnlyChange(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "custom.txt")
	old := []byte("{\"env\":{\"OPAQUE\":\"PREVIEW_SECRET_2021_OLD\"}}\n")
	next := []byte("{\"env\":{\"OPAQUE\":\"PREVIEW_SECRET_2021_NEW\"}}\n")
	writeFile(t, p, string(old))
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	err := checkGlobalWrites(cmd, []globalWrite{{path: p, data: next}}, nil, "", globalState{}, true)
	if err == nil {
		t.Error("secret-only drift must fail")
	}
	if strings.Contains(out.String(), "PREVIEW_SECRET_2021_") || !strings.Contains(out.String(), "sensitive") {
		t.Error("global diff leaked credentials or lost the change signal")
	}
	data, err := os.ReadFile(p)
	if err != nil || !bytes.Equal(data, old) {
		t.Error("global check changed disk content")
	}
}

func TestCredentialPreview_DiffsHideBothSidesAndKeepChangeSignal(t *testing.T) {
	old := "{\"env\":{\"OPAQUE\":\"PREVIEW_SECRET_2021_OLD\"}}\n"
	next := "{\"env\":{\"OPAQUE\":\"PREVIEW_SECRET_2021_NEW\"}}\n"
	diff := unifiedDiff("custom.txt", old, next, diffBodyMax)
	if strings.Contains(diff, "PREVIEW_SECRET_2021_") {
		t.Error("sync diff leaked an old or new credential")
	}
	if !strings.Contains(diff, "sensitive") {
		t.Error("secret-only drift lost its change signal")
	}
	for _, existed := range []bool{false, true} {
		diff := importPreviewDiff(importPreviewEntry{path: "custom.txt", before: []byte(old), after: []byte(next), existed: existed})
		if strings.Contains(diff, "PREVIEW_SECRET_2021_") {
			t.Error("import preview leaked a credential")
		}
	}
}

func TestCredentialPreview_ReviewAcceptedArgumentsAndContinueReference(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, continue]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "local.yaml"), "name: local\ncommand: server\nargs:\n  - --token\n  - 'PREVIEW_SECRET_2021_ARGS:${SUFFIX}'\n  - '--header=Cookie: PREVIEW_SECRET_2021_COOKIE'\nenv:\n  OPAQUE: '${TOKEN}'\n")
	out, err := runCLI(t, "render", ".agnostic-ai/mcps/local.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "PREVIEW_SECRET_2021_") {
		t.Error("accepted MCP arguments leaked through render")
	}
	if !strings.Contains(out, "${{ secrets.TOKEN }}") {
		t.Error("Continue reference was hidden in render")
	}
}

func TestCredentialPreview_UnclosedMarkdownDiskDiffHidesCredentials(t *testing.T) {
	for _, old := range []string{
		"---\r\npassword: PREVIEW_SECRET_2021_A\r\n---\r\nprose\r\n",
		"---\nenv: {OPAQUE: PREVIEW_SECRET_2021_A}\n---",
		"---\npassword: PREVIEW_SECRET_2021_A\n",
		"# Setup\n```json\n{\"password\":\"PREVIEW_SECRET_2021_A\"}",
	} {
		if strings.Contains(unifiedDiff("agent.md", old, "# Updated\n", diffBodyMax), "PREVIEW_SECRET_2021_") {
			t.Error("disk Markdown diff exposed a known structured credential")
		}
	}
}

func TestCredentialPreview_ReferenceURLLiteralIsHidden(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "remote.yaml"), "name: remote\ntype: http\nurl: 'https://${HOST}/mcp?api_key=PREVIEW_SECRET_2021_URL'\n")
	out, err := runCLI(t, "render", ".agnostic-ai/mcps/remote.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "PREVIEW_SECRET_2021_") {
		t.Error("reference-containing MCP URL leaked through render")
	}
}

func TestCredentialPreview_OpenCodeReferenceURLAndAttachedHeadersAreHidden(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [opencode]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "remote.yaml"), "name: remote\ntype: http\nurl: '${API_BASE}/mcp?password=PREVIEW_SECRET_2021_URL'\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "local.yaml"), "name: local\ncommand: curl\nargs:\n  - '-HX-Opaque: PREVIEW_SECRET_2021_HEADER'\n")
	for _, name := range []string{"remote", "local"} {
		out, err := runCLI(t, "render", ".agnostic-ai/mcps/"+name+".yaml")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out, "PREVIEW_SECRET_2021_") {
			t.Error("native URL or attached header leaked through render")
		}
	}
}

func TestCredentialPreview_OpenCodeCommandArrayHidesLiteralArgument(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [opencode]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "local.yaml"), "name: local\ncommand: server\nargs:\n  - --token\n  - PREVIEW_SECRET_2021_ARG\n")
	out, err := runCLI(t, "render", ".agnostic-ai/mcps/local.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "PREVIEW_SECRET_2021_") {
		t.Error("native command array leaked through render")
	}
}

func TestCredentialPreview_QuotedHookAssignmentIsHiddenInRender(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "check.yaml"), "name: check\nevent: PreToolUse\ncommand: env 'TOKEN=PREVIEW_SECRET_2021_HOOK' server\n")
	out, err := runCLI(t, "render", ".agnostic-ai/hooks/check.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "PREVIEW_SECRET_2021_") {
		t.Error("quoted hook assignment leaked through render")
	}
}

func TestCredentialPreview_PunctuationOnlyArgumentsRender(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "local.yaml"), "name: local\ncommand: server\nargs: ['?', '#']\n")
	if _, err := runCLI(t, "render", ".agnostic-ai/mcps/local.yaml"); err != nil {
		t.Fatal(err)
	}
}

func TestCredentialPreview_ReferenceHookAndRelativeURLAreHidden(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "check.yaml"), "name: check\nevent: PreToolUse\ncommand: '${BIN}/server --password PREVIEW_SECRET_2021_HOOK'\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "remote.yaml"), "name: remote\ntype: http\nurl: '#api_key=PREVIEW_SECRET_2021_URL'\n")
	for _, path := range []string{".agnostic-ai/hooks/check.yaml", ".agnostic-ai/mcps/remote.yaml"} {
		out, err := runCLI(t, "render", path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out, "PREVIEW_SECRET_2021_") {
			t.Error("reference command or relative URL leaked through render")
		}
	}
}

func TestCredentialPreview_HookAssignmentsWithURLPunctuationAreHidden(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	for _, command := range []string{
		"TOKEN=PREVIEW_SECRET_2021_HOOK;#comment",
		"TOKEN=PREVIEW_SECRET_2021_HOOK?",
		"TOKEN=PREVIEW_SECRET_2021_HOOK;URL=https://example.test",
	} {
		writeFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "check.yaml"), "name: check\nevent: PreToolUse\ncommand: '"+command+"'\n")
		out, err := runCLI(t, "render", ".agnostic-ai/hooks/check.yaml")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out, "PREVIEW_SECRET_2021_") {
			t.Error("hook assignment with URL punctuation leaked through render")
		}
	}
}

func TestCredentialPreview_CopilotSplitShellCommandsAreHidden(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [copilot]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "check.yaml"), "name: check\nevent: SessionStart\ncommand: '/bin/true;TOKEN=PREVIEW_SECRET_2021_BASH?'\ncommandWindows: 'Write-Output visible'\n")
	out, err := runCLI(t, "render", ".agnostic-ai/hooks/check.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "PREVIEW_SECRET_2021_") {
		t.Error("Copilot native bash command leaked through render")
	}
	if !strings.Contains(out, "Write-Output visible") {
		t.Error("ordinary Windows command was lost")
	}
}

func TestCredentialPreview_OpenCodeShellArgumentIsHidden(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [opencode]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "local.yaml"), "name: local\ncommand: bash\nargs: ['-c', '/bin/true;TOKEN=PREVIEW_SECRET_2021_ARG']\n")
	out, err := runCLI(t, "render", ".agnostic-ai/mcps/local.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "PREVIEW_SECRET_2021_") {
		t.Error("OpenCode shell argument leaked through render")
	}
}

func TestCredentialPreview_ContinueSubstitutionArgumentIsHidden(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [continue]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "local.yaml"), "name: local\ncommand: server\nargs: ['${BIN}/$(TOKEN=PREVIEW_SECRET_2021_ARG)']\n")
	out, err := runCLI(t, "render", ".agnostic-ai/mcps/local.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "PREVIEW_SECRET_2021_") {
		t.Error("Continue substitution argument leaked through render")
	}
}

func TestCredentialPreview_GlobalSummariesHideCredentialsAndPreserveWrites(t *testing.T) {
	home, source := globalAgentTestHome(t)
	localPath := filepath.Join(source, "mcps", "local.yaml")
	remotePath := filepath.Join(source, "mcps", "remote.yaml")
	local := "name: local\ncommand: server\nargs: ['--token', 'PREVIEW_SECRET_2021_FLAG']\nenv:\n  OPAQUE: !literal PREVIEW_SECRET_2021_ENV\n  REFERENCED: '${TOKEN}'\n"
	remote := "name: remote\ntype: http\nurl: 'https://${HOST}/mcp?api_key=PREVIEW_SECRET_2021_URL'\nheaders:\n  Authorization: !literal 'Bearer PREVIEW_SECRET_2021_HEADER_OLD'\n"
	mustWriteGlobalTest(t, localPath, local)
	mustWriteGlobalTest(t, remotePath, remote)
	path := filepath.Join(home, ".cursor", "mcp.json")

	checkPreview := func(args []string, wantError bool, reference bool) {
		t.Helper()
		before := snapshotProject(t, home)
		out, warnings, err := runGlobalAgentTest(append([]string{"--only", "cursor"}, args...)...)
		if (err != nil) != wantError {
			t.Errorf("global preview %v: unexpected success/failure", args)
		}
		public := out + warnings
		if err != nil {
			public += err.Error()
		}
		if strings.Contains(public, "PREVIEW_SECRET_2021_") {
			t.Errorf("global preview %v leaked a credential", args)
		}
		if len(args) > 0 && args[len(args)-1] == "--json" && !json.Valid([]byte(out)) {
			t.Errorf("global preview %v did not produce valid JSON", args)
		}
		if reference && (!strings.Contains(out, "mcpServers.local") || !strings.Contains(out, "server") || !strings.Contains(out, "${") || !strings.Contains(out, "TOKEN}")) {
			t.Errorf("global preview %v lost server names, ordinary command, or pure reference", args)
		}
		assertProjectUnchanged(t, before, snapshotProject(t, home))
	}
	previewModes := []struct {
		args  []string
		drift bool
	}{
		{[]string{"--dry-run"}, false},
		{[]string{"--dry-run", "--json"}, false},
		{[]string{"--plan"}, false},
		{[]string{"--plan", "--json"}, false},
		{[]string{"--check", "--json"}, true},
	}
	for _, mode := range previewModes {
		checkPreview(mode.args, mode.drift, true)
	}

	readServers := func() map[string]any {
		t.Helper()
		var doc map[string]any
		if err := json.Unmarshal([]byte(readGlobalTest(t, path)), &doc); err != nil {
			t.Fatal("decode native MCP output")
		}
		servers, ok := doc["mcpServers"].(map[string]any)
		if !ok {
			t.Fatal("native output lacks MCP servers")
		}
		return servers
	}
	assertRaw := func(header string) {
		t.Helper()
		servers := readServers()
		localServer := servers["local"].(map[string]any)
		remoteServer := servers["remote"].(map[string]any)
		if localServer["env"].(map[string]any)["OPAQUE"] != "PREVIEW_SECRET_2021_ENV" || localServer["args"].([]any)[1] != "PREVIEW_SECRET_2021_FLAG" {
			t.Error("global write changed literal environment or flag values")
		}
		if remoteServer["headers"].(map[string]any)["Authorization"] != header || !strings.HasSuffix(remoteServer["url"].(string), "?api_key=PREVIEW_SECRET_2021_URL") {
			t.Error("global write changed literal header or URL values")
		}
		var state globalState
		if err := json.Unmarshal([]byte(readGlobalTest(t, filepath.Join(source, "state", "global.json"))), &state); err != nil {
			t.Fatal("decode global ownership state")
		}
		for _, name := range []string{"local", "remote"} {
			if !sameSetting(state.MCP["cursor"]["mcpServers."+name], servers[name]) {
				t.Error("global ownership state no longer records the exact raw server value")
			}
		}
		if _, _, err := runGlobalAgentTest("--only", "cursor", "--check"); err != nil {
			t.Error("global check after raw write failed")
		}
	}
	if _, _, err := runGlobalAgentTest("--only", "cursor"); err != nil {
		t.Fatal("initial global write failed")
	}
	assertRaw("Bearer PREVIEW_SECRET_2021_HEADER_OLD")
	mustWriteGlobalTest(t, remotePath, strings.ReplaceAll(remote, "HEADER_OLD", "HEADER_NEW"))
	for _, mode := range previewModes {
		checkPreview(mode.args, mode.drift, false)
	}
	if _, _, err := runGlobalAgentTest("--only", "cursor"); err != nil {
		t.Fatal("owned global update failed")
	}
	assertRaw("Bearer PREVIEW_SECRET_2021_HEADER_NEW")

	servers := readServers()
	servers["remote"].(map[string]any)["headers"].(map[string]any)["Authorization"] = "Bearer PREVIEW_SECRET_2021_HANDWRITTEN"
	data, err := json.Marshal(map[string]any{"mcpServers": servers})
	if err != nil {
		t.Fatal("encode conflicting native fixture")
	}
	mustWriteGlobalTest(t, path, string(data))
	checkPreview([]string{"--dry-run"}, false, false)
	checkPreview([]string{"--plan", "--json"}, false, false)
	checkPreview(nil, true, false)
}

func TestCredentialPreview_GlobalEnvironmentArraysHideLiteralsAndPreserveWrites(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "opaque.yaml"), "name: opaque\nmodel: opus\nx-claude:\n  env:\n    args: [PREVIEW_SECRET_2021_OPAQUE]\n    command: [PREVIEW_SECRET_2021_COMMAND]\n    referenced: ['${TOKEN}']\n")
	before := snapshotProject(t, home)
	for _, args := range [][]string{{"--dry-run"}, {"--plan", "--json"}} {
		out, warnings, err := runGlobalAgentTest(append([]string{"--only", "claude"}, args...)...)
		if err != nil {
			t.Fatal("accepted native environment arrays failed to preview")
		}
		if strings.Contains(out+warnings, "PREVIEW_SECRET_2021_") {
			t.Errorf("global environment preview %v leaked literal array values", args)
		}
		if !strings.Contains(out, "${TOKEN}") || !strings.Contains(out, "opus") {
			t.Errorf("global environment preview %v lost reference or ordinary model", args)
		}
		if args[len(args)-1] == "--json" && !json.Valid([]byte(out)) {
			t.Error("global environment plan is not valid JSON")
		}
		assertProjectUnchanged(t, before, snapshotProject(t, home))
	}
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal("accepted native environment arrays failed to write")
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(readGlobalTest(t, filepath.Join(home, ".claude", "settings.json"))), &doc); err != nil {
		t.Fatal("decode native environment settings")
	}
	env := doc["env"].(map[string]any)
	for key, expected := range map[string]string{"args": "PREVIEW_SECRET_2021_OPAQUE", "command": "PREVIEW_SECRET_2021_COMMAND", "referenced": "${TOKEN}"} {
		values, ok := env[key].([]any)
		if !ok || len(values) != 1 || values[0] != expected {
			t.Errorf("global write changed accepted native environment array %s", key)
		}
	}
	var state globalState
	if err := json.Unmarshal([]byte(readGlobalTest(t, filepath.Join(source, "state", "global.json"))), &state); err != nil {
		t.Fatal("decode global environment ownership")
	}
	for _, key := range []string{"args", "command", "referenced"} {
		if !sameSetting(state.Settings["claude"]["env."+key], env[key]) {
			t.Errorf("global ownership changed raw environment array %s", key)
		}
	}
	if _, _, err := runGlobalAgentTest("--only", "claude", "--check"); err != nil {
		t.Error("global check after environment write failed")
	}
}
