package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func mcpEntry(t *testing.T, yaml string) spec.Entry {
	t.Helper()
	e, err := spec.ParseYAMLBytes(spec.KindMCP, []byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	e.Path = "mcps/" + e.Name + ".yaml"
	return e
}

func TestLintMCPLiterals_NamesServerFieldAndKeyNeverTheValue(t *testing.T) {
	e := mcpEntry(t, "name: api\ntype: http\nurl: https://x.example/mcp\nenv:\n  API_KEY: sk-live-abc\nheaders:\n  X-Tenant: acme-corp\n")
	findings := lintMCPLiterals([]spec.Entry{e})
	if len(findings) != 2 {
		t.Fatalf("findings = %+v, want one per literal", findings)
	}
	for i, want := range []string{`"api": env API_KEY`, `"api": headers X-Tenant`} {
		f := findings[i]
		if f.Code != "LINT035" || f.Severity != lintError || f.Path != "mcps/api.yaml" || !strings.Contains(f.Message, want) {
			t.Errorf("finding %d = %+v, want a LINT035 error naming %s", i, f, want)
		}
		if strings.Contains(f.Message, "sk-live-abc") || strings.Contains(f.Message, "acme-corp") {
			t.Errorf("finding %d quotes the value: %s", i, f.Message)
		}
	}
}

func TestLintMCPLiterals_LeavesReferencesMarkedAndNonStringValuesAlone(t *testing.T) {
	e := mcpEntry(t, `name: ok
command: srv
env:
  TOKEN: ${TOKEN}
  WITH_DEFAULT: ${MODE:-fast}
  TWO: ${A}${B}
  ESCAPED: $${USER}
  NODE_ENV: !literal production
  PORT: 8080
  VERBOSE: true
  EMPTY: ""
headers:
  Authorization: Bearer ${API_KEY}
  X-GitHub: token ${GITHUB_TOKEN}
x-claude:
  env:
    MODE: fast
`)
	if findings := lintMCPLiterals([]spec.Entry{e}); len(findings) != 0 {
		t.Errorf("findings = %+v, want none", findings)
	}
}

func TestLintMCPLiterals_TextAroundAReferenceIsALiteral(t *testing.T) {
	e := mcpEntry(t, "name: db\ncommand: srv\nenv:\n  DATABASE_URL: postgres://u:pw@${HOST}/db\n  SCHEME: Basic ${CREDS}\n")
	findings := lintMCPLiterals([]spec.Entry{e})
	if len(findings) != 2 {
		t.Errorf("findings = %+v, want both env values: text around a reference may be the secret, and only a header takes a scheme word", findings)
	}
}

// The issue's first scenario: lint and sync both fail, naming the
// server and key without printing the value, and sync writes nothing.
func TestLintAndSync_HandWrittenLiteralFailsWithoutTheValue(t *testing.T) {
	dir := newProject(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "api.yaml"), "name: api\ncommand: srv\nenv: {API_KEY: sk-live-abc}\n")

	out, err := runCLI(t, "lint")
	if err == nil || !strings.Contains(out, "LINT035 [error] "+filepath.Join(".agnostic-ai", "mcps", "api.yaml")+`: MCP server "api": env API_KEY is a literal value`) {
		t.Errorf("lint must fail on LINT035: %v\n%s", err, out)
	}

	syncOut, syncErr := runCLI(t, "sync")
	if syncErr == nil || !strings.Contains(syncErr.Error(), `MCP server "api": env API_KEY is a literal value`) {
		t.Errorf("sync must refuse the spec with the lint message: %v\n%s", syncErr, syncOut)
	}
	for _, text := range []string{out, syncOut, fmt.Sprint(syncErr)} {
		if strings.Contains(text, "sk-live-abc") {
			t.Errorf("output quotes the value:\n%s", text)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".mcp.json")); !os.IsNotExist(err) {
		t.Errorf("sync must write nothing: %v", err)
	}
}

// `!literal production` syncs as the bare value on every target that
// writes MCP servers, where the unmarked value no longer syncs.
func TestSync_LiteralTagWritesTheBareValueOnEveryTarget(t *testing.T) {
	var targets []string
	for target := range targetsSupportingKind[spec.KindMCP] {
		targets = append(targets, target)
	}
	dir := testutil.TempCwd(t)
	silence(t)
	captureLogOut(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+strings.Join(targets, ", ")+"]\n")
	mcp := filepath.Join(dir, ".agnostic-ai", "mcps", "app.yaml")
	mustWriteFile(t, mcp, "name: app\ncommand: srv\nenv:\n  NODE_ENV: production\n")
	if _, err := runCLI(t, "sync"); err == nil {
		t.Fatal("sync must refuse the unmarked value")
	}
	mustWriteFile(t, mcp, "name: app\ncommand: srv\nenv:\n  NODE_ENV: !literal production\n")
	mustSync(t)
	written := 0
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.Contains(path, ".agnostic-ai") {
			return err
		}
		data, err := os.ReadFile(path)
		if strings.Contains(string(data), "!literal") {
			t.Errorf("%s keeps the YAML-only tag", path)
		}
		if strings.Contains(string(data), "NODE_ENV") {
			written++
			if !strings.Contains(string(data), "production") {
				t.Errorf("%s lost the value:\n%s", path, data)
			}
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if written == 0 {
		t.Fatal("no target wrote the server")
	}
	if got, _ := os.ReadFile(filepath.Join(dir, ".mcp.json")); !strings.Contains(string(got), `"NODE_ENV": "production"`) {
		t.Errorf(".mcp.json = %s, want the bare value", got)
	}
}
