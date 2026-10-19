package authority

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuild_SecretsCarryRightsButNeverValues(t *testing.T) {
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml": "version: 1\ntargets: [claude]\n",
		".agnostic-ai/mcps/github.yaml": `name: github
type: http
url: https://ci:supersecretpassword@api.example.com/mcp?token=abc123token&mode=${MODE}
headers:
  Authorization: Bearer ${GITHUB_TOKEN}
  X-Debug: !literal "Bearer secret"
`,
		".agnostic-ai/agents/offline.md": "---\nname: offline\ndescription: No servers.\nmcpServers: []\n---\n\nWork offline.\n",
	})
	v := buildView(t, root, Options{})

	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"Bearer secret", "supersecretpassword", "abc123token"} {
		if strings.Contains(string(data), value) {
			t.Errorf("the view carries the value %q", value)
		}
	}
	var token *Secret
	for i := range v.Secrets {
		if v.Secrets[i].Name == "GITHUB_TOKEN" {
			token = &v.Secrets[i]
		}
	}
	if token == nil {
		t.Fatalf("secrets = %+v, want GITHUB_TOKEN", v.Secrets)
	}
	if token.Label != "GITHUB_TOKEN (github credential)" || token.ReferencedBy[0].Field != "headers.Authorization" {
		t.Errorf("secret = %+v", token)
	}
	// No rule names a github tool, so its use is left to the tool's
	// default and the right is unknown rather than claimed.
	want := SecretRights{Discover: "allowed", ReadValue: "unknown", Use: "unknown", Manage: "unknown"}
	if token.Rights != want {
		t.Errorf("rights = %+v, want %+v", token.Rights, want)
	}
	if !strings.Contains(token.Explanation, "depends on the Filesystem & Execution rules") {
		t.Errorf("explanation = %q", token.Explanation)
	}
	if len(uncertaintiesWith(v, UncertaintySecretReadUnknown)) != 1 {
		t.Error("no secret-read-unknown uncertainty")
	}

	offline := buildView(t, root, Options{AgentID: "offline"})
	for _, s := range offline.Secrets {
		if s.Rights.Use != "unavailable" {
			t.Errorf("%s use = %s for an agent that reaches no server", s.Name, s.Rights.Use)
		}
	}
}

func TestBuild_ReferencedEnvironmentValueNeverReachesTheViewOrTheImpact(t *testing.T) {
	const marker = "marker-7d1e9c-value-that-must-stay-out"
	t.Setenv("AUTHORITY_MARKER", marker)
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml": "version: 1\ntargets: [claude, codex]\n",
		".agnostic-ai/mcps/vendor.yaml": `name: vendor
type: http
url: https://mcp.vendor.example/mcp
headers:
  X-Api-Key: ${AUTHORITY_MARKER}
`,
		".agnostic-ai/settings/permissions.yaml": "permissions:\n  allow: [mcp:vendor/read_report]\n",
	})
	cfg, b := loadProject(t, root)
	opts := Options{Root: root, Now: fixedNow}
	v, err := Build(cfg, b, opts)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Impact(cfg, b, opts, Proposal{Changes: []Change{{Op: "add", List: "deny", Rule: "mcp:vendor/delete_report"}}})
	if err != nil {
		t.Fatal(err)
	}
	for name, doc := range map[string]any{"view": v, "impact": res} {
		data, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), marker) {
			t.Errorf("the %s carries the referenced value", name)
		}
	}
	if !strings.Contains(mustJSON(t, v), "AUTHORITY_MARKER") {
		t.Error("the view does not name the referenced variable")
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestRedactURL_KeepsReferencesAndHidesWrittenCredentials(t *testing.T) {
	for in, want := range map[string]string{
		"https://api.githubcopilot.com/mcp/":                   "https://api.githubcopilot.com/mcp/",
		"https://${HOST}/mcp?key=${KEY}":                       "https://${HOST}/mcp?key=${KEY}",
		"https://u:pw@db.example/x?token=t1&mode=a":            "https://<redacted>@db.example/x?token=<redacted>&mode=<redacted>",
		"https://${USER}:${PASS}@db.example/x":                 "https://${USER}:${PASS}@db.example/x",
		"https://example.com/mcp#access_token=a.b.c":           "https://example.com/mcp#access_token=<redacted>",
		"https://hooks.slack.com/services/T0A1/B0C2/xoxb9Zq":   "https://hooks.slack.com/services/<redacted>/<redacted>/<redacted>",
		"https://api.telegram.org/bot123:abc/sendMessage":      "https://api.telegram.org/<redacted>/sendMessage",
		"https://mcp.example.com/v1/sse/sk-live-4f9a/${TOKEN}": "https://mcp.example.com/v1/sse/<redacted>/${TOKEN}",
	} {
		if got := redactURL(in); got != want {
			t.Errorf("redactURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuild_CredentialUseFollowsTheSubjectsToolPermissions(t *testing.T) {
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":                       "version: 1\ntargets: [claude, codex]\n",
		".agnostic-ai/settings/permissions.yaml": "permissions:\n  allow:\n    - mcp:github/get_pull_request\n",
		".agnostic-ai/mcps/github.yaml":          "name: github\ntype: http\nurl: https://api.example.com/mcp\nheaders:\n  Authorization: Bearer ${GITHUB_TOKEN}\n",
		".agnostic-ai/agents/reader.md":          "---\nname: reader\ndescription: Reads files only.\ncan: [read]\n---\n\nRead.\n",
	})
	useOf := func(v *View) string {
		t.Helper()
		for _, s := range v.Secrets {
			if s.Name == "GITHUB_TOKEN" {
				return s.Rights.Use
			}
		}
		t.Fatal("no GITHUB_TOKEN secret")
		return ""
	}
	if got := useOf(buildView(t, root, Options{Target: "claude"})); got != "allowed" {
		t.Errorf("session on claude: use = %s, want allowed: a github tool is permitted", got)
	}
	if got := useOf(buildView(t, root, Options{AgentID: "reader", Target: "claude"})); got != "unavailable" {
		t.Errorf("reader on claude: use = %s, want unavailable: its can list leaves every github tool out", got)
	}
	if got := useOf(buildView(t, root, Options{Target: "codex"})); got != "unknown" {
		t.Errorf("session on codex: use = %s, want unknown: the rule is not expressed there", got)
	}
}

func TestBuild_AServerCommandThatHoldsACredentialIsRedacted(t *testing.T) {
	const token = "ghp_0123456789abcdefghijklmnop"
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":              "version: 1\ntargets: [claude]\n",
		".agnostic-ai/mcps/github.yaml": "name: github\ncommand: \"env GITHUB_TOKEN=" + token + " npx -y server\"\n",
		".agnostic-ai/mcps/plain.yaml":  "name: plain\ncommand: npx\nargs: [-y, server]\n",
	})
	v := buildView(t, root, Options{})
	if data := mustJSON(t, v); strings.Contains(data, token) {
		t.Fatal("the view carries the token written in a server command")
	}
	endpoints := map[string]string{}
	for _, p := range v.Providers {
		if p.Transport != nil {
			endpoints[p.ID] = p.Transport.Endpoint
		}
	}
	if endpoints["mcp-github"] != "<redacted>" || endpoints["mcp-plain"] != "npx" {
		t.Errorf("endpoints = %v", endpoints)
	}
}
