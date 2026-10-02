package adapters

import (
	"bytes"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func envRefMCPs() []spec.Entry {
	return []spec.Entry{
		{Kind: spec.KindMCP, Name: "gh", Path: "mcps/gh.yaml", Meta: map[string]any{
			"command": "gh-mcp",
			"env":     map[string]any{"GITHUB_TOKEN": "${GITHUB_TOKEN}", "GH_HOST": "${HOST}", "DEBUG": "1"},
		}},
		{Kind: spec.KindMCP, Name: "api", Path: "mcps/api.yaml", Meta: map[string]any{
			"type": "http",
			"url":  "https://api.example.com/mcp",
			"headers": map[string]any{
				"Authorization": "Bearer ${API_KEY}",
				"X-Team":        "${TEAM_ID}",
				"X-Mixed":       "team-${TEAM_ID}",
			},
		}},
	}
}

// emitEnvRefs emits envRefMCPs to target and returns every written file
// joined, plus the coverage notes.
func emitEnvRefs(t *testing.T, target string) (string, string) {
	t.Helper()
	testutil.TempCwd(t)
	var notes bytes.Buffer
	old := emit.Warner
	emit.Warner = &notes
	ResetCoverageNotes()
	t.Cleanup(func() { emit.Warner = old; ResetCoverageNotes() })
	a, err := Resolve(target)
	if err != nil {
		t.Fatal(err)
	}
	sess := NewSession()
	sess.StartCapture()
	if err := EmitWithProvenance(sess, a, spec.NewBundle(envRefMCPs()), &config.Config{Targets: []string{target}}, false); err != nil {
		sess.StopCapture()
		t.Fatalf("%s: %v", target, err)
	}
	var out strings.Builder
	for _, f := range sess.StopCapture() {
		out.WriteString(f.Content)
	}
	FlushCoverageNotes()
	return out.String(), notes.String()
}

func TestMCPEnvRefs_EachTargetWritesItsOwnForm(t *testing.T) {
	for target, wants := range map[string][]string{
		"claude":   {`"GITHUB_TOKEN": "${GITHUB_TOKEN}"`, `"Authorization": "Bearer ${API_KEY}"`, `"X-Mixed": "team-${TEAM_ID}"`},
		"factory":  {`"GITHUB_TOKEN": "${GITHUB_TOKEN}"`, `"Authorization": "Bearer ${API_KEY}"`},
		"cursor":   {`"GITHUB_TOKEN": "${env:GITHUB_TOKEN}"`, `"Authorization": "Bearer ${env:API_KEY}"`, `"X-Mixed": "team-${env:TEAM_ID}"`},
		"windsurf": {`"GITHUB_TOKEN": "${env:GITHUB_TOKEN}"`, `"Authorization": "Bearer ${env:API_KEY}"`},
		"opencode": {`"GITHUB_TOKEN": "{env:GITHUB_TOKEN}"`, `"Authorization": "Bearer {env:API_KEY}"`},
	} {
		t.Run(target, func(t *testing.T) {
			out, _ := emitEnvRefs(t, target)
			for _, want := range append(wants, `"DEBUG": "1"`) {
				if !strings.Contains(out, want) {
					t.Errorf("missing %s in:\n%s", want, out)
				}
			}
		})
	}
}

func TestMCPEnvRefs_CodexForwardsNames(t *testing.T) {
	out, notes := emitEnvRefs(t, "codex")
	for _, want := range []string{
		`env_vars = ["GITHUB_TOKEN"]`,
		`bearer_token_env_var = "API_KEY"`,
		`env_http_headers = { X-Team = "TEAM_ID" }`,
		`DEBUG = "1"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"${", "GH_HOST", "X-Mixed"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("codex cannot read %s:\n%s", unwanted, out)
		}
	}
	for _, want := range []string{"`env.GH_HOST`", "server gh reads ${HOST}", "`headers.X-Mixed`", "server api reads ${TEAM_ID}"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes missing %q:\n%s", want, notes)
		}
	}
}

func TestMCPEnvRefs_TargetWithoutFormDropsTheKey(t *testing.T) {
	out, notes := emitEnvRefs(t, "zed")
	if strings.Contains(out, "${") || strings.Contains(out, "GITHUB_TOKEN") || strings.Contains(out, "Authorization") {
		t.Errorf("zed must not receive a reference it never expands:\n%s", out)
	}
	if !strings.Contains(out, `"DEBUG": "1"`) {
		t.Errorf("a literal value still emits:\n%s", out)
	}
	for _, want := range []string{"`env.GITHUB_TOKEN`", "zed", "server gh reads ${GITHUB_TOKEN}", "`headers.Authorization`", "server api reads ${API_KEY}"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes missing %q:\n%s", want, notes)
		}
	}
}

func TestMCPEnvRefs_GeminiKeepsEnvAndDropsHeaders(t *testing.T) {
	out, notes := emitEnvRefs(t, "gemini")
	if !strings.Contains(out, `"GITHUB_TOKEN": "${GITHUB_TOKEN}"`) {
		t.Errorf("gemini expands env references:\n%s", out)
	}
	if strings.Contains(out, "API_KEY") {
		t.Errorf("gemini documents no header expansion:\n%s", out)
	}
	if !strings.Contains(notes, "`headers.Authorization`") {
		t.Errorf("no note for the dropped header:\n%s", notes)
	}
}

func TestMCPEnvRefs_RewritesTheTargetBlockWithoutMutatingTheSpec(t *testing.T) {
	mcps := []spec.Entry{{Kind: spec.KindMCP, Name: "gh", Meta: map[string]any{
		"command": "gh-mcp",
		"x-cursor": map[string]any{
			"env": map[string]any{"GITHUB_TOKEN": "${GITHUB_TOKEN}"},
		},
	}}}
	got := emit.RewriteMCPEnvRefs("cursor", mcps)
	block, _ := got[0].Meta["x-cursor"].(map[string]any)
	env, _ := block["env"].(map[string]any)
	if env["GITHUB_TOKEN"] != "${env:GITHUB_TOKEN}" {
		t.Errorf("x-cursor env = %v", env)
	}
	original := mcps[0].Meta["x-cursor"].(map[string]any)["env"].(map[string]any)
	if original["GITHUB_TOKEN"] != "${GITHUB_TOKEN}" {
		t.Errorf("the spec's meta was mutated: %v", original)
	}
}
