package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestCollapseWebPairs_PreservesSurroundingText(t *testing.T) {
	for before, want := range map[string]string{
		"tools: [Read, WebFetch, WebSearch] # keep\nother: ok\n": "tools: [Read, web] # keep\nother: ok\n",
		"tools:\n  - WebFetch\n  - WebSearch\n  - Grep\n":        "tools:\n  - web\n  - Grep\n",
		"tools: [WebSearch, WebFetch]\n":                         "tools: [WebSearch, WebFetch]\n",
	} {
		got, err := collapseWebPairs(before, []string{"tools"})
		if err != nil || got != want {
			t.Errorf("rewrite = %q, %v; want %q", got, err, want)
		}
	}
}

func TestImportNeutralToolNames_ChangesOnlyExactMappings(t *testing.T) {
	input := "---\nname: a\ntools: [Read(src/**), Edit, WebFetch, Grep]\n---\n\nBody.\n"
	got := importNeutralToolNames(input, "tools", true)
	if !strings.Contains(got, "can: [read(src/**), edit, WebFetch, Grep]") || !strings.HasSuffix(got, "\n\nBody.\n") {
		t.Errorf("import = %s", got)
	}
}

func TestLintNeutralCapabilities_ReportsExactAliases(t *testing.T) {
	b := spec.Bundle{Skills: []spec.Entry{{Kind: spec.KindSkill, Path: "s.md", Meta: map[string]any{"allowed-tools": []any{"Read", "WebFetch", "shell"}}}}}
	got := lintNeutralCapabilities(b)
	if len(got) != 1 || got[0].Code != "LINT037" || !strings.Contains(got[0].Message, "use read instead of Read") {
		t.Errorf("findings = %v", got)
	}
}

func TestImportNeutralToolNames_ScalarKeepsCommandCommas(t *testing.T) {
	input := "---\nname: review\nallowed-tools: 'Read, Bash(git log --format=%h,%s)'\n---\n\nBody.\n"
	got := importNeutralToolNames(input, "allowed-tools", false)
	entry, err := spec.ParseMarkdownBytes(spec.KindSkill, []byte(got))
	if err != nil {
		t.Fatal(err)
	}
	if want := "read, shell(git log --format=%h,%s)"; entry.Meta["allowed-tools"] != want {
		t.Errorf("import = %v; want %s", entry.Meta["allowed-tools"], want)
	}
}

func TestLintNeutralCapabilities_ScalarSuggestionKeepsCommandCommas(t *testing.T) {
	bundle := spec.Bundle{Skills: []spec.Entry{{Kind: spec.KindSkill, Meta: map[string]any{"allowed-tools": "Bash(git log --format=%h,%s)"}}}}
	findings := lintNeutralCapabilities(bundle)
	if len(findings) != 1 || !strings.Contains(findings[0].Message, "shell(git log --format=%h,%s)") {
		t.Errorf("findings = %v", findings)
	}
}

func TestExplainToolList_KeepsCommandCommas(t *testing.T) {
	got := explainToolList("Read, Bash(git log --format=%h,%s)")
	if len(got) != 2 || got[1] != "Bash(git log --format=%h,%s)" {
		t.Errorf("tools = %v", got)
	}
}
