package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestQuoteHashInPlainScalars_DescriptionWithHash(t *testing.T) {
	in := "name: gh-issue\ndescription: End-to-end. Use when given #number or URL.\nmodel: gpt-4"
	got := quoteHashInPlainScalars(in)
	if !strings.Contains(got, `description: "End-to-end. Use when given #number or URL."`) {
		t.Errorf("expected description to be quoted, got:\n%s", got)
	}
	var fm map[string]any
	if err := yaml.Unmarshal([]byte(got), &fm); err != nil {
		t.Fatal(err)
	}
	if d, _ := fm["description"].(string); d != "End-to-end. Use when given #number or URL." {
		t.Errorf("strict YAML lost data after quoting; got %q", d)
	}
}

func TestQuoteHashInPlainScalars_AlreadyQuoted(t *testing.T) {
	in := `description: "End-to-end #number"` + "\n"
	got := quoteHashInPlainScalars(in)
	if got != in {
		t.Errorf("already-quoted value must not be rewritten:\ngot %q\nwant %q", got, in)
	}
}

func TestQuoteHashInPlainScalars_NoHashUnchanged(t *testing.T) {
	in := "name: plain\ndescription: nothing fancy here\n"
	got := quoteHashInPlainScalars(in)
	if got != in {
		t.Errorf("plain scalars without '#' must pass through:\ngot %q\nwant %q", got, in)
	}
}

func TestQuoteHashInPlainScalars_EscapesQuotesAndBackslash(t *testing.T) {
	in := `description: a " and \ then #hash` + "\n"
	got := quoteHashInPlainScalars(in)
	var fm map[string]any
	if err := yaml.Unmarshal([]byte(got), &fm); err != nil {
		t.Fatal(err)
	}
	if d, _ := fm["description"].(string); d != `a " and \ then #hash` {
		t.Errorf("quote/backslash escape round-trip failed; got %q", d)
	}
}

func TestQuoteHashInPlainScalars_BlockScalarUntouched(t *testing.T) {
	in := "description: |\n  multi-line #literal\n  stays here\n"
	got := quoteHashInPlainScalars(in)
	if got != in {
		t.Errorf("block scalar must not be rewritten:\n%s", got)
	}
}

func TestQuoteHashInSkillFrontmatter_RewritesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	writeFile(t, path, "---\nname: x\ndescription: with #hash in it\n---\n\nbody.\n")
	if err := quoteHashInSkillFrontmatter(path); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, path)
	if !strings.Contains(got, `description: "with #hash in it"`) {
		t.Errorf("description not quoted on disk:\n%s", got)
	}
	if !strings.HasSuffix(got, "body.\n") {
		t.Errorf("body lost during rewrite:\n%s", got)
	}
}

func TestQuoteHashInSkillFrontmatter_NoFrontmatterNoOp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	orig := "no frontmatter here\n"
	writeFile(t, path, orig)
	if err := quoteHashInSkillFrontmatter(path); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != orig {
		t.Errorf("file without frontmatter should be untouched:\ngot  %q\nwant %q", got, orig)
	}
}

func TestImportFromCodex_FreshSkillDescriptionWithHash_NotTruncated(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".codex", "skills", "gh-issue", "SKILL.md"),
		"---\nname: gh-issue\ndescription: End-to-end workflow. Use when given #number or issue URL.\n---\n\nBody here.\n")
	if err := importFromCodex(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "skills", "gh-issue", "SKILL.md"))
	if !strings.Contains(got, "#number or issue URL") {
		t.Errorf("description lost data after import:\n%s", got)
	}
	front, _, ok := splitCodexAgentFrontmatter(got)
	if !ok {
		t.Fatalf("imported SKILL.md missing frontmatter:\n%s", got)
	}
	var fm map[string]any
	if err := yaml.Unmarshal([]byte(front), &fm); err != nil {
		t.Fatalf("imported frontmatter does not parse: %v\n%s", err, front)
	}
	if d, _ := fm["description"].(string); d != "End-to-end workflow. Use when given #number or issue URL." {
		t.Errorf("strict-YAML round-trip truncated description: got %q", d)
	}
}

func TestImportFromCodex_MergedSkillDescriptionWithHash_NotTruncated(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude", "skills", "gh-issue", "SKILL.md"),
		"---\nname: gh-issue\ndescription: claude version\n---\n\nclaude body.\n")
	writeFile(t, filepath.Join(dir, ".codex", "skills", "gh-issue", "SKILL.md"),
		"---\nname: gh-issue\ndescription: End-to-end. Use when given #number or URL.\n---\n\ncodex body.\n")
	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if err := importFromCodex(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "skills", "gh-issue", "SKILL.md"))
	if !strings.Contains(got, "#number or URL") {
		t.Errorf("codex description truncated during merge:\n%s", got)
	}
}
