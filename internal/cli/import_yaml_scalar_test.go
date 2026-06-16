package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// trickyDescriptions are values that break an unquoted YAML scalar: a
// colon-space mapping indicator, embedded double quotes, and leading
// whitespace. Each must round-trip through writeAgentMD / writeCodexRule
// into a spec that the parser accepts (#413).
var trickyDescriptions = []string{
	`Re-explain the answer. Use when the user says "simpler" or "ELI5". Find the middle ground: keep it readable.`,
	`key: value pair inside a description`,
	`  leading and trailing spaces  `,
	`has #hash and "quotes" and : colon`,
}

func TestWriteAgentMD_QuotesDescriptionSoSpecParses(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	for i, desc := range trickyDescriptions {
		path := filepath.Join(dir, "agent.md")
		if err := writeAgentMD(path, "example", desc, nil, "body"); err != nil {
			t.Fatalf("case %d: writeAgentMD: %v", i, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("case %d: read: %v", i, err)
		}
		entry, err := spec.ParseMarkdownBytes(spec.KindAgent, data)
		if err != nil {
			t.Fatalf("case %d: spec did not parse: %v\n--- file ---\n%s", i, err, data)
		}
		if got, _ := entry.Meta["description"].(string); got != desc {
			t.Errorf("case %d: description round-trip mismatch\n got: %q\nwant: %q", i, got, desc)
		}
	}
}

func TestWriteCodexRule_QuotesDescriptionSoSpecParses(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	for i, desc := range trickyDescriptions {
		sub := filepath.Join(dir, "rules", "case")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		name := "rule"
		if err := writeCodexRule(sub, name, desc, "", "body"); err != nil {
			t.Fatalf("case %d: writeCodexRule: %v", i, err)
		}
		data, err := os.ReadFile(filepath.Join(sub, name+".md"))
		if err != nil {
			t.Fatalf("case %d: read: %v", i, err)
		}
		entry, err := spec.ParseMarkdownBytes(spec.KindRule, data)
		if err != nil {
			t.Fatalf("case %d: spec did not parse: %v\n--- file ---\n%s", i, err, data)
		}
		if got, _ := entry.Meta["description"].(string); got != desc {
			t.Errorf("case %d: description round-trip mismatch\n got: %q\nwant: %q", i, got, desc)
		}
		_ = os.RemoveAll(sub)
	}
}
