package adapters

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func urlArgsRefMCPs() []spec.Entry {
	return []spec.Entry{
		{Kind: spec.KindMCP, Name: "api", Meta: map[string]any{
			"type": "http",
			"url":  "https://${API_HOST}/mcp",
		}},
		{Kind: spec.KindMCP, Name: "gh", Meta: map[string]any{
			"command": "gh-mcp",
			"args":    []any{"--token", "${GH_TOKEN}", "--root", "${workspaceFolder}", "--plain"},
		}},
		{Kind: spec.KindMCP, Name: "plain", Meta: map[string]any{
			"command": "npx",
			"args":    []any{"-y", "@scope/server", "--prompt=${input:id}"},
		}},
		{Kind: spec.KindMCP, Name: "remote", Meta: map[string]any{
			"type": "http",
			"url":  "https://mcp.example.com/mcp?$filter=x",
		}},
	}
}

// rewriteURLArgs runs the rewrite for target and returns the kept
// servers by name plus the flushed coverage notes.
func rewriteURLArgs(t *testing.T, target string, mcps []spec.Entry) (map[string]map[string]any, string) {
	t.Helper()
	var notes bytes.Buffer
	old := emit.Warner
	emit.Warner = &notes
	ResetCoverageNotes()
	t.Cleanup(func() { emit.Warner = old; ResetCoverageNotes() })
	kept := map[string]map[string]any{}
	a, err := Resolve(target)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range rewriteMCPRefs(a, mcps) {
		kept[e.Name] = e.Meta
	}
	FlushCoverageNotes()
	return kept, notes.String()
}

func TestMCPURLArgsRefs_EachTargetWritesItsOwnForm(t *testing.T) {
	for target, want := range map[string]struct{ url, arg string }{
		"claude":    {"https://${API_HOST}/mcp", "${GH_TOKEN}"},
		"crush":     {"https://${API_HOST}/mcp", "${GH_TOKEN}"},
		"openhands": {"https://${API_HOST}/mcp", "${GH_TOKEN}"},
		"gemini":    {"https://${API_HOST}/mcp", "${GH_TOKEN}"},
		"cursor":    {"https://${env:API_HOST}/mcp", "${env:GH_TOKEN}"},
		"windsurf":  {"https://${env:API_HOST}/mcp", "${env:GH_TOKEN}"},
		"opencode":  {"https://{env:API_HOST}/mcp", "{env:GH_TOKEN}"},
		"continue":  {"https://${{ secrets.API_HOST }}/mcp", "${{ secrets.GH_TOKEN }}"},
	} {
		t.Run(target, func(t *testing.T) {
			kept, notes := rewriteURLArgs(t, target, urlArgsRefMCPs())
			if got := kept["api"]["url"]; got != want.url {
				t.Errorf("url = %v, want %s", got, want.url)
			}
			args, _ := kept["gh"]["args"].([]any)
			if wantArgs := []any{"--token", want.arg, "--root", "${workspaceFolder}", "--plain"}; !slices.Equal(args, wantArgs) {
				t.Errorf("args = %v, want %v", args, wantArgs)
			}
			if notes != "" {
				t.Errorf("supported references should have no notes:\n%s", notes)
			}
		})
	}
}

func TestMCPURLArgsRefs_LiteralsAndOtherTokensStayAsWritten(t *testing.T) {
	for _, target := range []string{"claude", "cursor", "zed", "codex"} {
		t.Run(target, func(t *testing.T) {
			kept, _ := rewriteURLArgs(t, target, urlArgsRefMCPs())
			if got := kept["remote"]["url"]; got != "https://mcp.example.com/mcp?$filter=x" {
				t.Errorf("a literal url changed: %v", got)
			}
			args, _ := kept["plain"]["args"].([]any)
			if want := []any{"-y", "@scope/server", "--prompt=${input:id}"}; !slices.Equal(args, want) {
				t.Errorf("literal args changed: %v", args)
			}
		})
	}
}

func TestMCPURLArgsRefs_TargetWithoutFormLeavesTheServerOut(t *testing.T) {
	for _, target := range []string{"zed", "factory", "kiro", "codex", "copilot"} {
		t.Run(target, func(t *testing.T) {
			kept, notes := rewriteURLArgs(t, target, urlArgsRefMCPs())
			for _, name := range []string{"api", "gh"} {
				if _, ok := kept[name]; ok {
					t.Errorf("server %s must be left out, not written with a literal reference: %v", name, kept[name])
				}
			}
			for _, name := range []string{"plain", "remote"} {
				if _, ok := kept[name]; !ok {
					t.Errorf("server %s has no reference and must still emit", name)
				}
			}
			for _, want := range []string{
				"`url`", "server api reads ${API_HOST} in `url`",
				"`args`", "server gh reads ${GH_TOKEN} in `args`",
				"sync leaves the server out",
			} {
				if !strings.Contains(notes, want) {
					t.Errorf("notes missing %q:\n%s", want, notes)
				}
			}
		})
	}
}

func TestMCPURLArgsRefs_AmpWritesURLAndLeavesArgsOut(t *testing.T) {
	kept, notes := rewriteURLArgs(t, "amp", urlArgsRefMCPs())
	if got := kept["api"]["url"]; got != "https://${API_HOST}/mcp" {
		t.Errorf("amp expands url references: %v", got)
	}
	if _, ok := kept["gh"]; ok {
		t.Errorf("amp documents no args reference, so gh must be left out")
	}
	if !strings.Contains(notes, "server gh reads ${GH_TOKEN} in `args`") {
		t.Errorf("no note for the left-out server:\n%s", notes)
	}
}

func TestMCPURLArgsRefs_DefaultOnlyWhereDocumented(t *testing.T) {
	mcps := []spec.Entry{{Kind: spec.KindMCP, Name: "api", Meta: map[string]any{
		"type": "http",
		"url":  "${API_BASE:-https://secret.example.com}/mcp",
	}}}
	for _, target := range []string{"claude", "crush", "openhands"} {
		kept, _ := rewriteURLArgs(t, target, mcps)
		if got := kept["api"]["url"]; got != "${API_BASE:-https://secret.example.com}/mcp" {
			t.Errorf("%s documents defaults: url = %v", target, got)
		}
	}
	for _, target := range []string{"cursor", "gemini", "amp"} {
		kept, notes := rewriteURLArgs(t, target, mcps)
		if _, ok := kept["api"]; ok {
			t.Errorf("%s documents no default, so the server must be left out", target)
		}
		if !strings.Contains(notes, "${API_BASE:-...}") || strings.Contains(notes, "secret.example.com") {
			t.Errorf("%s note must name the reference without its default:\n%s", target, notes)
		}
	}
}

func TestMCPURLArgsRefs_TargetBlockOverridesWithoutMutatingTheSpec(t *testing.T) {
	mcps := []spec.Entry{{Kind: spec.KindMCP, Name: "gh", Meta: map[string]any{
		"command":  "gh-mcp",
		"args":     []any{"${TOP}"},
		"x-cursor": map[string]any{"args": []any{"--token", "${GH_TOKEN}"}},
		"x-zed":    map[string]any{"args": []any{"--plain"}},
	}}}
	kept, _ := rewriteURLArgs(t, "cursor", mcps)
	x, _ := kept["gh"]["x-cursor"].(map[string]any)
	if args, _ := x["args"].([]any); !slices.Equal(args, []any{"--token", "${env:GH_TOKEN}"}) {
		t.Errorf("x-cursor args = %v", x["args"])
	}
	if args := mcps[0].Meta["x-cursor"].(map[string]any)["args"].([]any); args[1] != "${GH_TOKEN}" {
		t.Errorf("the spec's meta was mutated: %v", args)
	}
	if args := mcps[0].Meta["args"].([]any); args[0] != "${TOP}" {
		t.Errorf("the spec's top-level args were mutated: %v", args)
	}
	kept, notes := rewriteURLArgs(t, "zed", mcps)
	if _, ok := kept["gh"]; ok {
		t.Errorf("zed writes the top-level args and ignores x-zed.args, so ${TOP} must leave the server out")
	}
	if !strings.Contains(notes, "server gh reads ${TOP} in `args`") {
		t.Errorf("no note for the left-out server:\n%s", notes)
	}
}

func TestMCPURLArgsRefs_ReadEachTargetFormBack(t *testing.T) {
	for target, native := range map[string]struct{ url, arg string }{
		"claude":   {"https://${API_HOST}/mcp", "${GH_TOKEN}"},
		"crush":    {"https://${API_HOST}/mcp", "$GH_TOKEN"},
		"gemini":   {"https://${API_HOST}/mcp", "$GH_TOKEN"},
		"cursor":   {"https://${env:API_HOST}/mcp", "${env:GH_TOKEN}"},
		"windsurf": {"https://${env:API_HOST}/mcp", "${env:GH_TOKEN}"},
		"opencode": {"https://{env:API_HOST}/mcp", "{env:GH_TOKEN}"},
		"continue": {"https://${{ secrets.API_HOST }}/mcp", "${{ secrets.GH_TOKEN }}"},
		"amp":      {"https://${API_HOST}/mcp", "${GH_TOKEN}"},
	} {
		t.Run(target, func(t *testing.T) {
			server := map[string]any{
				"url":  native.url,
				"args": []any{"--token", native.arg, "${workspaceFolder}", "pa55$word", 3},
			}
			ReadMCPEnvRefs(target, server)
			if got := server["url"]; got != "https://${API_HOST}/mcp" {
				t.Errorf("url = %v", got)
			}
			if want := []any{"--token", "${GH_TOKEN}", "${workspaceFolder}", "pa55$word", 3}; !slices.Equal(server["args"].([]any), want) {
				t.Errorf("args = %v, want %v", server["args"], want)
			}
		})
	}
}

func TestMCPURLArgsRefs_ReadKeepsTextATargetDoesNotExpand(t *testing.T) {
	server := map[string]any{"url": "https://${env:HOST}/mcp", "args": []any{"$TOKEN", "{env:X}"}}
	ReadMCPEnvRefs("zed", server)
	if server["url"] != "https://${env:HOST}/mcp" || !slices.Equal(server["args"].([]any), []any{"$TOKEN", "{env:X}"}) {
		t.Errorf("zed expands nothing, so import keeps its text: %v", server)
	}
}

// emitMCPEntries emits mcps to target and returns every written file
// joined, plus the coverage notes.
func emitMCPEntries(t *testing.T, target string, mcps []spec.Entry) (string, string) {
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
	if err := EmitWithProvenance(sess, a, spec.NewBundle(mcps), &config.Config{Targets: []string{target}}, false); err != nil {
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

func mcpEntry(name string, meta map[string]any) []spec.Entry {
	return []spec.Entry{{Kind: spec.KindMCP, Name: name, Path: "mcps/" + name + ".yaml", Meta: meta}}
}

func TestMCPURLArgsRefs_EmitChecksTheArgsEachWriterWrites(t *testing.T) {
	for _, tc := range []struct {
		name, target string
		meta         map[string]any
		kept         bool
		want, absent string
	}{
		{"zed writes top-level args over x-zed", "zed", map[string]any{
			"command": "gh-mcp", "args": []any{"${TOP}"}, "x-zed": map[string]any{"args": []any{"--plain"}},
		}, false, "", "${TOP}"},
		{"zed ignores a reference only in x-zed", "zed", map[string]any{
			"command": "gh-mcp", "args": []any{"--plain"}, "x-zed": map[string]any{"args": []any{"${X_ONLY}"}},
		}, true, "--plain", "X_ONLY"},
		{"codex writes top-level args only", "codex", map[string]any{
			"command": "gh-mcp", "args": []any{"--plain"}, "x-codex": map[string]any{"args": []any{"${X_ONLY}"}},
		}, true, "--plain", "X_ONLY"},
		{"kilo resolves x-kilo over top-level args", "kilo", map[string]any{
			"command": "gh-mcp", "args": []any{"--plain"}, "x-kilo": map[string]any{"args": []any{"${X_ONLY}"}},
		}, false, "", "X_ONLY"},
		{"opencode copies x-opencode args as written", "opencode", map[string]any{
			"command": "gh-mcp", "args": []any{"--plain"}, "x-opencode": map[string]any{"args": []any{"${X_ONLY}"}},
		}, true, "{env:X_ONLY}", "${X_ONLY}"},
		{"antigravity copies x-antigravity url as written", "antigravity", map[string]any{
			"type": "http", "url": "https://mcp.example.com/mcp", "x-antigravity": map[string]any{"url": "https://${X_ONLY}/mcp"},
		}, false, "", "X_ONLY"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, notes := emitMCPEntries(t, tc.target, mcpEntry("gh", tc.meta))
			if tc.absent != "" && strings.Contains(out, tc.absent) {
				t.Errorf("%s must not reach %s:\n%s", tc.absent, tc.target, out)
			}
			if tc.want != "" && !strings.Contains(out, tc.want) {
				t.Errorf("missing %s in:\n%s", tc.want, out)
			}
			if dropped := strings.Contains(notes, "sync leaves the server out"); dropped == tc.kept {
				t.Errorf("kept = %v, but notes say:\n%s", tc.kept, notes)
			}
		})
	}
}

func TestMCPURLArgsRefs_EmitChecksOnlyTheEffectiveTransport(t *testing.T) {
	t.Run("factory stdio override ignores the inherited url", func(t *testing.T) {
		out, notes := emitMCPEntries(t, "factory", mcpEntry("api", map[string]any{
			"type": "http", "url": "https://${HOST}/mcp",
			"x-factory": map[string]any{"type": "stdio", "command": "run-api", "args": []any{"--plain"}},
		}))
		if !strings.Contains(out, "run-api") || strings.Contains(out, "HOST") {
			t.Errorf("factory writes the stdio override and no url:\n%s", out)
		}
		if strings.Contains(notes, "sync leaves the server out") {
			t.Errorf("an unused url must not leave the server out:\n%s", notes)
		}
	})
	t.Run("codex http server ignores unused args", func(t *testing.T) {
		out, notes := emitMCPEntries(t, "codex", mcpEntry("api", map[string]any{
			"type": "http", "url": "https://api.example.com/mcp", "args": []any{"${UNUSED}"},
			"headers": map[string]any{"Authorization": "Bearer ${API_KEY}"},
		}))
		if !strings.Contains(out, `bearer_token_env_var = "API_KEY"`) || strings.Contains(out, "UNUSED") {
			t.Errorf("codex forwards the header and writes no args:\n%s", out)
		}
		if strings.Contains(notes, "sync leaves the server out") {
			t.Errorf("unused args must not leave the server out:\n%s", notes)
		}
	})
	t.Run("qoder streamable-http url is checked", func(t *testing.T) {
		out, notes := emitMCPEntries(t, "qoder", mcpEntry("api", map[string]any{
			"type": "streamable-http", "url": "https://${HOST}/mcp",
		}))
		if strings.Contains(out, "${HOST}") || !strings.Contains(notes, "sync leaves the server out") {
			t.Errorf("qoder has no url form, so the server is left out:\n%s\n%s", out, notes)
		}
	})
	t.Run("continue streamable-http url default is checked", func(t *testing.T) {
		out, notes := emitMCPEntries(t, "continue", mcpEntry("api", map[string]any{
			"type": "streamable-http", "url": "https://${HOST:-example.com}/mcp",
		}))
		if strings.Contains(out, "HOST") || !strings.Contains(notes, "sync leaves the server out") {
			t.Errorf("continue documents no default, so the server is left out:\n%s\n%s", out, notes)
		}
	})
	t.Run("zed stdio server ignores an unused url", func(t *testing.T) {
		out, notes := emitMCPEntries(t, "zed", mcpEntry("gh", map[string]any{
			"command": "gh-mcp", "url": "https://${HOST}/mcp",
		}))
		if !strings.Contains(out, "gh-mcp") || strings.Contains(notes, "sync leaves the server out") {
			t.Errorf("zed keeps a stdio server whose url it never writes:\n%s\n%s", out, notes)
		}
	})
}

// An explicit native env reference named like an editor variable must
// come back the same way, not as the editor variable.
func TestMCPURLArgsRefs_EditorVariableNamesRoundTrip(t *testing.T) {
	for _, name := range []string{"workspaceFolder", "workspaceFolderBasename", "userHome", "pathSeparator"} {
		for target, native := range map[string]string{
			"cursor":   "${env:" + name + "}",
			"windsurf": "${env:" + name + "}",
			"opencode": "{env:" + name + "}",
			"continue": "${{ secrets." + name + " }}",
			"crush":    "$" + name,
			"gemini":   "$" + name,
			"claude":   "${" + name + "}",
		} {
			t.Run(target+"/"+name, func(t *testing.T) {
				server := map[string]any{"type": "http", "url": "https://h/" + native, "args": []any{native}}
				ReadMCPEnvRefs(target, server)
				a, err := Resolve(target)
				if err != nil {
					t.Fatal(err)
				}
				kept, notes := rewriteURLArgsMeta(t, a, server)
				if kept == nil {
					t.Fatalf("server left out:\n%s", notes)
				}
				if got := kept["url"]; got != "https://h/"+native {
					t.Errorf("url = %v, want https://h/%s", got, native)
				}
				if got := kept["args"].([]any); got[0] != native {
					t.Errorf("args = %v, want %s", got, native)
				}
			})
		}
	}
}

func rewriteURLArgsMeta(t *testing.T, a Adapter, meta map[string]any) (map[string]any, string) {
	t.Helper()
	var notes bytes.Buffer
	old := emit.Warner
	emit.Warner = &notes
	ResetCoverageNotes()
	t.Cleanup(func() { emit.Warner = old; ResetCoverageNotes() })
	got := rewriteMCPRefs(a, mcpEntry("s", meta))
	FlushCoverageNotes()
	if len(got) == 0 {
		return nil, notes.String()
	}
	return got[0].Meta, notes.String()
}
