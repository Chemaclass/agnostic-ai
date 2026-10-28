package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// The Codex overlay is committed, so a config.toml value that holds a
// credential stays out of it, as the Claude settings overlay does (#2669).
func TestImportCodexConfigOverlay_LeavesOutCredentials(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".codex", "config.toml"), `model = "gpt-6-luna"
notify = ["curl", "-H", "Authorization: Bearer NOTIFYSECRET123", "https://hooks.example.com"]

[model_providers.proxy]
base_url = "https://proxy.example.com/v1"
env_key = "OPENAI_API_KEY"
experimental_bearer_token = "sk-proj-PROVIDERSECRET456"
http_headers = { "X-Api-Key" = "HEADERSECRET789xyz", "X-Team" = "platform" }
env_http_headers = { "X-Org" = "ORG_TOKEN" }

[shell_environment_policy]
set = { GITHUB_TOKEN = "ghp_SHELLSECRET0123456789", NODE_ENV = "production" }
`)
	var log bytes.Buffer
	prev := logOut
	logOut = &log
	t.Cleanup(func() { logOut = prev })

	if _, _, err := importCodexConfigOverlay(root, filepath.Join(root, "settings"), adapters.GrantedRoots{}); err != nil {
		t.Fatalf("import: %v", err)
	}

	overlay := readFileString(t, codexOverlayPath(root))
	for _, secret := range []string{"NOTIFYSECRET123", "PROVIDERSECRET456", "HEADERSECRET789xyz", "SHELLSECRET0123456789"} {
		if strings.Contains(overlay, secret) {
			t.Errorf("overlay holds %s:\n%s", secret, overlay)
		}
	}
	for _, kept := range []string{"gpt-6-luna", "https://proxy.example.com/v1", "OPENAI_API_KEY", "platform", "ORG_TOKEN", "production"} {
		if !strings.Contains(overlay, kept) {
			t.Errorf("overlay lost %q:\n%s", kept, overlay)
		}
	}
	for _, path := range []string{"notify", "model_providers.proxy.experimental_bearer_token", "model_providers.proxy.http_headers.X-Api-Key", "shell_environment_policy.set.GITHUB_TOKEN"} {
		if !strings.Contains(log.String(), "warning: codex: left out "+path+" in "+codexConfigTOML+": it holds a credential") {
			t.Errorf("no warning names %s:\n%s", path, log.String())
		}
	}
	if strings.Contains(log.String(), "SECRET") {
		t.Errorf("warning printed a credential:\n%s", log.String())
	}
}

func TestImportCodexConfigOverlay_KeepsLayoutWithoutCredentials(t *testing.T) {
	root := t.TempDir()
	config := "# my model\nmodel = \"gpt-6-luna\"\n\n[shell_environment_policy]\nset = { NODE_ENV = \"production\" }\n"
	writeFile(t, filepath.Join(root, ".codex", "config.toml"), config)

	if _, _, err := importCodexConfigOverlay(root, filepath.Join(root, "settings"), adapters.GrantedRoots{}); err != nil {
		t.Fatalf("import: %v", err)
	}
	if got := readFileString(t, codexOverlayPath(root)); got != config {
		t.Errorf("overlay = %q, want the file as written %q", got, config)
	}
}

// A re-import keeps the overlay an earlier import wrote when the tool file
// still matches the last sync. One that holds a credential is not kept:
// import stops, and --overwrite writes it without the credential.
func TestImport_RestoredCodexOverlayLeavesOutCredentials(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	mustWriteFile(t, ".agnostic-ai/overlays/codex.config.toml", "model = \"gpt-6-luna\"\n\n[shell_environment_policy]\nset = { GITHUB_TOKEN = \"ghp_RESTOREDSECRET0123456\", NODE_ENV = \"production\" }\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}

	if _, err := runCLI(t, "import", "codex"); errs.CodeOf(err) != errs.CodeImportWouldReplace {
		t.Fatalf("import codex = %v, want AAI-203 instead of keeping the credential", err)
	}
	if out, err := runCLI(t, "import", "codex", "--overwrite"); err != nil {
		t.Fatalf("import codex --overwrite: %v\n%s", err, out)
	}

	got := readFile(t, ".agnostic-ai/overlays/codex.config.toml")
	if strings.Contains(got, "RESTOREDSECRET") || !strings.Contains(got, "production") || !strings.Contains(got, "gpt-6-luna") {
		t.Errorf("restored overlay = %q, want the token out and the rest kept", got)
	}
}

func TestImportCodexConfigOverlay_ReadsProseAndVariableNames(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".codex", "config.toml"), `developer_instructions = """
Local DB: postgres://postgres:postgres@localhost:5432/app
Token: never commit it
"""

[model_providers.proxy]
env_key = "sk-proj-PASTEDKEYSECRET12345"
env_http_headers = { "X-Org" = "ORG_TOKEN" }
`)
	silence(t)

	if _, _, err := importCodexConfigOverlay(root, filepath.Join(root, "settings"), adapters.GrantedRoots{}); err != nil {
		t.Fatalf("import: %v", err)
	}
	overlay := readFileString(t, codexOverlayPath(root))
	if strings.Contains(overlay, "PASTEDKEYSECRET") {
		t.Errorf("overlay kept a token pasted under env_key:\n%s", overlay)
	}
	for _, kept := range []string{"never commit it", "ORG_TOKEN"} {
		if !strings.Contains(overlay, kept) {
			t.Errorf("overlay lost %q:\n%s", kept, overlay)
		}
	}
}

func TestImportCodexConfigOverlay_WritesNoOverlayForOnlyCredentials(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".codex", "config.toml"), "[model_providers.p]\nexperimental_bearer_token = \"sk-proj-ONLYSECRET123456789\"\n")
	silence(t)

	seeded, _, err := importCodexConfigOverlay(root, filepath.Join(root, "settings"), adapters.GrantedRoots{})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if seeded {
		t.Error("import reported an overlay it left empty")
	}
	if _, err := os.Stat(codexOverlayPath(root)); !os.IsNotExist(err) {
		t.Errorf("overlay written for a credentials-only config: %v", err)
	}
}

func TestImport_RestoredClaudeOverlayLeavesOutCredentials(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/overlays/claude.settings.json", "{\n  \"env\": {\n    \"ANTHROPIC_AUTH_TOKEN\": \"sk-ant-api03-RESTOREDCLAUDE99\",\n    \"DISABLE_TELEMETRY\": \"1\"\n  },\n  \"model\": \"opus\"\n}\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}

	if _, err := runCLI(t, "import", "claude"); errs.CodeOf(err) != errs.CodeImportWouldReplace {
		t.Fatalf("import claude = %v, want AAI-203 instead of keeping the credential", err)
	}
	if out, err := runCLI(t, "import", "claude", "--overwrite"); err != nil {
		t.Fatalf("import claude --overwrite: %v\n%s", err, out)
	}

	got := readFile(t, ".agnostic-ai/overlays/claude.settings.json")
	if strings.Contains(got, "RESTOREDCLAUDE") || !strings.Contains(got, "DISABLE_TELEMETRY") || !strings.Contains(got, "opus") {
		t.Errorf("restored overlay = %q, want the token out and the rest kept", got)
	}
}
