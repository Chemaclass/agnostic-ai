package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportMCP_CredentialShapesBecomeReferences(t *testing.T) {
	for name, tc := range map[string]struct {
		server map[string]any
		secret string
		spec   []string
		output []string
	}{
		"flag with a separate value": {
			server: map[string]any{"command": "gh-mcp", "args": []any{"--api-key", "K3Y-SPACE", "--port", "8080"}},
			secret: "K3Y-SPACE",
			spec:   []string{"- --api-key\n", "- ${GH_API_KEY}\n", "- \"8080\""},
			output: []string{"MCP server gh: args[1] api-key now reads ${GH_API_KEY}; set GH_API_KEY"},
		},
		"flag with an equals value": {
			server: map[string]any{"command": "gh-mcp", "args": []any{"--password=PW-EQUALS"}},
			secret: "PW-EQUALS",
			spec:   []string{"- --password=${GH_PASSWORD}"},
			output: []string{"MCP server gh: args[0] password now reads ${GH_PASSWORD}; set GH_PASSWORD"},
		},
		"bare tokens": {
			server: map[string]any{"command": "gh-mcp", "args": []any{"ghp_AAAAbbbbCCCC1111", "xoxb-1234567890-abcdef"}},
			secret: "AAAAbbbbCCCC1111",
			spec:   []string{"- ${GH_TOKEN}\n", "- ${GH_TOKEN_2}\n"},
			output: []string{"MCP server gh: args[0] token now reads ${GH_TOKEN}; set GH_TOKEN", "MCP server gh: args[1] token now reads ${GH_TOKEN_2}; set GH_TOKEN_2"},
		},
		"token username with a password": {
			server: map[string]any{"command": "git-mcp", "args": []any{"git+https://ghp_AAAAbbbbCCCC2222:x-oauth-basic@github.com/o/r.git"}},
			secret: "AAAAbbbbCCCC2222",
			spec:   []string{"- git+https://${GH_TOKEN}:${GH_PASSWORD}@github.com/o/r.git"},
			output: []string{"MCP server gh: args[0] token now reads ${GH_TOKEN}; set GH_TOKEN"},
		},
		"token username alone": {
			server: map[string]any{"url": "https://ghp_AAAAbbbbCCCC3333@github.com/mcp"},
			secret: "AAAAbbbbCCCC3333",
			spec:   []string{"url: https://${GH_TOKEN}@github.com/mcp"},
			output: []string{"MCP server gh: url token now reads ${GH_TOKEN}; set GH_TOKEN"},
		},
		"fragment parameter": {
			server: map[string]any{"url": "https://x.example/cb#access_token=FR4GMENT&state=1"},
			secret: "FR4GMENT",
			spec:   []string{"url: https://x.example/cb#access_token=${GH_ACCESS_TOKEN}&state=1"},
			output: []string{"MCP server gh: url access_token now reads ${GH_ACCESS_TOKEN}; set GH_ACCESS_TOKEN"},
		},
		"query-only credential words": {
			server: map[string]any{"url": "https://x.example/mcp?sig=S1GNED&code=C0DEVAL&page=2"},
			secret: "S1GNED",
			spec:   []string{"url: https://x.example/mcp?sig=${GH_SIG}&code=${GH_CODE}&page=2"},
			output: []string{"MCP server gh: url sig now reads ${GH_SIG}; set GH_SIG", "MCP server gh: url code now reads ${GH_CODE}; set GH_CODE"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			specs, out := importMCPServers(t, "claude", map[string]any{"gh": tc.server})
			if strings.Contains(specs["gh"], tc.secret) || strings.Contains(out, tc.secret) {
				t.Errorf("the credential reached the spec or the output:\n%s\n%s", specs["gh"], out)
			}
			for _, want := range tc.spec {
				if !strings.Contains(specs["gh"], want) {
					t.Errorf("spec lacks %q:\n%s", want, specs["gh"])
				}
			}
			for _, want := range tc.output {
				if !strings.Contains(out, want) {
					t.Errorf("output lacks %q:\n%s", want, out)
				}
			}
		})
	}
}

func TestImportMCP_CredentialShapesLeaveServerOut(t *testing.T) {
	for name, tc := range map[string]struct {
		server        map[string]any
		field, secret string
	}{
		"token inside a command string": {map[string]any{"command": "sh", "args": []any{"-c", "exec srv --auth ghp_AAAAbbbbCCCC4444"}}, "args[1]", "AAAAbbbbCCCC4444"},
		"token flag inside a command":   {map[string]any{"command": "sh", "args": []any{"-c", "exec srv --api-key=FLAGINSH"}}, "args[1]", "FLAGINSH"},
		"scheme-less user and password": {map[string]any{"command": "mysql-mcp", "args": []any{"root:S3CRETPW@db:3306"}}, "args[0]", "S3CRETPW"},
		"scp-style target":              {map[string]any{"command": "sync-mcp", "args": []any{"deploy:SCPPASS@host:/srv"}}, "args[0]", "SCPPASS"},
		"assignment in a command":       {map[string]any{"command": "sh", "args": []any{"-c", "API_TOKEN=ASSIGNED exec srv"}}, "args[1]", "ASSIGNED"},
		"whole assignment argument":     {map[string]any{"command": "docker", "args": []any{"run", "-e", "GITHUB_TOKEN=DOCKERTOK", "img"}}, "args[2]", "DOCKERTOK"},
		"digit password before a hash":  {map[string]any{"url": "redis://:1234#5@cache:6379"}, "url", "1234#5"},
		"digit password before a slash": {map[string]any{"command": "r", "args": []any{"redis://:1234/5@cache:6379"}}, "args[0]", "1234/5"},
		"percent-encoded separators":    {map[string]any{"url": "https://x.example/mcp?next=%3Ftoken%3DENC0DED"}, "url", "ENC0DED"},
		"command field":                 {map[string]any{"command": "sh -c 'TOKEN=CMDSECRET srv'"}, "command", "CMDSECRET"},
		"cwd field":                     {map[string]any{"command": "srv", "cwd": "/home/u/ghp_AAAAbbbbCCCC5555"}, "cwd", "AAAAbbbbCCCC5555"},
		"target block":                  {map[string]any{"command": "srv", "x-claude": map[string]any{"env": map[string]any{"T": "ghp_AAAAbbbbCCCC6666"}}}, "x-claude.env.T", "AAAAbbbbCCCC6666"},
		"request options":               {map[string]any{"url": "https://h/mcp", "requestOptions": map[string]any{"proxy": "http://u:PR0XYPW@proxy:8080"}}, "requestOptions.proxy", "PR0XYPW"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			log := captureLog(t)
			if _, err := writeMCPYAMLs("claude", map[string]any{"gh": tc.server, "good": map[string]any{"command": "y"}}, dir); err != nil {
				t.Fatal(err)
			}
			out := log.String()
			if strings.Contains(out, tc.secret) {
				t.Errorf("import output prints a credential value:\n%s", out)
			}
			if _, err := os.Stat(filepath.Join(dir, "gh.yaml")); !os.IsNotExist(err) {
				t.Errorf("a server with a credential import cannot rewrite must be left out")
			}
			if want := "MCP server gh: left out; " + tc.field + " holds a credential import cannot rewrite"; !strings.Contains(out, want) {
				t.Errorf("output lacks %q:\n%s", want, out)
			}
			if _, err := os.Stat(filepath.Join(dir, "good.yaml")); err != nil {
				t.Errorf("a server without a credential is written: %v", err)
			}
		})
	}
}

func TestImportMCP_PlainArgumentsImportUnchanged(t *testing.T) {
	args := []any{
		"--port", "8080",
		"--token-file", "./t",
		"--key-id", "5",
		"-p", "3000",
		"-pS3CRET",
		"--token", "$GH_TOKEN",
		"--verbose",
		"git@github.com:o/r.git",
		"https://git@github.com/o/r.git",
		"node:20@sha256:abc123",
		"sk-learn-mcp-server-tools-extra",
		"GITHUB_TOKEN=$GITHUB_TOKEN exec srv",
		"--token=${GH_TOKEN}",
		"https://x.example/cb#section",
	}
	specs, out := importMCPServers(t, "claude", map[string]any{
		"plain": map[string]any{"command": "srv", "cwd": "/srv/app", "args": args, "x-claude": map[string]any{"alwaysLoad": true}},
	})
	for _, want := range []string{
		"- --port\n", "- \"8080\"", "- --token-file\n", "- ./t\n", "- --key-id\n", "- -p\n", "- \"3000\"", "- -pS3CRET\n",
		"- $GH_TOKEN\n", "- git@github.com:o/r.git", "- https://git@github.com/o/r.git", "- node:20@sha256:abc123",
		"- sk-learn-mcp-server-tools-extra", "- GITHUB_TOKEN=$GITHUB_TOKEN exec srv", "- --token=${GH_TOKEN}", "- https://x.example/cb#section",
	} {
		if !strings.Contains(specs["plain"], want) {
			t.Errorf("spec lacks %q:\n%s", want, specs["plain"])
		}
	}
	if strings.Contains(out, "left out") || strings.Contains(out, "now reads") {
		t.Errorf("import reports nothing for arguments without a credential:\n%s", out)
	}
}

func TestImportFromContinue_RequestOptionsCredentialLeavesServerOut(t *testing.T) {
	dir := t.TempDir()
	log := captureLog(t)
	mcpDir := filepath.Join(dir, ".continue", "mcpServers")
	writeFile(t, filepath.Join(mcpDir, "proxied.yaml"), "name: proxied\nmcpServers:\n  - name: proxied\n    type: streamable-http\n    url: https://h/mcp\n    requestOptions:\n      proxy: http://u:PR0XYPW@proxy:8080\n      headers:\n        A: b\n")
	writeFile(t, filepath.Join(mcpDir, "plain.yaml"), "name: plain\nmcpServers:\n  - name: plain\n    type: streamable-http\n    url: https://h/mcp\n    requestOptions:\n      timeout: 30\n")
	if err := importFromContinue(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	out := log.String()
	if strings.Contains(out, "PR0XYPW") {
		t.Errorf("import output prints a credential value:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "mcps", "proxied.yaml")); !os.IsNotExist(err) {
		t.Errorf("a requestOptions credential must leave the server out")
	}
	if want := "MCP server proxied: left out; requestOptions.proxy holds a credential import cannot rewrite"; !strings.Contains(out, want) {
		t.Errorf("output lacks %q:\n%s", want, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "mcps", "plain.yaml")); err != nil {
		t.Errorf("requestOptions without a credential imports: %v", err)
	}
}

func TestImportGlobal_LeavesOutServersWithCredentials(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(home, ".cursor", "mcp.json"), `{"mcpServers": {
  "flag": {"command": "x", "args": ["--api-key", "GL0BALKEY"]},
  "env": {"command": "x", "env": {"NODE_ENV": "production", "GITHUB_TOKEN": "GL0BALENV"}},
  "auth": {"url": "https://h/mcp", "headers": {"Authorization": "Bearer GL0BALHDR"}},
  "plainenv": {"command": "x", "env": {"NODE_ENV": "production"}},
  "plainhdr": {"url": "https://h/mcp", "headers": {"X-Region": "eu"}},
  "shell": {"command": "sh", "args": ["-c", "TOKEN=GL0BALSH exec srv"]},
  "ok": {"command": "y", "args": ["--port", "8080"]}}}`+"\n")
	out, warnings, err := runImportGlobalTest("cursor")
	if err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	for _, secret := range []string{"GL0BALKEY", "GL0BALSH", "GL0BALENV", "GL0BALHDR"} {
		if strings.Contains(out+warnings, secret) {
			t.Errorf("import --global prints a credential value:\n%s%s", out, warnings)
		}
	}
	for name, field := range map[string]string{"flag": "args[1]", "env": "env.GITHUB_TOKEN", "auth": "headers.Authorization", "shell": "args[1]"} {
		if _, err := os.Stat(filepath.Join(source, "mcps", name+".yaml")); !os.IsNotExist(err) {
			t.Errorf("server %s holds a literal credential and must stay out of the home", name)
		}
		if want := "skipped MCP server " + name + ": left out; " + field + " holds a credential import cannot rewrite; add it under local/mcps"; !strings.Contains(warnings, want) {
			t.Errorf("warnings lack %q:\n%s", want, warnings)
		}
	}
	for _, name := range []string{"ok", "plainenv", "plainhdr"} {
		if _, err := os.Stat(filepath.Join(source, "mcps", name+".yaml")); err != nil {
			t.Errorf("server %s has no credential and is imported: %v\n%s", name, err, warnings)
		}
	}
}

func TestImportMCP_ReviewedCredentialShapesLeaveServerOut(t *testing.T) {
	for name, tc := range map[string]struct {
		server        map[string]any
		field, secret string
	}{
		"bearer in a header argument":    {map[string]any{"command": "mcp-remote", "args": []any{"https://h/mcp", "--header", "Authorization:Bearer BEARERARG1"}}, "args[2]", "BEARERARG1"},
		"bearer inside a command":        {map[string]any{"command": "sh", "args": []any{"-c", "curl -H 'Authorization: Bearer BEARERSH12' https://h"}}, "args[1]", "BEARERSH12"},
		"bearer in a JSON value":         {map[string]any{"command": "srv", "x-claude": map[string]any{"headers": `{"Authorization": "Bearer ntn_NOTIONSECRET"}`}}, "x-claude.headers", "NOTIONSECRET"},
		"credential header after -H":     {map[string]any{"command": "mcp-remote", "args": []any{"-H", "X-Api-Key: HDRKEYVAL1"}}, "args[1]", "HDRKEYVAL1"},
		"cookie header with equals":      {map[string]any{"command": "mcp-remote", "args": []any{"--header=Cookie: sid=COOKIEVAL1"}}, "args[0]", "COOKIEVAL1"},
		"credential key in a block":      {map[string]any{"command": "srv", "x-claude": map[string]any{"settings": map[string]any{"apiKey": "XKEYSECRET"}}}, "x-claude.settings.apiKey", "XKEYSECRET"},
		"credential key in options":      {map[string]any{"url": "https://h/mcp", "requestOptions": map[string]any{"clientSecret": "ROPTSECRET"}}, "requestOptions.clientSecret", "ROPTSECRET"},
		"flag among shell words":         {map[string]any{"command": "sh", "args": []any{"-c", "npx srv --token abcdefgh123"}}, "args[1]", "abcdefgh123"},
		"glued credential word in sh -c": {map[string]any{"command": "sh", "args": []any{"-c", "PGPASSWORD=GLUEDPW1 exec pg-mcp"}}, "args[1]", "GLUEDPW1"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			log := captureLog(t)
			if _, err := writeMCPYAMLs("claude", map[string]any{"gh": tc.server}, dir); err != nil {
				t.Fatal(err)
			}
			out := log.String()
			if strings.Contains(out, tc.secret) {
				t.Errorf("import output prints a credential value:\n%s", out)
			}
			if _, err := os.Stat(filepath.Join(dir, "gh.yaml")); !os.IsNotExist(err) {
				t.Errorf("a server with a credential import cannot rewrite must be left out")
			}
			if want := "MCP server gh: left out; " + tc.field + " holds a credential import cannot rewrite"; !strings.Contains(out, want) {
				t.Errorf("output lacks %q:\n%s", want, out)
			}
		})
	}
}

func TestImportMCP_MoreTokenPrefixesBecomeReferences(t *testing.T) {
	tokens := []string{
		"npm_" + strings.Repeat("a1", 18),
		"AIza" + strings.Repeat("B", 35),
		"sk_live_ABCDEFGH1",
		"sk_test_ABCDEFGH2",
		"rk_live_ABCDEFGH3",
		"xapp-1-ABCDEFGH4",
		"sk-proj-abcdefghij",
		"sk-ant-abcdefghij",
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJl",
	}
	args := []any{"--private-key", "KEYMATERIAL1"}
	for _, token := range tokens {
		args = append(args, token)
	}
	specs, out := importMCPServers(t, "claude", map[string]any{"gh": map[string]any{"command": "srv", "args": args}})
	for _, secret := range append(tokens, "KEYMATERIAL1") {
		if strings.Contains(specs["gh"], secret) || strings.Contains(out, secret) {
			t.Errorf("token %.12s reached the spec or the output", secret)
		}
	}
	for _, want := range []string{"- ${GH_PRIVATE_KEY}\n", "- ${GH_TOKEN}\n", "- ${GH_TOKEN_9}\n"} {
		if !strings.Contains(specs["gh"], want) {
			t.Errorf("spec lacks %q:\n%s", want, specs["gh"])
		}
	}
	if !strings.Contains(out, "MCP server gh: args[1] private-key now reads ${GH_PRIVATE_KEY}; set GH_PRIVATE_KEY") {
		t.Errorf("output does not name the flag reference:\n%s", out)
	}
}

func TestImportMCP_NonCredentialNamesAndValuesImportUnchanged(t *testing.T) {
	url := "https://x.example/mcp?country_code=US&language_code=en&sortKey=a&primary_key=id&cache_key=x&sort_key=y#code_view=1"
	args := []any{
		"--sort-key", "name",
		"--cache-key", "abcdefghij",
		"--partition-key", "pk-value-long",
		"--idempotency-key", "1234abcd-x",
		"--public-key", "ssh-ed25519AAAA",
		"--require-api-key", "run",
		"--api-key", "/run/secrets/key",
		"--token=true",
		"--password", "12345678",
		"--token", "~/t",
		"--secret", "../s",
		"--api-key", "short",
		"--header", "Authorization: Bearer ${TOKEN}",
		"-H", "X-Region: eu",
	}
	specs, out := importMCPServers(t, "claude", map[string]any{
		"api":   map[string]any{"url": url},
		"plain": map[string]any{"command": "srv", "args": args, "x-claude": map[string]any{"settings": map[string]any{"sortKey": "name", "token": "${TOKEN}"}}},
	})
	if !strings.Contains(specs["api"], "url: "+url) {
		t.Errorf("a parameter name that only contains a credential word imports unchanged:\n%s", specs["api"])
	}
	for _, arg := range args {
		if !strings.Contains(specs["plain"], arg.(string)) {
			t.Errorf("spec lacks %q:\n%s", arg, specs["plain"])
		}
	}
	if strings.Contains(out, "left out") || strings.Contains(out, "now reads") {
		t.Errorf("import reports nothing for values without a credential:\n%s", out)
	}
}

func TestImportGlobal_CredentialKeysAndReferenceValues(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(home, ".cursor", "mcp.json"), `{"mcpServers": {
  "pg": {"command": "x", "env": {"PGPASSWORD": "GL0BALPGPW"}},
  "notion": {"command": "x", "env": {"OPENAPI_MCP_HEADERS": "{\"Authorization\": \"Bearer ntn_GL0BALNOTION\"}"}},
  "ref": {"url": "https://h/mcp", "headers": {"Authorization": "Bearer ${env:GH_TOKEN}", "X-Token": "token ${env:X_TOKEN}"}},
  "paths": {"command": "x", "env": {"PWD": "/home/u", "PORT": "8080", "NODE_ENV": "production"}}}}`+"\n")
	out, warnings, err := runImportGlobalTest("cursor")
	if err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	for _, secret := range []string{"GL0BALPGPW", "GL0BALNOTION"} {
		if strings.Contains(out+warnings, secret) {
			t.Errorf("import --global prints a credential value:\n%s%s", out, warnings)
		}
	}
	for name, field := range map[string]string{"pg": "env.PGPASSWORD", "notion": "env.OPENAPI_MCP_HEADERS"} {
		if _, err := os.Stat(filepath.Join(source, "mcps", name+".yaml")); !os.IsNotExist(err) {
			t.Errorf("server %s holds a literal credential and must stay out of the home", name)
		}
		if want := "skipped MCP server " + name + ": left out; " + field + " holds a credential"; !strings.Contains(warnings, want) {
			t.Errorf("warnings lack %q:\n%s", want, warnings)
		}
	}
	for _, name := range []string{"ref", "paths"} {
		if _, err := os.Stat(filepath.Join(source, "mcps", name+".yaml")); err != nil {
			t.Errorf("server %s holds no literal credential and is imported: %v\n%s", name, err, warnings)
		}
	}
}

func TestImportMCP_FinalCredentialShapesBecomeReferences(t *testing.T) {
	secrets := []string{"BEARERFLAG1", "OPENAIKEYVAL", "abc"}
	specs, out := importMCPServers(t, "claude", map[string]any{
		"gh": map[string]any{"command": "srv", "args": []any{"--bearer", "BEARERFLAG1", "--openai-key", "OPENAIKEYVAL", "--token", "abc"}},
	})
	for _, secret := range secrets {
		if strings.Contains(specs["gh"], "- "+secret+"\n") || strings.Contains(out, secret) {
			t.Errorf("%s reached the spec or the output:\n%s\n%s", secret, specs["gh"], out)
		}
	}
	for _, want := range []string{"- ${GH_BEARER}\n", "- ${GH_OPENAI_KEY}\n", "- ${GH_TOKEN}\n"} {
		if !strings.Contains(specs["gh"], want) {
			t.Errorf("spec lacks %q:\n%s", want, specs["gh"])
		}
	}
	for _, want := range []string{
		"MCP server gh: args[1] bearer now reads ${GH_BEARER}; set GH_BEARER",
		"MCP server gh: args[3] openai-key now reads ${GH_OPENAI_KEY}; set GH_OPENAI_KEY",
		"MCP server gh: args[5] token now reads ${GH_TOKEN}; set GH_TOKEN",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestImportMCP_FinalCredentialShapesLeaveServerOut(t *testing.T) {
	for name, tc := range map[string]struct {
		server        map[string]any
		field, secret string
	}{
		"auth key in a block":          {map[string]any{"command": "srv", "x-claude": map[string]any{"X_AUTH": "AUTHVALUE1"}}, "x-claude.X_AUTH", "AUTHVALUE1"},
		"cred key in a block":          {map[string]any{"command": "srv", "x-claude": map[string]any{"MY_CRED": "CREDVALUE1"}}, "x-claude.MY_CRED", "CREDVALUE1"},
		"vendor key in a block":        {map[string]any{"command": "srv", "x-claude": map[string]any{"ANTHROPIC_KEY": "ANTHKEYVAL"}}, "x-claude.ANTHROPIC_KEY", "ANTHKEYVAL"},
		"short token flag in a string": {map[string]any{"command": "sh", "args": []any{"-c", "npx srv --token abc"}}, "args[1]", "--token abc"},
		"header line in a string":      {map[string]any{"command": "srv", "x-claude": map[string]any{"headers": "Accept: json\nX-Api-Key: HDRLINE123"}}, "x-claude.headers", "HDRLINE123"},
		"JSON pair in a string":        {map[string]any{"command": "srv", "x-claude": map[string]any{"config": `{"apiKey":"JSONKEYVAL"}`}}, "x-claude.config", "JSONKEYVAL"},
		"short bearer with a digit":    {map[string]any{"command": "sh", "args": []any{"-c", "curl -H 'Authorization: Bearer ab1' https://h"}}, "args[1]", "ab1"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			log := captureLog(t)
			if _, err := writeMCPYAMLs("claude", map[string]any{"gh": tc.server}, dir); err != nil {
				t.Fatal(err)
			}
			out := log.String()
			if strings.Contains(out, tc.secret) {
				t.Errorf("import output prints a credential value:\n%s", out)
			}
			if _, err := os.Stat(filepath.Join(dir, "gh.yaml")); !os.IsNotExist(err) {
				t.Errorf("a server with a credential import cannot rewrite must be left out")
			}
			if want := "MCP server gh: left out; " + tc.field + " holds a credential import cannot rewrite"; !strings.Contains(out, want) {
				t.Errorf("output lacks %q:\n%s", want, out)
			}
		})
	}
}

func TestImportMCP_FinalNonCredentialValuesImportUnchanged(t *testing.T) {
	args := []any{
		"--auth", "oauth",
		"--ssh-key", "mykeyname123",
		"--public-key", "pubkeyvalue1",
		"--token", "config.json",
		"--password", `C:\secrets\pw.txt`,
		"--token", "@token-file",
		"--secret", "mcp-server-postgres",
		"--no-password", "something123",
		"--transport-token", "streamable-http",
		"sk-learn-mcp-server-tools-2",
		`C:\Users\me@corp\a.db`,
	}
	specs, out := importMCPServers(t, "claude", map[string]any{
		"plain": map[string]any{"command": "srv", "args": args, "x-claude": map[string]any{"description": "Uses bearer authentication", "sort_key": "name"}},
	})
	for _, arg := range args {
		if !strings.Contains(specs["plain"], arg.(string)) {
			t.Errorf("spec lacks %q:\n%s", arg, specs["plain"])
		}
	}
	if strings.Contains(out, "left out") || strings.Contains(out, "now reads") {
		t.Errorf("import reports nothing for values without a credential:\n%s", out)
	}
}

func TestImportGlobal_FinalCredentialKeys(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(home, ".cursor", "mcp.json"), `{"mcpServers": {
  "openai": {"command": "x", "env": {"OPENAI_KEY": "GL0BALOAI1"}},
  "auth": {"command": "x", "env": {"X_AUTH": "GL0BALAUTH"}},
  "headers": {"command": "x", "env": {"MCP_HEADERS": "X-Api-Key: GL0BALHDR1"}},
  "sorted": {"command": "x", "env": {"SORT_KEY": "name", "MODE": "streamable-http"}}}}`+"\n")
	out, warnings, err := runImportGlobalTest("cursor")
	if err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	for _, secret := range []string{"GL0BALOAI1", "GL0BALAUTH", "GL0BALHDR1"} {
		if strings.Contains(out+warnings, secret) {
			t.Errorf("import --global prints a credential value:\n%s%s", out, warnings)
		}
	}
	for name, field := range map[string]string{"openai": "env.OPENAI_KEY", "auth": "env.X_AUTH", "headers": "env.MCP_HEADERS"} {
		if _, err := os.Stat(filepath.Join(source, "mcps", name+".yaml")); !os.IsNotExist(err) {
			t.Errorf("server %s holds a literal credential and must stay out of the home", name)
		}
		if want := "skipped MCP server " + name + ": left out; " + field + " holds a credential"; !strings.Contains(warnings, want) {
			t.Errorf("warnings lack %q:\n%s", want, warnings)
		}
	}
	if _, err := os.Stat(filepath.Join(source, "mcps", "sorted.yaml")); err != nil {
		t.Errorf("a server without a credential is imported: %v\n%s", err, warnings)
	}
}
