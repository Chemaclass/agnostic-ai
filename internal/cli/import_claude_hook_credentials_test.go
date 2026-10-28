package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// The Slack fixtures are split so push protection does not read them as
// real secrets.
const (
	slackBotToken = "xox" + "b-1234567890-MCPINPUTSECRET"
	slackWebhook  = "https://hooks.slack" + ".com/services/T000/B000/SLACKSECRET42"
)

// A hook spec is committed, so a matcher group with a hook that holds a
// credential is left out whole with a warning, and sync leaves the group
// in settings.json as written (#2668).
func TestImportClaudeHooks_LeavesOutCredentialHooks(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".claude", "settings.json"), `{"hooks": {"PostToolUse": [{"matcher": "Bash", "hooks": [
  {"type": "command", "command": "curl -H 'Authorization: Bearer HOOKCMDSECRET123' https://hooks.example.com"},
  {"type": "command", "command": "notify", "args": ["--api-key", "HOOKARGSECRET4567"]},
  {"type": "http", "url": "https://hooks.example.com/in?token=HOOKURLSECRET890"},
  {"type": "http", "url": "https://hooks.example.com/in", "headers": {"Authorization": "Bearer HOOKHDRSECRET246"}},
  {"type": "command", "command": "go vet ./..."}
]}],
"Stop": [
  {"matcher": "", "hooks": [{"type": "command", "command": "curl -X POST -d '{}' `+slackWebhook+`"}]},
  {"matcher": "", "hooks": [{"type": "mcp_tool", "server": "slack", "tool": "post", "input": {"token": "`+slackBotToken+`"}}]},
  {"matcher": "", "hooks": [{"type": "command", "command": "echo done", "statusMessage": "using ghp_STATUSSECRET1234567890"}]},
  {"matcher": "", "hooks": [{"type": "command", "command": "npm test"}]}
],
"PreToolUse": [{"matcher": "Bash", "hooks": [
  {"type": "http", "url": "https://hooks.example.com/ok", "headers": {"Authorization": "Bearer $HOOK_TOKEN"}, "allowedEnvVars": ["HOOK_TOKEN"]},
  {"type": "command", "command": "gh pr view --json title"}
]}]}}`)
	var log bytes.Buffer
	prev := logOut
	logOut = &log
	t.Cleanup(func() { logOut = prev })
	dst := filepath.Join(root, ".agnostic-ai", "hooks")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}

	n, err := importClaudeHooks(root, dst)
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	var all strings.Builder
	entries, _ := os.ReadDir(dst)
	for _, e := range entries {
		all.WriteString(readFileString(t, filepath.Join(dst, e.Name())))
	}
	for _, secret := range []string{"HOOKCMDSECRET123", "HOOKARGSECRET4567", "HOOKURLSECRET890", "HOOKHDRSECRET246", "SLACKSECRET42", "MCPINPUTSECRET", "STATUSSECRET", "go vet"} {
		if strings.Contains(all.String(), secret) {
			t.Errorf("hook specs hold %s:\n%s", secret, all.String())
		}
	}
	for _, kept := range []string{"npm test", "gh pr view", "$HOOK_TOKEN"} {
		if !strings.Contains(all.String(), kept) {
			t.Errorf("hook specs lost %q:\n%s", kept, all.String())
		}
	}
	if n != 3 {
		t.Errorf("wrote %d hook specs, want 3", n)
	}
	for _, want := range []string{
		`left out the PostToolUse hooks matching "Bash" in ` + filepath.Join(claudeDir, "settings.json") + `: a hook's command holds a credential`,
		`left out the Stop hooks matching "" in ` + filepath.Join(claudeDir, "settings.json") + `: a hook's command holds a credential`,
		"a hook's input.token holds a credential",
		"a hook's statusMessage holds a credential",
	} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("no warning says %q:\n%s", want, log.String())
		}
	}
	if strings.Contains(log.String(), "SECRET") {
		t.Errorf("warning printed a credential:\n%s", log.String())
	}
}

// Sync keeps the left-out group as written and adds none of its hooks a
// second time.
func TestImportClaudeHooks_SyncKeepsALeftOutGroupOnce(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".claude/settings.json", `{"hooks": {"PostToolUse": [{"matcher": "Bash", "hooks": [
  {"type": "command", "command": "curl -H 'Authorization: Bearer SYNCSECRET123456' https://hooks.example.com"},
  {"type": "command", "command": "go vet ./..."}
]}]}}`)
	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import claude: %v\n%s", err, out)
	}
	for range 2 {
		if out, err := runCLI(t, "sync"); err != nil {
			t.Fatalf("sync: %v\n%s", err, out)
		}
	}
	settings := readFile(t, ".claude/settings.json")
	if n := strings.Count(settings, "go vet ./..."); n != 1 {
		t.Errorf("settings.json runs go vet %d times, want once:\n%s", n, settings)
	}
}
