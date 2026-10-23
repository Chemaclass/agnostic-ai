package authority

import (
	"strings"
	"testing"
)

func TestBuild_CredentialsStayPrivateAcrossMatchingAndDisplay(t *testing.T) {
	const tokenA = "ghp_0123456789abcdefghijklmnop"
	const tokenB = "ghp_9876543210abcdefghijklmnop"
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":                       "version: 1\ntargets: [claude, codex, cursor, opencode]\n",
		".agnostic-ai/settings/permissions.yaml": "permissions:\n  allow:\n    - 'shell(echo " + tokenA + ")'\n    - 'shell(echo " + tokenB + ")'\n    - 'shell(curl -H \"X-Api-Key: plain-secret-value\":*)'\n",
		".agnostic-ai/agents/reader.md":          "---\nname: reader\ndescription: Reads.\ncan: [read]\n---\nRead.\n",
		".agnostic-ai/mcps/private.yaml":         "name: private\ncommand: 'env API_TOKEN=command-secret-value npx server'\n",
	})
	for _, agent := range []string{"", "reader"} {
		v := buildView(t, root, Options{AgentID: agent})
		data := mustJSON(t, v)
		for _, secret := range []string{tokenA, tokenB, strings.ReplaceAll(tokenA, "_", "-"), strings.ReplaceAll(tokenB, "_", "-"), "plain-secret-value", "command-secret-value"} {
			if strings.Contains(data, secret) {
				t.Errorf("agent %q retained %q", agent, secret)
			}
		}
		ids := map[string]bool{}
		redacted := 0
		for _, r := range v.Resources {
			if r.Kind == ResourceCommandPattern && r.Identity.Pattern != "*" {
				if ids[r.ID] {
					t.Errorf("duplicate ID %s", r.ID)
				}
				ids[r.ID] = true
				redacted++
				if agent == "" && r.Operations[0].Declared != EffectAllowed {
					t.Errorf("redaction changed matching: %+v", r.Operations[0])
				}
			}
		}
		if redacted != 3 {
			t.Errorf("redacted commands = %d", redacted)
		}
	}
	result := impactOf(t, root, Options{}, Proposal{Changes: []Change{{Op: "add", List: "deny", Rule: "Bash(rm:*)"}}})
	for _, secret := range []string{tokenA, tokenB, "plain-secret-value", "command-secret-value"} {
		if strings.Contains(mustJSON(t, result), secret) {
			t.Errorf("impact retained %q", secret)
		}
	}
	if len(result.Diff) != 1 || !strings.Contains(result.Diff[0].After, "Bash(rm:*)") {
		t.Errorf("impact diff lost safe rules: %+v", result.Diff)
	}
}

func TestBuild_CredentialResourceIDsDoNotCarryNormalizedValues(t *testing.T) {
	const token = "ghp_0123456789abcdefghijklmnop"
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":                       "version: 1\ntargets: [claude]\n",
		".agnostic-ai/settings/permissions.yaml": "permissions:\n  allow: ['read(" + token + ")']\n",
		".agnostic-ai/mcps/server.yaml":          "name: " + token + "\ntype: http\nurl: https://hooks.example.test/services/T0A1/B0C2/token123value\n",
	})
	data := mustJSON(t, buildView(t, root, Options{}))
	for _, secret := range []string{token, strings.ReplaceAll(token, "_", "-"), "T0A1", "B0C2", "token123value"} {
		if strings.Contains(data, secret) {
			t.Errorf("view retained %q", secret)
		}
	}
}

func TestBuild_RedactedSubjectIDsStayDistinctAndSelectable(t *testing.T) {
	const first = "ghp_0123456789abcdefghijklmnop"
	const second = "ghp_9876543210abcdefghijklmnop"
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":              "version: 1\ntargets: [claude]\n",
		".agnostic-ai/agents/first.md":  "---\nname: " + first + "\ndescription: First.\ncan: [read]\n---\nFirst.\n",
		".agnostic-ai/agents/second.md": "---\nname: " + second + "\ndescription: Second.\ncan: [read]\n---\nSecond.\n",
	})
	a := buildView(t, root, Options{AgentID: first})
	b := buildView(t, root, Options{AgentID: second})
	if a.Agent.ID == b.Agent.ID {
		t.Errorf("redaction merged distinct subjects: %s", a.Agent.ID)
	}
	selected := buildView(t, root, Options{AgentID: a.Agent.ID})
	if selected.Agent.ID != a.Agent.ID {
		t.Errorf("display ID did not select the original subject")
	}
	for _, v := range []*View{a, b, selected} {
		data := mustJSON(t, v)
		if strings.Contains(data, first) || strings.Contains(data, second) {
			t.Error("subject identity exposed a credential")
		}
	}
}

func TestBuild_RedactedToolOperationsKeepDistinctIDs(t *testing.T) {
	const first = "ghp_0123456789abcdefghijklmnop"
	const second = "ghp_9876543210abcdefghijklmnop"
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":                       "version: 1\ntargets: [claude]\n",
		".agnostic-ai/settings/permissions.yaml": "permissions:\n  allow: ['mcp:github/" + first + "', 'mcp:github/" + second + "']\n",
		".agnostic-ai/mcps/github.yaml":          "name: github\ncommand: npx\n",
	})
	v := buildView(t, root, Options{})
	ids := map[string]bool{}
	for _, r := range v.Resources {
		for _, op := range r.Operations {
			if ids[op.ID] {
				t.Errorf("duplicate operation ID %s", op.ID)
			}
			ids[op.ID] = true
		}
	}
	data := strings.ToLower(mustJSON(t, v))
	for _, token := range []string{first, second, strings.ReplaceAll(first, "_", " "), strings.ReplaceAll(second, "_", " ")} {
		if strings.Contains(data, token) {
			t.Errorf("tool identity exposed %q", token)
		}
	}
}
