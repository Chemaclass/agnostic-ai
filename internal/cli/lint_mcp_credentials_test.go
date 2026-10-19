package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const lint049Token = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"

func TestLintMCPCredentials_WarnsOnEachFormOutsideEnvAndHeaders(t *testing.T) {
	cases := map[string]struct{ body, field string }{
		"url userinfo":             {"type: http\nurl: https://user:hunter2secretpw@mcp.example.com/mcp\n", "url"},
		"url query":                {"type: http\nurl: https://mcp.example.com/mcp?token=" + lint049Token + "\n", "url"},
		"args flag":                {"command: srv\nargs: [--api-key=sk-live-abcdef1234567890]\n", "args"},
		"args separate value":      {"command: srv\nargs: [--token, " + lint049Token + "]\n", "args"},
		"x-target env":             {"command: srv\nx-cursor:\n  env:\n    GITHUB_TOKEN: " + lint049Token + "\n", "x-cursor.env GITHUB_TOKEN"},
		"x-target headers":         {"type: http\nurl: https://x.example/mcp\nx-claude:\n  headers:\n    Authorization: Bearer " + lint049Token + "\n", "x-claude.headers Authorization"},
		"x-target url":             {"type: http\nurl: https://x.example/mcp\nx-cursor:\n  url: https://u:hunter2secretpw@x.example/mcp\n", "x-cursor.url"},
		"env reference default":    {"command: srv\nenv:\n  GITHUB_TOKEN: ${GITHUB_TOKEN:-" + lint049Token + "}\n", "env GITHUB_TOKEN"},
		"header reference default": {"type: http\nurl: https://x.example/mcp\nheaders:\n  Authorization: Bearer ${API_KEY:-sk-live-abcdef1234567890}\n", "headers Authorization"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			findings := lintMCPCredentials([]spec.Entry{mcpEntry(t, "name: api\n"+c.body)})
			if len(findings) != 1 {
				t.Fatalf("findings = %+v, want one", findings)
			}
			f := findings[0]
			if f.Code != "LINT049" || f.Severity != lintWarn || !strings.Contains(f.Message, `MCP server "api": `+c.field+" ") {
				t.Errorf("finding = %+v, want a LINT049 warning naming %s", f, c.field)
			}
			for _, secret := range []string{lint049Token, "hunter2secretpw", "sk-live-abcdef1234567890"} {
				if strings.Contains(f.Message, secret) {
					t.Errorf("finding quotes the value: %s", f.Message)
				}
			}
		})
	}
}

func TestLintMCPCredentials_LeavesReferencesAndPlainValuesAlone(t *testing.T) {
	e := `name: ok
type: http
url: https://${API_HOST}/mcp?region=eu&token=${API_TOKEN}
args: [--port, "8080", --token, "${GH_TOKEN}", --mode=fast]
env:
  GITHUB_TOKEN: ${GITHUB_TOKEN}
  MODE: ${MODE:-fast}
  NODE_ENV: !literal production
headers:
  Authorization: Bearer ${API_KEY}
x-cursor:
  env:
    GITHUB_TOKEN: ${env:GITHUB_TOKEN}
    LOG_LEVEL: debug
  headers:
    X-Team: platform
`
	if findings := lintMCPCredentials([]spec.Entry{mcpEntry(t, e)}); len(findings) != 0 {
		t.Errorf("findings = %+v, want none", findings)
	}
}

// LINT049 warns: the detector reads shapes, and a url or args value has
// no `!literal` mark to silence a false match, so sync still runs.
func TestLintAndSync_CredentialInArgsWarnsWithoutStoppingSync(t *testing.T) {
	dir := newProject(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "api.yaml"), "name: api\ncommand: srv\nargs: [--token, "+lint049Token+"]\n")
	out, err := runCLI(t, "lint")
	if !strings.Contains(out, `LINT049 [warn] `+filepath.Join(".agnostic-ai", "mcps", "api.yaml")+`: MCP server "api": args holds`) {
		t.Errorf("lint must warn with LINT049: %v\n%s", err, out)
	}
	if strings.Contains(out, lint049Token) {
		t.Errorf("lint quotes the value:\n%s", out)
	}
	if _, err := runCLI(t, "lint", "--strict"); err == nil {
		t.Errorf("lint --strict must fail on the warning")
	}
	if out, err := runCLI(t, "sync"); err != nil {
		t.Errorf("a warning must not stop sync: %v\n%s", err, out)
	}
}
