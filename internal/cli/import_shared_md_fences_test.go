package cli

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestImport_H3ChildrenKeepFencedHeadingsInTheirBody(t *testing.T) {
	for _, code := range []string{
		"~~~markdown\n### Code heading\n~~~",
		"```markdown\n### Code heading\n```",
		"````markdown\n### Code heading\n```\n### Still fenced\n````",
		"~~~markdown\n### Code heading\n~~~not-a-close\n### Still fenced\n~~~",
		"~~~markdown\n<div>\n### Code heading\n~~~",
	} {
		t.Run(code, func(t *testing.T) {
			body := "### content\n\n" + code + "\n\n### following\n\nOutside code.\n"
			merged, ok := unwrapMergedH3Children(body, map[string]int{})
			if !ok || len(merged) != 2 {
				t.Errorf("merged children = %+v, want content and following", merged)
			} else if merged[0].slug != "content" || merged[1].slug != "following" || !strings.Contains(merged[0].body, code) {
				t.Errorf("merged split changed fenced body: %+v", merged)
			}
			codex := unwrapH3(body)
			if len(codex) != 2 {
				t.Errorf("Codex children = %+v, want content and following", codex)
			} else if codex[0].slug != "content" || codex[1].slug != "following" || !strings.Contains(codex[0].body, code) {
				t.Errorf("Codex split changed fenced body: %+v", codex)
			}
		})
	}
}

func TestImport_H3ChildrenKeepHTMLHeadingsInTheirBody(t *testing.T) {
	for _, tc := range []struct{ name, html, separator string }{
		{"tag block", "<div>\n### Literal\n</div>\n", "\n"},
		{"indented opening", "   <div>\n### Literal\n</div>\n", "\n"},
		{"tag closing does not end block", "<div>\n### Literal\n</div>\n### Still literal\n", "\n"},
		{"indented blank ends block", "<div>\n### Literal\n</div>\n    \n", ""},
		{"tab blank ends block", "<div>\n### Literal\n</div>\n\t\n", ""},
		{"comment indented closing", "<!--\n\n### Literal\n    -->\n", ""},
		{"script indented closing", "<script>\n\n### Literal\n    </script>\n", ""},
		{"processing instruction", "<?example\n### Literal\n?>\n", ""},
		{"CDATA", "<![CDATA[\n### Literal\n]]>\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := "### content\n\n" + tc.html + tc.separator + "### following\n\nOutside HTML.\n"
			merged, ok := unwrapMergedH3Children(body, map[string]int{})
			if !ok || len(merged) != 2 {
				t.Errorf("merged children = %+v, want content and following", merged)
			} else if merged[0].slug != "content" || merged[1].slug != "following" || !strings.Contains(merged[0].body, strings.TrimSpace(tc.html)) {
				t.Errorf("merged split changed HTML body: %+v", merged)
			}
			codex := unwrapH3(body)
			if len(codex) != 2 {
				t.Errorf("Codex children = %+v, want content and following", codex)
			} else if codex[0].slug != "content" || codex[1].slug != "following" || !strings.Contains(codex[0].body, strings.TrimSpace(tc.html)) {
				t.Errorf("Codex split changed HTML body: %+v", codex)
			}
		})
	}
}

func TestImport_H3AfterIndentedCodeIsNotHTML(t *testing.T) {
	body := "### content\n\n    <div>\n### following\n\nOutside indented code.\n"
	if children := unwrapH3(body); len(children) != 2 || children[1].slug != "following" {
		t.Errorf("indented code swallowed a heading: %+v", children)
	}
	if children, ok := unwrapMergedH3Children(body, map[string]int{}); !ok || len(children) != 2 || children[1].slug != "following" {
		t.Errorf("indented code swallowed a merged heading: %+v", children)
	}
}

func TestImport_BlockLeftOpenByOneRuleKeepsTheRulesAfterIt(t *testing.T) {
	for _, open := range []string{"```sh\necho unclosed", "<?php declare(strict_types=1);", "<!-- draft"} {
		t.Run(open, func(t *testing.T) {
			block := adapters.RenderRulesAppendix(spec.Bundle{Rules: []spec.Entry{
				{Kind: spec.KindRule, Name: "alpha", Path: ".agnostic-ai/rules/alpha.md", Body: "Alpha text.\n\n" + open + "\n"},
				{Kind: spec.KindRule, Name: "beta", Path: ".agnostic-ai/rules/beta.md", Body: "Beta text.\n"},
				{Kind: spec.KindRule, Name: "gamma", Path: ".agnostic-ai/rules/gamma.md", Body: "Gamma text.\n"},
			}})
			for file, importer := range map[string]func(string, config.Sources) error{"AGENTS.md": importFromCodex, geminiMainFile: importFromGemini} {
				dir := t.TempDir()
				writeFile(t, filepath.Join(dir, file), "# Project\n\n"+block)
				if err := importer(dir, rootSources()); err != nil {
					t.Fatal(err)
				}
				want := []string{"alpha.md", "beta.md", "gamma.md"}
				if got := names(mustReadDir(t, filepath.Join(dir, "rules"))); !slices.Equal(got, want) {
					t.Errorf("%s: rules = %v, want %v", file, got, want)
				}
				if alpha := readFileString(t, filepath.Join(dir, "rules", "alpha.md")); strings.Contains(alpha, "Beta text.") {
					t.Errorf("%s: alpha swallowed beta:\n%s", file, alpha)
				}
			}
		})
	}
}

func TestImport_H2LinesInsideCodeAndHTMLStayInTheirRule(t *testing.T) {
	for _, literal := range []string{
		"<div>\n## Literal\n</div>",
		"````markdown\n```\n## Inside\n```\n````",
		"<!--\n\n## Commented out\n\n-->",
	} {
		t.Run(literal, func(t *testing.T) {
			block := adapters.RenderRulesAppendix(spec.Bundle{Rules: []spec.Entry{
				{Kind: spec.KindRule, Name: "alpha", Path: ".agnostic-ai/rules/alpha.md", Body: "Alpha text.\n\n" + literal + "\n"},
				{Kind: spec.KindRule, Name: "beta", Path: ".agnostic-ai/rules/beta.md", Body: "Beta text.\n"},
			}})
			for file, importer := range map[string]func(string, config.Sources) error{"AGENTS.md": importFromCodex, geminiMainFile: importFromGemini} {
				dir := t.TempDir()
				writeFile(t, filepath.Join(dir, file), "# Project\n\n"+block)
				if err := importer(dir, rootSources()); err != nil {
					t.Fatal(err)
				}
				want := []string{"alpha.md", "beta.md"}
				if got := names(mustReadDir(t, filepath.Join(dir, "rules"))); !slices.Equal(got, want) {
					t.Errorf("%s: rules = %v, want %v", file, got, want)
				}
				if alpha := readFileString(t, filepath.Join(dir, "rules", "alpha.md")); !strings.Contains(alpha, literal) {
					t.Errorf("%s: alpha lost its literal heading:\n%s", file, alpha)
				}
			}
		})
	}
}

func TestSplitH2Sections_WrapperAfterAnOpenBlockStartsItsSection(t *testing.T) {
	doc := "## Rules\n\n### alpha\n\n<!-- source: .agnostic-ai/rules/alpha.md body-lines: 3 -->\n<?php declare(strict_types=1);\n\n\n" +
		"## Agents\n\n### reviewer\n\n<!-- source: .agnostic-ai/agents/reviewer.md body-lines: 3 -->\n<!-- draft\n\n\n" +
		"## Skills\n\nNo native skill execution. Reference only; invoke by reading the source file.\n\n" +
		"### tidy\n\n<!-- source: .agnostic-ai/skills/tidy/SKILL.md -->\nSource: `.agnostic-ai/skills/tidy/SKILL.md`\n"
	_, sections := splitH2Sections(doc)
	var slugs []string
	for _, s := range sections {
		slugs = append(slugs, s.slug)
	}
	if want := []string{"rules", "agents", "skills"}; !slices.Equal(slugs, want) {
		t.Errorf("sections = %v, want %v", slugs, want)
	}
}
