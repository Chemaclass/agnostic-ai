package preview

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

func TestDisplay_StructuredCredentialsAreHiddenAndSyntaxRemainsValid(t *testing.T) {
	for _, c := range []struct{ name, path, body, format string }{
		{"json custom extension", "custom.txt", `{"env":{"OPAQUE":"PREVIEW_SECRET_A"},"headers":{"Authorization":"Bearer PREVIEW_SECRET_B"},"apiKeyHelper":"/tools/get-key","model":"visible"}`, "json"},
		{"jsonc", "settings.jsonc", "// Native settings\n{\"password\":\"PREVIEW_SECRET_A\",\"model\":\"visible\",}\n", "json"},
		{"yaml flow and alias", "custom.txt", "defaults: &credential PREVIEW_SECRET_A\nheaders: {Authorization: *credential}\nmodel: visible\n", "yaml"},
		{"yaml multiline", "native.yaml", "password: |\n  PREVIEW_SECRET_A\n  PREVIEW_SECRET_B\nmodel: visible\n", "yaml"},
		{"yaml quoted native keys", "custom.txt", "\"password\": PREVIEW_SECRET_A\nmodel: visible\n", "yaml"},
		{"yaml sequence", "custom.txt", "- password: PREVIEW_SECRET_A\n  model: visible\n", "yaml"},
		{"toml inline", "custom.txt", "model = 'visible'\n[mcp_servers.example]\nenv = { OPAQUE = 'PREVIEW_SECRET_A' }\n", "toml"},
		{"toml array tables", "custom.txt", "model = 'visible'\n[[servers]]\npassword = 'PREVIEW_SECRET_A'\n", "toml"},
		{"toml multiline", "config.toml", "model = 'visible'\npassword = '''PREVIEW_SECRET_A\nPREVIEW_SECRET_B'''\n", "toml"},
		{"arguments", "native.json", `{"args":["-y","server","--token","PREVIEW_SECRET_A","https://user:PREVIEW_SECRET_B@example.test"],"model":"visible"}`, "json"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := Display(c.path, c.body)
			if !got.Hidden || strings.Contains(got.Text, "PREVIEW_SECRET_") {
				t.Error("structured display failed to protect a literal credential")
			}
			if got.Withheld {
				return
			}
			if !strings.Contains(got.Text, "visible") {
				t.Error("safe adjacent value was lost")
			}
			switch c.format {
			case "json":
				if !json.Valid([]byte(got.Text)) {
					t.Error("sanitized JSON is invalid")
				}
			case "yaml":
				var value any
				if yaml.Unmarshal([]byte(got.Text), &value) != nil {
					t.Error("sanitized YAML is invalid")
				}
			case "toml":
				var value map[string]any
				if _, err := toml.Decode(got.Text, &value); err != nil {
					t.Error("sanitized TOML is invalid")
				}
			}
		})
	}
}

func TestDisplay_PureReferencesStayAndMixedLiteralsDisappear(t *testing.T) {
	body := `{"env":{"canonical":"${TOKEN}","native":"${env:TOKEN}","opencode":"{env:TOKEN}","default":"${TOKEN:-PREVIEW_SECRET_A}","mixed":"PREVIEW_SECRET_B-${SUFFIX}"},"headers":{"Authorization":"Bearer ${TOKEN}"}}`
	got := Display("custom.txt", body)
	if strings.Contains(got.Text, "PREVIEW_SECRET_") || got.Withheld {
		t.Error("reference display failed")
	}
	for _, ref := range []string{"${TOKEN}", "${env:TOKEN}", "{env:TOKEN}", "Bearer ${TOKEN}"} {
		if !strings.Contains(got.Text, ref) {
			t.Errorf("pure reference was hidden: %s", ref)
		}
	}
}

func TestDisplay_OrdinaryBytesAndHelperPathsAreUnchanged(t *testing.T) {
	for _, body := range []string{
		"# Rule\n\nChoose a password manager. Token counts are metadata.\n",
		"---\nname: guide\ndescription: Token naming conventions\n---\n\nOrdinary skill steps.\n",
		"{ \"apiKeyHelper\": \"/tools/get-key\", \"model\": \"visible\" }\n",
		"#!/bin/sh\n/tools/get-key\necho ready\n",
	} {
		got := Display("custom.txt", body)
		if got.Text != body || got.Hidden || got.Withheld {
			t.Error("credential-free display changed original bytes")
		}
	}
}

func TestDisplay_KnownSensitiveParseFailuresWithholdContent(t *testing.T) {
	for _, c := range []struct{ path, body string }{
		{"custom.txt", `{ "password": "PREVIEW_SECRET_A"`},
		{"config.toml", "password = \"PREVIEW_SECRET_A\n"},
		{"native.yaml", "headers: {Authorization: PREVIEW_SECRET_A\n"},
	} {
		got := Display(c.path, c.body)
		if !got.Withheld || !got.Hidden || strings.Contains(got.Text, "PREVIEW_SECRET_") {
			t.Error("sensitive parse failure exposed the original body")
		}
	}
}

func TestDisplay_GeneratedShellCredentialsAndMarkdownFrontmatterAreProtected(t *testing.T) {
	for _, c := range []struct{ path, body string }{
		{"custom.txt", "#!/bin/sh\nTOKEN=PREVIEW_SECRET_A server --password PREVIEW_SECRET_B\n"},
		{"hook.sh", "#!/bin/sh\nserver --token \"$(printf PREVIEW_SECRET_A)\"\n"},
		{"agent.md", "---\nenv:\n  OPAQUE: PREVIEW_SECRET_A\n---\n\nOrdinary agent prose.\n"},
		{"agent.md", "# Setup\n\n```json\n{\"password\":\"PREVIEW_SECRET_A\"}\n```\n"},
	} {
		got := Display(c.path, c.body)
		if !got.Hidden || strings.Contains(got.Text, "PREVIEW_SECRET_") {
			t.Error("known command or structured Markdown credential leaked")
		}
	}
}

func TestDisplay_ReviewCredentialBoundaries(t *testing.T) {
	for _, c := range []struct{ name, path, body string }{
		{"yaml leading document marker", "native.yaml", "---\nmodel: visible\n---\npassword: PREVIEW_SECRET_A\n"},
		{"URL reference host", "native.json", `{"url":"https://${HOST}/mcp?api_key=PREVIEW_SECRET_A"}`},
		{"URL reference base", "native.json", `{"url":"${API_BASE}/mcp?password=PREVIEW_SECRET_A"}`},
		{"URL malformed query", "native.json", `{"url":"https://example.test/mcp?api_key=PREVIEW_SECRET_A%ZZ"}`},
		{"yaml later password", "native.yaml", "model: visible\n---\npassword: PREVIEW_SECRET_A\n"},
		{"yaml later opaque env", "native.yaml", "model: visible\n---\nenv: {OPAQUE: PREVIEW_SECRET_A}\n"},
		{"frontmatter CRLF", "agent.md", "---\r\npassword: PREVIEW_SECRET_A\r\n---\r\nprose\r\n"},
		{"frontmatter EOF", "agent.md", "---\npassword: PREVIEW_SECRET_A\n---"},
		{"frontmatter incomplete", "agent.md", "---\nenv: {OPAQUE: PREVIEW_SECRET_A}\n"},
		{"longer fence", "agent.md", "````yaml\nmodel: visible\n```\npassword: PREVIEW_SECRET_A\n````\n"},
		{"unclosed fence", "agent.md", "# Setup\n```json\n{\"password\":\"PREVIEW_SECRET_A\"}"},
		{"mixed flag argument", "native.json", `{"args":["--token","PREVIEW_SECRET_A:${SUFFIX}"]}`},
		{"mixed shell flag", "hook.sh", "server --token 'PREVIEW_SECRET_A:${SUFFIX}'\n"},
		{"inline Cookie header", "native.json", `{"args":["--header=Cookie: PREVIEW_SECRET_A"]}`},
		{"inline opaque header", "native.json", `{"args":["--header=X-Opaque: PREVIEW_SECRET_A"]}`},
		{"inline shell header", "hook.sh", "server '--header=X-Api-Key: PREVIEW_SECRET_A'\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := Display(c.path, c.body)
			if !got.Hidden || strings.Contains(got.Text, "PREVIEW_SECRET_") {
				t.Error("recognized credential boundary exposed a literal value")
			}
		})
	}
}

func TestDisplay_ContinueNativeReferenceRemainsVisible(t *testing.T) {
	body := `{"env":{"OPAQUE":"${{ secrets.TOKEN }}"},"headers":{"Authorization":"Bearer ${{ secrets.TOKEN }}"}}`
	got := Display("native.json", body)
	if got.Text != body || got.Hidden {
		t.Error("Continue native reference was hidden")
	}
}

func TestDisplay_ReviewPureArgumentAndURLControlsStayVisible(t *testing.T) {
	for _, body := range []string{
		`{"args":["--token","${TOKEN}","--header=X-Opaque: ${TOKEN}"]}`,
		`{"url":"https://${HOST}/mcp?api_key=${TOKEN}&mode=ordinary"}`,
		`{"url":"${API_BASE}/mcp?password=${TOKEN}&mode=ordinary"}`,
		`{"url":"https://example.test/mcp?mode=ordinary"}`,
		"model: visible\n---\nmodel: still-visible\n",
	} {
		got := Display("custom.txt", body)
		if got.Hidden || got.Text != body {
			t.Error("safe reference or ordinary query/document changed")
		}
	}
}

func TestDisplay_CustomSuffixYAMLDocumentStreamIsProtected(t *testing.T) {
	got := Display("native.txt", "---\nmodel: visible\n---\npassword: PREVIEW_SECRET_A\n")
	if strings.Contains(got.Text, "PREVIEW_SECRET_") || !got.Hidden {
		t.Error("custom-suffix YAML document stream leaked")
	}
}

func TestDisplay_NativeURLAndAttachedShortHeaders(t *testing.T) {
	for _, c := range []struct{ path, body string }{
		{"native.json", `{"url":"{env:API_BASE}/mcp?password=PREVIEW_SECRET_A"}`},
		{"native.json", `{"url":"$API_BASE/mcp?password=PREVIEW_SECRET_A"}`},
		{"native.json", `{"url":"%API_BASE%/mcp#api_key=PREVIEW_SECRET_A"}`},
		{"native.json", `{"url":"${{ secrets.API_BASE }}/mcp?password=PREVIEW_SECRET_A"}`},
		{"native.json", `{"args":["-HX-Opaque: PREVIEW_SECRET_A"]}`},
		{"hook.sh", "curl '-HX-Opaque: PREVIEW_SECRET_A'\n"},
	} {
		got := Display(c.path, c.body)
		if !got.Hidden || strings.Contains(got.Text, "PREVIEW_SECRET_") {
			t.Error("native URL or attached short header leaked")
		}
	}
	for _, body := range []string{
		`{"url":"{env:API_BASE}/mcp?password={env:TOKEN}"}`,
		`{"url":"$API_BASE/mcp?password=$TOKEN"}`,
		`{"url":"%API_BASE%/mcp#api_key=%TOKEN%"}`,
		`{"url":"${{ secrets.API_BASE }}/mcp?password=${{ secrets.TOKEN }}"}`,
		`{"args":["-HX-Opaque: ${TOKEN}"]}`,
	} {
		got := Display("native.json", body)
		if got.Hidden || got.Text != body {
			t.Error("pure native reference changed")
		}
	}
}

func TestDisplay_CommandArraysAndEnvCommandAssignments(t *testing.T) {
	for _, c := range []struct{ path, body string }{
		{"native.json", `{"command":["server","--token","PREVIEW_SECRET_A"]}`},
		{"hook.sh", "env TOKEN=PREVIEW_SECRET_A server\n"},
	} {
		got := Display(c.path, c.body)
		if !got.Hidden || strings.Contains(got.Text, "PREVIEW_SECRET_") {
			t.Error("command array or env command assignment leaked")
		}
	}
	for _, c := range []struct{ path, body string }{
		{"native.json", `{"command":["server","--token","${TOKEN}"]}`},
		{"hook.sh", "env TOKEN='${TOKEN}' MODE=visible server\n"},
		{"hook.sh", "env MODE=visible server\n"},
	} {
		got := Display(c.path, c.body)
		if got.Hidden || got.Text != c.body {
			t.Error("pure command reference or ordinary assignment changed")
		}
	}
}

func TestDisplay_QuotedEnvAssignmentsInCommandStrings(t *testing.T) {
	for _, body := range []string{
		`{"command":"env 'TOKEN=PREVIEW_SECRET_A' server"}`,
		`{"command":"env \"TOKEN=PREVIEW_SECRET_A\" server"}`,
		`{"command":"env TO'KEN'=PREVIEW_SECRET_A server"}`,
	} {
		got := Display("native.json", body)
		if !got.Hidden || strings.Contains(got.Text, "PREVIEW_SECRET_") {
			t.Error("quoted credential assignment leaked through command string")
		}
	}
	for _, body := range []string{
		`{"command":"env 'TOKEN=${TOKEN}' server"}`,
		`{"command":"env \"TOKEN=${TOKEN}\" server"}`,
		`{"command":"env MODE=visible server"}`,
	} {
		got := Display("native.json", body)
		if got.Hidden || got.Text != body {
			t.Error("safe quoted reference or ordinary assignment changed")
		}
	}
}

func TestDisplay_PunctuationOnlyValuesRemainVisible(t *testing.T) {
	for _, body := range []string{`{"model":"?"}`, `{"model":"#"}`, `{"model":"?#"}`, `{"url":"https://example.test/?#"}`} {
		got := Display("native.json", body)
		if got.Text != body || got.Hidden {
			t.Error("punctuation-only ordinary value changed")
		}
	}
}

func TestDisplay_ReferenceExecutablesAndRelativeURLs(t *testing.T) {
	for _, body := range []string{
		`{"command":"${BIN}/server --password PREVIEW_SECRET_A"}`,
		`{"command":"$BIN/server --password PREVIEW_SECRET_A"}`,
		`{"command":"${BASE}/mcp?mode=ordinary --password PREVIEW_SECRET_A"}`,
		`{"url":"#api_key=PREVIEW_SECRET_A"}`,
		`{"url":"?password=PREVIEW_SECRET_A"}`,
	} {
		got := Display("native.json", body)
		if !got.Hidden || strings.Contains(got.Text, "PREVIEW_SECRET_") {
			t.Error("reference executable or relative URL leaked")
		}
	}
	for _, body := range []string{
		`{"url":"${{ secrets.BASE }}/mcp?password=${{ secrets.TOKEN }}"}`,
		`{"url":"#api_key=${TOKEN}"}`,
		`{"url":"?password=${TOKEN}"}`,
		`{"model":"?"}`,
		`{"model":"#"}`,
	} {
		got := Display("native.json", body)
		if got.Hidden || got.Text != body {
			t.Error("pure relative URL or punctuation changed")
		}
	}
}

func TestDisplay_AssignmentStatementsAreNotMistakenForURLs(t *testing.T) {
	for _, body := range []string{
		`{"command":"TOKEN=PREVIEW_SECRET_A;#comment"}`,
		`{"command":"TOKEN=PREVIEW_SECRET_A?"}`,
		`{"command":"TOKEN=PREVIEW_SECRET_A;URL=https://example.test"}`,
	} {
		got := Display("native.json", body)
		if !got.Hidden || strings.Contains(got.Text, "PREVIEW_SECRET_") {
			t.Error("shell assignment was mistaken for URL content")
		}
	}
}

func TestDisplay_NativeExecutionKeysNeverUseRelativeURLShortcut(t *testing.T) {
	for _, key := range []string{"bash", "powershell", "commandWindows", "apiKeyHelper"} {
		body := `{"` + key + `":"/bin/true;TOKEN=PREVIEW_SECRET_A?"}`
		got := Display("native.json", body)
		if !got.Hidden || strings.Contains(got.Text, "PREVIEW_SECRET_") {
			t.Error("native execution field used the URL shortcut")
		}
	}
	for _, body := range []string{
		`{"unknown":"/bin/true;TOKEN=PREVIEW_SECRET_A?"}`,
		`{"unknown":"/bin/true;TOKEN=PREVIEW_SECRET_A;URL=https://example.test"}`,
	} {
		got := Display("native.json", body)
		if !got.Hidden || strings.Contains(got.Text, "PREVIEW_SECRET_") {
			t.Error("slash-prefixed command leaked through generic scalar")
		}
	}
}

func TestDisplay_ShellBodiesInArgumentArraysAreProtected(t *testing.T) {
	for _, body := range []string{
		`{"args":["-c","/bin/true;TOKEN=PREVIEW_SECRET_A"]}`,
		`{"command":["bash","-c","/bin/true;TOKEN=PREVIEW_SECRET_A"]}`,
		`{"args":["${BIN}/server;TOKEN=PREVIEW_SECRET_A"]}`,
	} {
		got := Display("native.json", body)
		if !got.Hidden || strings.Contains(got.Text, "PREVIEW_SECRET_") {
			t.Error("shell statement in argument array leaked")
		}
	}
}

func TestDisplay_ReferenceURLFallbackRejectsSubstitutionParts(t *testing.T) {
	for _, body := range []string{
		`{"args":["${{ secrets.BIN }}/$(TOKEN=PREVIEW_SECRET_A)"]}`,
		`{"args":["${{ secrets.BIN }}/<(TOKEN=PREVIEW_SECRET_A)"]}`,
	} {
		got := Display("native.json", body)
		if !got.Hidden || strings.Contains(got.Text, "PREVIEW_SECRET_") {
			t.Error("URL fallback allowed executable word parts")
		}
	}
	body := `{"args":["${{ secrets.BASE }}/mcp?password=${{ secrets.TOKEN }}"]}`
	got := Display("native.json", body)
	if got.Hidden || got.Text != body {
		t.Error("pure Continue URL argument changed")
	}
}

func TestDisplay_EnvironmentArraysKeepInheritedSensitivity(t *testing.T) {
	body := `{"env":{"args":["PREVIEW_SECRET_2021_OPAQUE"],"command":["PREVIEW_SECRET_2021_COMMAND"],"referenced":["${TOKEN}"]},"model":"visible"}`
	shown := Display("settings.json", body)
	if strings.Contains(shown.Text, "PREVIEW_SECRET_2021_") {
		t.Error("environment arrays lost their sensitive parent context")
	}
	if !json.Valid([]byte(shown.Text)) {
		t.Fatal("rewritten environment arrays are not valid JSON")
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(shown.Text), &doc); err != nil {
		t.Fatal(err)
	}
	env := doc["env"].(map[string]any)
	if env["referenced"].([]any)[0] != "${TOKEN}" || doc["model"] != "visible" {
		t.Error("pure environment reference or ordinary model changed")
	}
}

func TestDisplay_HelperLiteralArgumentsAreHidden(t *testing.T) {
	for _, helper := range []string{"printf '%s' 'PREVIEW_SECRET_HELPER'", "echo PREVIEW_SECRET_HELPER", "/tools/get-key PREVIEW_SECRET_HELPER", "printf 'PREVIEW_SECRET_HELPER'", "echo 'PREVIEW_SECRET_HELPER", "$(printf PREVIEW_SECRET_HELPER)", "cat <<EOF\nPREVIEW_SECRET_HELPER\nEOF"} {
		body, err := json.Marshal(map[string]string{"apiKeyHelper": helper, "model": "visible"})
		if err != nil {
			t.Fatal(err)
		}
		got := Display(".claude/settings.json", string(body))
		if !got.Hidden || strings.Contains(got.Text, "PREVIEW_SECRET_HELPER") {
			t.Errorf("helper literal remains visible: %q", helper)
		}
	}
}

func TestDisplay_HelperPathsAndReferencesRemainVisible(t *testing.T) {
	for _, helper := range []string{"/tools/get-key", "'/tools/get key'", "${HELPER}", "/tools/get-key ${TOKEN}", `/tools/get-key "${TOKEN}"`} {
		body, err := json.Marshal(map[string]string{"apiKeyHelper": helper, "model": "visible"})
		if err != nil {
			t.Fatal(err)
		}
		got := Display(".claude/settings.json", string(body))
		if got.Hidden || got.Text != string(body) {
			t.Errorf("safe helper changed: %q", helper)
		}
	}
}
