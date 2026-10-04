package cli

import (
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
		if f.Code != "LINT035" || f.Severity != lintWarn || f.Path != "mcps/api.yaml" || !strings.Contains(f.Message, want) {
			t.Errorf("finding %d = %+v, want a LINT035 warning naming %s", i, f, want)
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

// The issue's first scenario, in phase 1: lint warns and fails under
// --strict, and sync still writes the value but says so, all without
// printing it.
func TestLintAndSync_HandWrittenLiteralWarnsWithoutTheValue(t *testing.T) {
	dir := newProject(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "api.yaml"), "name: api\ncommand: srv\nenv: {API_KEY: sk-live-abc}\n")

	out, err := runCLI(t, "lint")
	if err != nil || !strings.Contains(out, `LINT035 [warn] .agnostic-ai/mcps/api.yaml: MCP server "api": env API_KEY is a literal value`) {
		t.Errorf("lint: %v\n%s", err, out)
	}
	if out, err := runCLI(t, "lint", "--strict"); err == nil {
		t.Errorf("lint --strict must fail on LINT035:\n%s", out)
	}

	log := captureLogOut(t)
	mustSync(t)
	if !strings.Contains(log.String(), "1 MCP env or headers value is neither a ${NAME} reference nor marked !literal (LINT035)") {
		t.Errorf("sync must say why the value is a problem:\n%s", log)
	}
	for _, text := range []string{out, log.String()} {
		if strings.Contains(text, "sk-live-abc") {
			t.Errorf("output quotes the value:\n%s", text)
		}
	}
	got, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil || !strings.Contains(string(got), `"API_KEY": "sk-live-abc"`) {
		t.Errorf("phase 1 sync writes the literal as before: %v\n%s", err, got)
	}
}

// `!literal production` syncs exactly as a plain `production` did, on
// every target that writes MCP servers.
func TestSync_LiteralTagWritesTheBareValueOnEveryTarget(t *testing.T) {
	var targets []string
	for target := range targetsSupportingKind[spec.KindMCP] {
		targets = append(targets, target)
	}
	config := "version: 1\ntargets: [" + strings.Join(targets, ", ") + "]\n"
	outputs := func(env string) map[string]string {
		t.Helper()
		dir := testutil.TempCwd(t)
		silence(t)
		captureLogOut(t)
		mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), config)
		mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "app.yaml"), "name: app\ncommand: srv\nenv:\n  NODE_ENV: "+env+"\n")
		mustSync(t)
		files := map[string]string{}
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || strings.Contains(path, ".agnostic-ai") {
				return err
			}
			data, err := os.ReadFile(path)
			rel, _ := filepath.Rel(dir, path)
			files[rel] = string(data)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return files
	}
	plain, marked := outputs("production"), outputs("!literal production")
	if len(plain) == 0 || len(plain) != len(marked) {
		t.Fatalf("outputs differ in files: %d plain, %d marked", len(plain), len(marked))
	}
	for path, want := range plain {
		if marked[path] != want {
			t.Errorf("%s differs:\nplain:\n%s\nmarked:\n%s", path, want, marked[path])
		}
		if strings.Contains(marked[path], "!literal") {
			t.Errorf("%s keeps the YAML-only tag", path)
		}
	}
	if !strings.Contains(marked[".mcp.json"], `"NODE_ENV": "production"`) {
		t.Errorf(".mcp.json = %s, want the bare value", marked[".mcp.json"])
	}
}
