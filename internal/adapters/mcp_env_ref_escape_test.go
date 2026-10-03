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

func escapedMCPs() []spec.Entry {
	return []spec.Entry{
		{Kind: spec.KindMCP, Name: "chroma", Path: "mcps/chroma.yaml", Meta: map[string]any{
			"command": "npx",
			"args":    []any{"-y", "mcp-remote", "https://mcp.example.com", "--header", "x-chroma-token: $${X_CHROMA_TOKEN}"},
			"env":     map[string]any{"X_CHROMA_TOKEN": "$${X_CHROMA_TOKEN}"},
		}},
		{Kind: spec.KindMCP, Name: "remote", Path: "mcps/remote.yaml", Meta: map[string]any{
			"type":    "http",
			"url":     "https://mcp.example.com/$${TENANT}/mcp",
			"headers": map[string]any{"X-Tenant": "$${TENANT}"},
		}},
	}
}

func emitEscaped(t *testing.T, target string) (string, string) {
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
	if err := EmitWithProvenance(sess, a, spec.NewBundle(escapedMCPs()), &config.Config{Targets: []string{target}}, false); err != nil {
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

func TestMCPEnvRefEscape_EveryTargetGetsTheLiteralText(t *testing.T) {
	for _, target := range []string{"warp", "zed", "cursor", "gemini", "claude", "opencode"} {
		t.Run(target, func(t *testing.T) {
			out, notes := emitEscaped(t, target)
			if !strings.Contains(out, "x-chroma-token: ${X_CHROMA_TOKEN}") {
				t.Errorf("args must carry the literal ${X_CHROMA_TOKEN}:\n%s", out)
			}
			if strings.Contains(out, "$${") || strings.Contains(out, "env:X_CHROMA_TOKEN") {
				t.Errorf("the escape must be decoded, never rewritten or copied:\n%s", out)
			}
			if strings.Contains(notes, "chroma") {
				t.Errorf("a literal is no reference to leave out:\n%s", notes)
			}
		})
	}
}

func TestMCPEnvRefEscape_DecodesURLEnvAndHeaders(t *testing.T) {
	out, notes := emitEscaped(t, "claude")
	for _, want := range []string{"https://mcp.example.com/${TENANT}/mcp", `"X-Tenant": "${TENANT}"`, `"X_CHROMA_TOKEN": "${X_CHROMA_TOKEN}"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s:\n%s", want, out)
		}
	}
	if notes != "" {
		t.Errorf("no notes expected:\n%s", notes)
	}
}

func TestMCPEnvRefEscape_CodexKeepsEscapedEnvAsText(t *testing.T) {
	out, _ := emitEscaped(t, "codex")
	if !strings.Contains(out, `X_CHROMA_TOKEN = "${X_CHROMA_TOKEN}"`) || strings.Contains(out, "env_vars") {
		t.Errorf("codex must write the literal env value, not forward it:\n%s", out)
	}
}

func TestMCPEnvRefEscape_LeftOutNoteNamesTheEscape(t *testing.T) {
	_, notes := emitEnvRefs(t, "zed")
	if !strings.Contains(notes, "$${GITHUB_TOKEN}") {
		t.Errorf("the note should say how to pass the text through:\n%s", notes)
	}
}
