package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Sync references in `url` and `args`, then import the native file
// back: the specs must say the same references and keep the literals,
// and a second sync must write the same bytes.
func TestImportMCP_URLArgsRefsRoundTrip(t *testing.T) {
	for target, tc := range map[string]struct{ native, url, arg string }{
		"claude":   {".mcp.json", `"https://${API_HOST}/mcp"`, `"${GH_TOKEN}"`},
		"windsurf": {".devin/mcp_config.json", `"https://${env:API_HOST}/mcp"`, `"${env:GH_TOKEN}"`},
		"opencode": {"opencode.json", `"https://{env:API_HOST}/mcp"`, `"{env:GH_TOKEN}"`},
		"continue": {".continue/mcpServers/gh.yaml", "", "${{ secrets.GH_TOKEN }}"},
	} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			silence(t)
			captureLog(t)
			writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+target+"]\n")
			gh := "name: gh\ncommand: gh-mcp\nargs:\n  - --token\n  - ${GH_TOKEN}\n  - --root\n  - ${workspaceFolder}\n  - pa55$word\n"
			api := "name: api\ntype: sse\nurl: https://${API_HOST}/mcp\n"
			plain := "name: plain\ntype: sse\nurl: https://mcp.example.com/mcp?$filter=x\n"
			writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "gh.yaml"), gh)
			writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "api.yaml"), api)
			writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "plain.yaml"), plain)

			execCLI(t, "sync", "-t", target)
			native := snapshotEmitted(t, dir)[tc.native]
			for _, want := range []string{tc.url, tc.arg, "${workspaceFolder}", "pa55$word"} {
				if !strings.Contains(native, want) {
					t.Fatalf("%s lacks %s:\n%s", tc.native, want, native)
				}
			}
			if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai")); err != nil {
				t.Fatal(err)
			}
			execCLI(t, "import", target)

			for file, wants := range map[string][]string{
				"gh.yaml":    {"- ${GH_TOKEN}", "- ${workspaceFolder}", "- pa55$word"},
				"api.yaml":   {"url: https://${API_HOST}/mcp"},
				"plain.yaml": {"url: https://mcp.example.com/mcp?$filter=x"},
			} {
				got := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", file))
				for _, want := range wants {
					if !strings.Contains(got, want) {
						t.Errorf("%s lacks %q:\n%s", file, want, got)
					}
				}
			}
			execCLI(t, "sync", "-t", target)
			if second := snapshotEmitted(t, dir)[tc.native]; second != native {
				t.Errorf("%s changed after import:\n%s\nwant:\n%s", tc.native, second, native)
			}
			execCLI(t, "sync", "--check", "-t", target)
		})
	}
}

func TestImportMCP_URLArgsLiteralsStayLiteral(t *testing.T) {
	specs, out := importMCPServers(t, "claude", map[string]any{
		"gh":    map[string]any{"command": "gh-mcp", "args": []any{"--token", "ghp_example", "--host", "$HOST"}},
		"plain": map[string]any{"command": "pg-mcp", "args": []any{"postgresql://admin@db:5432/app", "https://x.example/mcp?page=2&monkey=1"}},
		"api":   map[string]any{"type": "http", "url": "https://api.example.com/mcp?filter=x&keyword=y"},
	})
	for name, wants := range map[string][]string{
		"gh":    {"- ghp_example", "- $HOST"},
		"plain": {"- postgresql://admin@db:5432/app", "- https://x.example/mcp?page=2&monkey=1"},
		"api":   {"url: https://api.example.com/mcp?filter=x&keyword=y"},
	} {
		for _, want := range wants {
			if !strings.Contains(specs[name], want) {
				t.Errorf("import must keep a url or argument with no credential (%q):\n%s", want, specs[name])
			}
		}
	}
	if strings.Contains(out, "now reads") {
		t.Errorf("import reports no replacement when no credential is found:\n%s", out)
	}
}

func TestImportMCP_URLArgsCredentialsBecomeReferences(t *testing.T) {
	secrets := []string{"PASSW0RD", "T0KEN", "K3Y", "S3CRET", "PW2"}
	specs, out := importMCPServers(t, "claude", map[string]any{
		"pg":  map[string]any{"command": "npx", "args": []any{"-y", "@modelcontextprotocol/server-postgres", "postgresql://admin:PASSW0RD@db:5432/app"}},
		"api": map[string]any{"url": "https://x.example/mcp?token=T0KEN&page=2&api_key=K3Y#top"},
		"db":  map[string]any{"command": "db-mcp", "args": []any{"--url=mysql://root:${DB_PW}@h/x", "https://u:PW2@h/x?client_secret=S3CRET"}},
	})
	for name, spec := range specs {
		for _, secret := range secrets {
			if strings.Contains(spec, secret) {
				t.Errorf("spec %s keeps a credential from url or args", name)
			}
		}
	}
	for _, secret := range secrets {
		if strings.Contains(out, secret) {
			t.Errorf("import output prints a credential value")
		}
	}
	for name, wants := range map[string][]string{
		"pg":  {"- postgresql://admin:${PG_PASSWORD}@db:5432/app"},
		"api": {"url: https://x.example/mcp?token=${API_TOKEN}&page=2&api_key=${API_API_KEY}#top"},
		"db":  {"- --url=mysql://root:${DB_PW}@h/x", "- https://u:${DB_PASSWORD}@h/x?client_secret=${DB_CLIENT_SECRET}"},
	} {
		for _, want := range wants {
			if !strings.Contains(specs[name], want) {
				t.Errorf("spec %s lacks %q", name, want)
			}
		}
	}
	for _, want := range []string{
		"MCP server pg: args[2] password now reads ${PG_PASSWORD}; set PG_PASSWORD",
		"MCP server api: url token now reads ${API_TOKEN}; set API_TOKEN",
		"MCP server api: url api_key now reads ${API_API_KEY}; set API_API_KEY",
		"MCP server db: args[1] password now reads ${DB_PASSWORD}; set DB_PASSWORD",
		"MCP server db: args[1] client_secret now reads ${DB_CLIENT_SECRET}; set DB_CLIENT_SECRET",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	if strings.Contains(out, "DB_PW") {
		t.Errorf("a reference already in a url is not reported:\n%s", out)
	}
}

func TestImportMCP_URLArgsCredentialDefaultsAndTemplatedBase(t *testing.T) {
	secrets := []string{"SK-L1VE", "0THER", "T0KEN", "PW-DEF", "ARG-T0KEN"}
	specs, out := importMCPServers(t, "claude", map[string]any{
		"api":  map[string]any{"url": "https://x.example/mcp?token=${TOKEN:-SK-L1VE}&key=0THER"},
		"base": map[string]any{"url": "${API_BASE}/mcp?token=T0KEN&page=2"},
		"db":   map[string]any{"command": "db-mcp", "args": []any{"postgres://u:${PW:-PW-DEF}@h/x", "${API_BASE}/mcp?api_key=ARG-T0KEN"}},
	})
	for name, spec := range specs {
		for _, secret := range secrets {
			if strings.Contains(spec, secret) {
				t.Errorf("spec %s keeps a credential from url or args", name)
			}
		}
	}
	for _, secret := range secrets {
		if strings.Contains(out, secret) {
			t.Errorf("import output prints a credential value")
		}
	}
	for name, wants := range map[string][]string{
		"api":  {"url: https://x.example/mcp?token=${TOKEN}&key=${API_KEY}"},
		"base": {"url: ${API_BASE}/mcp?token=${BASE_TOKEN}&page=2"},
		"db":   {"- postgres://u:${PW}@h/x", "- ${API_BASE}/mcp?api_key=${DB_API_KEY}"},
	} {
		for _, want := range wants {
			if !strings.Contains(specs[name], want) {
				t.Errorf("spec %s lacks %q", name, want)
			}
		}
	}
	for _, want := range []string{
		"MCP server api: url token now reads ${TOKEN} without its default; set TOKEN",
		"MCP server api: url key now reads ${API_KEY}; set API_KEY",
		"MCP server base: url token now reads ${BASE_TOKEN}; set BASE_TOKEN",
		"MCP server db: args[0] password now reads ${PW} without its default; set PW",
		"MCP server db: args[1] api_key now reads ${DB_API_KEY}; set DB_API_KEY",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q", want)
		}
	}
}

func TestImportMCP_URLCamelCaseCredentialNames(t *testing.T) {
	secrets := []string{"CL1ENT", "AP1TOK", "S3CKEY", "DBP4SS"}
	specs, out := importMCPServers(t, "claude", map[string]any{
		"api": map[string]any{"url": "https://x.example/mcp?clientSecret=CL1ENT&apiToken=AP1TOK&secretKey=S3CKEY&dbPassword=DBP4SS&pageSize=2"},
	})
	for _, secret := range secrets {
		if strings.Contains(specs["api"], secret) || strings.Contains(out, secret) {
			t.Errorf("a camelCase credential parameter reached the spec or the output")
		}
	}
	want := "url: https://x.example/mcp?clientSecret=${API_CLIENTSECRET}&apiToken=${API_APITOKEN}&secretKey=${API_SECRETKEY}&dbPassword=${API_DBPASSWORD}&pageSize=2"
	if !strings.Contains(specs["api"], want) {
		t.Errorf("spec api lacks %q", want)
	}
}

func TestImportMCP_URLUsernameReferenceKeepsDefault(t *testing.T) {
	specs, out := importMCPServers(t, "claude", map[string]any{
		"pg": map[string]any{"command": "pg-mcp", "args": []any{
			"postgresql://${DB_USER:-admin}:PASSW0RD@db/app",
			"${BASE:-https://h?x=1}/mcp?token=T0KEN",
		}},
	})
	if strings.Contains(specs["pg"], "PASSW0RD") || strings.Contains(specs["pg"], "T0KEN") || strings.Contains(out, "PASSW0RD") || strings.Contains(out, "T0KEN") {
		t.Errorf("a credential reached the spec or the output")
	}
	for _, want := range []string{
		"- postgresql://${DB_USER:-admin}:${PG_PASSWORD}@db/app",
		"- ${BASE:-https://h?x=1}/mcp?token=${PG_TOKEN}",
	} {
		if !strings.Contains(specs["pg"], want) {
			t.Errorf("spec pg lacks %q", want)
		}
	}
}

func TestImportMCP_URLCredentialInReferenceDefault(t *testing.T) {
	specs, out := importMCPServers(t, "claude", map[string]any{
		"api": map[string]any{"url": "${MCP_URL:-https://x.example/mcp?token=T0KEN}"},
		"db":  map[string]any{"command": "db-mcp", "args": []any{"${DB_URL:-postgres://u:PASSW0RD@h/x}", "${PLAIN:-https://h/x}"}},
	})
	for name, spec := range specs {
		if strings.Contains(spec, "T0KEN") || strings.Contains(spec, "PASSW0RD") {
			t.Errorf("spec %s keeps a credential from a reference default", name)
		}
	}
	if strings.Contains(out, "T0KEN") || strings.Contains(out, "PASSW0RD") {
		t.Errorf("import output prints a credential value")
	}
	for name, wants := range map[string][]string{
		"api": {"url: ${MCP_URL}"},
		"db":  {"- ${DB_URL}", "- ${PLAIN:-https://h/x}"},
	} {
		for _, want := range wants {
			if !strings.Contains(specs[name], want) {
				t.Errorf("spec %s lacks %q", name, want)
			}
		}
	}
	for _, want := range []string{
		"MCP server api: url reference now reads ${MCP_URL} without its default; set MCP_URL",
		"MCP server db: args[0] reference now reads ${DB_URL} without its default; set DB_URL",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q", want)
		}
	}
}

func TestImportMCP_URLCredentialOutsideOneURLLeavesServerOut(t *testing.T) {
	for target, servers := range map[string]map[string]any{
		"claude": {
			"sh":     map[string]any{"command": "sh", "args": []any{"-c", "curl https://health.example/ping?a=1&b=2; exec pg-mcp postgresql://admin:PASSW0RD@db/app"}, "env": map[string]any{"K": "v"}},
			"quoted": map[string]any{"command": "sh", "args": []any{"-c", "curl 'https://x.example/mcp'?token=T0KEN"}},
			"ops":    map[string]any{"command": "sh", "args": []any{"-c", "curl https://x.example/a?token=T0KEN; exec srv"}},
			"flag":   map[string]any{"command": "x", "args": []any{"--url=${API_BASE}/mcp?api_key=T0KEN"}},
			"two":    map[string]any{"command": "x", "args": []any{"https://a.example,https://u:PASSW0RD@b.example"}},
			"quote":  map[string]any{"url": "https://admin:PA;SS(W)'D@x.example/mcp"},
			"slash":  map[string]any{"url": "https://admin:PA/SSW0RD@x.example/mcp"},
			"at":     map[string]any{"command": "x", "args": []any{"https://admin:PA@SSW0RD@x.example/mcp"}},
			"good":   map[string]any{"command": "y"},
		},
		"warp": {
			"escaped": map[string]any{"url": "${MCP_URL:-https://x.example/mcp?token=T0KEN}"},
			"good":    map[string]any{"command": "y"},
		},
	} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			log := captureLog(t)
			fields := map[string]string{}
			for name, raw := range servers {
				server := raw.(map[string]any)
				switch {
				case server["url"] != nil:
					fields[name] = "url"
				case name == "sh" || name == "quoted" || name == "ops":
					fields[name] = "args[1]"
				case server["args"] != nil:
					fields[name] = "args[0]"
				}
			}
			if _, err := writeMCPYAMLs(target, servers, dir); err != nil {
				t.Fatal(err)
			}
			out := log.String()
			for _, secret := range []string{"PASSW0RD", "T0KEN", "PA;SS", "SSW0RD"} {
				if strings.Contains(out, secret) {
					t.Errorf("import output prints a credential value")
				}
			}
			for name, field := range fields {
				if _, err := os.Stat(filepath.Join(dir, name+".yaml")); !os.IsNotExist(err) {
					t.Errorf("server %s holds a credential import cannot rewrite and must be left out", name)
				}
				if want := "MCP server " + name + ": left out; " + field + " holds a URL credential import cannot rewrite"; !strings.Contains(out, want) {
					t.Errorf("output lacks %q", want)
				}
			}
			if _, err := os.Stat(filepath.Join(dir, "good.yaml")); err != nil {
				t.Errorf("a server without a credential is written: %v", err)
			}
			if strings.Contains(out, "MCP server sh: env") {
				t.Errorf("a left-out server reports no replacement:\n%s", out)
			}
		})
	}
}

func TestImportMCP_URLCredentialInStringArgsFromNativeImport(t *testing.T) {
	dir := t.TempDir()
	captureLog(t)
	writeFile(t, filepath.Join(dir, ".codex", "config.toml"), "[mcp_servers.pg]\ncommand = \"npx\"\nargs = [\"-y\", \"postgresql://admin:PASSW0RD@db/app\"]\n")
	writeFile(t, filepath.Join(dir, opencodeMCPFile), `{"mcp": {"pg": {"type": "local", "command": ["npx", "-y", "postgresql://admin:PASSW0RD@db/app"]}}}`)
	writeFile(t, filepath.Join(dir, ".zed", "settings.json"), `{"context_servers": {"pg": {"command": "npx", "args": ["-y", "postgresql://admin:PASSW0RD@db/app"]}}}`)
	for target, run := range map[string]func(dst string) error{
		"codex": func(dst string) error {
			_, _, err := importCodexConfig(dir, filepath.Join(dir, "hooks-codex"), dst)
			return err
		},
		"opencode": func(dst string) error { _, err := importOpencodeMCP(dir, dst); return err },
		"zed":      func(dst string) error { _, err := importZedContextServers(dir, dst); return err },
	} {
		dst := filepath.Join(dir, "mcps-"+target)
		if err := os.MkdirAll(dst, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := run(dst); err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		got := readFile(t, filepath.Join(dst, "pg.yaml"))
		if strings.Contains(got, "PASSW0RD") {
			t.Errorf("%s: spec keeps a password from args", target)
		}
		if !strings.Contains(got, "postgresql://admin:${PG_PASSWORD}@db/app") {
			t.Errorf("%s: spec lacks the password reference", target)
		}
	}
}

func TestImportMCP_URLQuotedQueryCredentialLeavesServerOut(t *testing.T) {
	dir := t.TempDir()
	log := captureLog(t)
	if _, err := writeMCPYAMLs("claude", map[string]any{
		"single": map[string]any{"command": "sh", "args": []any{"-c", "exec npx mcp-remote https://x.example/mcp?token='T0KEN'"}},
		"double": map[string]any{"command": "sh", "args": []any{"-c", `exec npx mcp-remote "https://x.example/mcp?a=1&token="T0KEN`}},
		"ref":    map[string]any{"command": "sh", "args": []any{"-c", `exec npx mcp-remote https://x.example/mcp?token='${TOKEN}'&key="${KEY}"`}},
	}, dir); err != nil {
		t.Fatal(err)
	}
	out := log.String()
	if strings.Contains(out, "T0KEN") {
		t.Errorf("import output prints a credential value")
	}
	for _, name := range []string{"single", "double"} {
		if _, err := os.Stat(filepath.Join(dir, name+".yaml")); !os.IsNotExist(err) {
			t.Errorf("server %s has a quoted token and must be left out", name)
		}
		if want := "MCP server " + name + ": left out; args[1]"; !strings.Contains(out, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "ref.yaml")); err != nil {
		t.Errorf("a quoted reference is not a credential: %v", err)
	}
}

func TestImportMCP_URLWithoutCredentialImportsUnchanged(t *testing.T) {
	args := []any{
		"curl https://health.example/ping?a=1&b=2; exec srv",
		"https://x.example/mcp?upstream=https://y.example/mcp",
		"https://registry.example:8443/@scope/pkg",
		"curl 'https://x.example/mcp'?page=2",
		"--url=https://x.example/mcp?token=${TOKEN}",
	}
	specs, out := importMCPServers(t, "claude", map[string]any{
		"sh":  map[string]any{"command": "sh", "args": args},
		"api": map[string]any{"url": "https://x.example/mcp?upstream=https://y.example/mcp"},
	})
	for _, arg := range args {
		if want := "- " + arg.(string); !strings.Contains(specs["sh"], want) && !strings.Contains(specs["sh"], "- '"+arg.(string)+"'") {
			t.Errorf("spec sh lacks %q:\n%s", arg, specs["sh"])
		}
	}
	if !strings.Contains(specs["api"], "url: https://x.example/mcp?upstream=https://y.example/mcp") {
		t.Errorf("a nested URL without a credential imports unchanged:\n%s", specs["api"])
	}
	if strings.Contains(out, "left out") || strings.Contains(out, "now reads") {
		t.Errorf("import reports nothing for values without a credential:\n%s", out)
	}
}

func TestImportMCP_URLPasswordWithShellCharacters(t *testing.T) {
	specs, out := importMCPServers(t, "claude", map[string]any{
		"api": map[string]any{"url": "https://admin:PA;SS(W)D@x.example/mcp"},
		"pg":  map[string]any{"command": "pg-mcp", "args": []any{"postgresql://admin:PA;SS(W)D@db:5432/app"}},
	})
	for name, spec := range specs {
		if strings.Contains(spec, "PA;SS") {
			t.Errorf("spec %s keeps a password with shell characters", name)
		}
	}
	if strings.Contains(out, "PA;SS") {
		t.Errorf("import output prints a credential value")
	}
	for name, want := range map[string]string{
		"api": "url: https://admin:${API_PASSWORD}@x.example/mcp",
		"pg":  "- postgresql://admin:${PG_PASSWORD}@db:5432/app",
	} {
		if !strings.Contains(specs[name], want) {
			t.Errorf("spec %s lacks %q", name, want)
		}
	}
}

// The import from the issue: the specs hold references, and sync writes
// them back to the native file instead of the credential.
func TestImportMCP_URLArgsCredentialsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	captureLog(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	writeFile(t, filepath.Join(dir, ".mcp.json"), `{"mcpServers":{
  "pg":{"command":"npx","args":["-y","@modelcontextprotocol/server-postgres","postgresql://admin:PASSW0RD@db:5432/app"]},
  "api":{"url":"https://x.example/mcp?token=T0KEN"}}}`)
	execCLI(t, "import", "claude")
	execCLI(t, "sync", "-t", "claude")
	native := readFile(t, filepath.Join(dir, ".mcp.json"))
	if strings.Contains(native, "PASSW0RD") || strings.Contains(native, "T0KEN") {
		t.Fatal("sync wrote a credential back after import")
	}
	for _, want := range []string{"postgresql://admin:${PG_PASSWORD}@db:5432/app", "https://x.example/mcp?token=${API_TOKEN}"} {
		if !strings.Contains(native, want) {
			t.Errorf(".mcp.json lacks %q:\n%s", want, native)
		}
	}
}

func TestImportMCP_ReadsCrushURLArgsForms(t *testing.T) {
	specs, _ := importMCPServers(t, "crush", map[string]any{
		"gh": map[string]any{"command": "gh-mcp", "args": []any{"$GH_TOKEN", "pa55$word"}},
	})
	if !strings.Contains(specs["gh"], "- ${GH_TOKEN}") || !strings.Contains(specs["gh"], "- pa55$word") {
		t.Errorf("crush reads a whole-value $NAME argument back:\n%s", specs["gh"])
	}
}

func TestSync_URLArgsRefLeavesServerOutWithNote(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	captureLog(t)
	notes := captureNotes(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [zed]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "gh.yaml"), "name: gh\ncommand: gh-mcp\nargs: [--token, \"${GH_TOKEN}\"]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "plain.yaml"), "name: plain\ncommand: npx\nargs: [-y, server]\n")
	execCLI(t, "sync", "-t", "zed")
	settings := snapshotEmitted(t, dir)[".zed/settings.json"]
	if strings.Contains(settings, "GH_TOKEN") || strings.Contains(settings, `"gh"`) {
		t.Errorf("zed expands no reference, so gh must be left out:\n%s", settings)
	}
	if !strings.Contains(settings, `"plain"`) {
		t.Errorf("a server without a reference still emits:\n%s", settings)
	}
	if got := notes.String(); !strings.Contains(got, "`args` on 1 mcp has no effect on zed (server gh reads ${GH_TOKEN} in `args`") {
		t.Errorf("no note names the server, field, and variable:\n%s", got)
	}
}

func TestImportGlobal_CursorURLArgsRefsRoundTrip(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mcpDir := filepath.Join(source, "mcps")
	mustWriteGlobalTest(t, filepath.Join(mcpDir, "gh.yaml"), "name: gh\ncommand: gh-mcp\nargs: [--token, \"${GH_TOKEN}\", \"${workspaceFolder}\", \"${env:workspaceFolder}\"]\n")
	mustWriteGlobalTest(t, filepath.Join(mcpDir, "api.yaml"), "name: api\ntype: http\nurl: https://${API_HOST}/mcp\n")
	if _, warnings, err := runGlobalAgentTest("--only", "cursor"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	cursorPath := filepath.Join(home, ".cursor", "mcp.json")
	want := readGlobalTest(t, cursorPath)
	for _, form := range []string{`"${env:GH_TOKEN}"`, `"${workspaceFolder}"`, `"${env:workspaceFolder}"`, `"https://${env:API_HOST}/mcp"`} {
		if !strings.Contains(want, form) {
			t.Fatalf("cursor lacks %s:\n%s", form, want)
		}
	}
	if err := os.RemoveAll(mcpDir); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runImportGlobalTest(); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, filepath.Join(mcpDir, "gh.yaml")); !strings.Contains(got, "- ${GH_TOKEN}") || !strings.Contains(got, "- ${workspaceFolder}") || !strings.Contains(got, "- ${env:workspaceFolder}") {
		t.Errorf("import must read ${env:NAME} back and keep ${workspaceFolder}:\n%s", got)
	}
	if got := readGlobalTest(t, filepath.Join(mcpDir, "api.yaml")); !strings.Contains(got, "url: https://${API_HOST}/mcp") {
		t.Errorf("import must read the url reference back:\n%s", got)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "cursor"); err != nil {
		t.Fatalf("sync after import: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, cursorPath); got != want {
		t.Errorf("mcp.json changed after import:\n%s\nwant:\n%s", got, want)
	}
}
