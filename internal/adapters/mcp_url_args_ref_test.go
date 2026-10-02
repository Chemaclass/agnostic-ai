package adapters

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
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
	for _, e := range emit.RewriteMCPEnvRefs(target, mcps) {
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
	if _, ok := kept["gh"]; !ok {
		t.Errorf("x-zed args replace the top-level reference, so zed keeps the server:\n%s", notes)
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
