package cli

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// setupFileContextFixture builds a project with a root rule, a rule
// scoped to services/payments, an unrelated scoped rule, activation
// variants, and a rule that excludes cursor.
func setupFileContextFixture(t *testing.T, targets ...string) string {
	t.Helper()
	dir := t.TempDir()
	cfg := "version: 1\nsources:\n  rules: rules\ntargets:\n"
	for _, tg := range targets {
		cfg += "  - " + tg + "\n"
	}
	files := map[string]string{
		"agnostic-ai.yaml": cfg,
		"rules/root-style.md": `---
name: root-style
description: Project-wide style.
---

Keep functions short.
`,
		"rules/payments-context.md": `---
name: payments-context
scope: services/payments
---

Use integer minor units for money.
`,
		"rules/web-context.md": `---
name: web-context
scope: web
---

Use semantic HTML.
`,
		"rules/ask-first.md": `---
name: ask-first
description: Migration guidance for database changes.
alwaysApply: false
---

Write reversible migrations.
`,
		"rules/manual-only.md": `---
name: manual-only
alwaysApply: false
---

Release checklist.
`,
		"rules/codex-only.md": `---
name: codex-only
targets: [codex]
---

Codex-only notes.
`,
	}
	for rel, body := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runExplainFile(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs(append([]string{"explain"}, args...))
	err := root.Execute()
	return out.String(), err
}

func explainFileJSON(t *testing.T, file string) explainFileOutput {
	t.Helper()
	out, err := runExplainFile(t, "--file", file, "--target", "cursor", "--json")
	if err != nil {
		t.Fatalf("explain --file: %v", err)
	}
	var got explainFileOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	return got
}

// findItem returns the first item with the given source and output.
func findItem(t *testing.T, items []fileContextItem, source, output string) fileContextItem {
	t.Helper()
	for _, it := range items {
		if it.Source == source && it.Output == output {
			return it
		}
	}
	t.Fatalf("no item for source %q output %q in %+v", source, output, items)
	return fileContextItem{}
}

func TestExplainFile_CursorOnlyClassifiesEveryRule(t *testing.T) {
	dir := setupFileContextFixture(t, "cursor")
	testutil.Chdir(t, dir)
	silence(t)

	got := explainFileJSON(t, "services/payments/handler.go")
	if got.Command != "explain" || got.Target != "cursor" || got.File != "services/payments/handler.go" {
		t.Errorf("envelope wrong: %+v", got)
	}
	if !strings.Contains(got.Note, "not a record") {
		t.Errorf("note must disclaim active context, got %q", got.Note)
	}
	cases := []struct {
		source, output, status, selector string
	}{
		{"rules/root-style.md", ".cursor/rules/root-style.mdc", statusAlways, "alwaysApply: true"},
		{"rules/payments-context.md", ".cursor/rules/services/payments/payments-context.mdc", statusMatch, "globs: services/payments/**"},
		{"rules/web-context.md", ".cursor/rules/web/web-context.mdc", statusNoMatch, "globs: web/**"},
		{"rules/ask-first.md", ".cursor/rules/ask-first.mdc", statusModel, "description"},
		{"rules/manual-only.md", ".cursor/rules/manual-only.mdc", statusManual, "@-mention"},
		{"rules/codex-only.md", "", statusExcluded, ""},
	}
	for _, c := range cases {
		it := findItem(t, got.Instructions, c.source, c.output)
		if it.Status != c.status {
			t.Errorf("%s: status want %q, got %q (%s)", c.source, c.status, it.Status, it.Reason)
		}
		if it.Selector != c.selector {
			t.Errorf("%s: selector want %q, got %q", c.source, c.selector, it.Selector)
		}
		if it.Reason == "" {
			t.Errorf("%s: missing reason", c.source)
		}
	}
	for _, it := range got.Instructions {
		if it.Output == "AGENTS.md" {
			t.Errorf("cursor-only project must not report a root AGENTS.md: %+v", it)
		}
	}
}

func TestExplainFile_SharedScopedAgentsMDWithCodex(t *testing.T) {
	dir := setupFileContextFixture(t, "cursor", "codex")
	testutil.Chdir(t, dir)
	silence(t)

	got := explainFileJSON(t, "services/payments/handler.go")

	payments := findItem(t, got.Instructions, "rules/payments-context.md", "services/payments/AGENTS.md")
	if payments.Status != statusMatch {
		t.Errorf("shared scoped AGENTS.md: want match, got %+v", payments)
	}
	web := findItem(t, got.Instructions, "rules/web-context.md", "web/AGENTS.md")
	if web.Status != statusNoMatch {
		t.Errorf("unrelated scoped AGENTS.md: want no-match, got %+v", web)
	}
	body := findItem(t, got.Instructions, ".agnostic-ai/AGNOSTIC_AI.md", "AGENTS.md")
	if body.Status != statusAlways {
		t.Errorf("root entry point: want always, got %+v", body)
	}
	// A rule excluded from cursor still reaches Cursor through the
	// root AGENTS.md codex reads, and the report must say so.
	inlined := findItem(t, got.Instructions, "rules/codex-only.md", "AGENTS.md")
	if inlined.Status != statusAlways || !strings.Contains(inlined.Reason, "codex") {
		t.Errorf("codex-only rule inlined in AGENTS.md: %+v", inlined)
	}
	findItem(t, got.Instructions, "rules/codex-only.md", "")
	for _, it := range got.Instructions {
		if it.Output == ".cursor/rules/services/payments/payments-context.mdc" {
			t.Errorf("shared scope must not also emit a Cursor .mdc: %+v", it)
		}
	}
}

func TestExplainFile_UnevaluableGlobIsUnknown(t *testing.T) {
	dir := setupFileContextFixture(t, "cursor")
	if err := os.WriteFile(filepath.Join(dir, "rules", "tsx.md"), []byte(`---
name: tsx
alwaysApply: false
globs: "src/**/*.{ts,tsx}"
---

TSX rules.
`), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
	silence(t)

	got := explainFileJSON(t, "src/app.ts")
	it := findItem(t, got.Instructions, "rules/tsx.md", ".cursor/rules/tsx.mdc")
	if it.Status != statusUnknown {
		t.Errorf("brace glob: want unknown, got %+v", it)
	}
}

func TestExplainFile_ScopedRuleWithoutCursorOutputIsNotEmitted(t *testing.T) {
	dir := setupFileContextFixture(t, "cursor")
	if err := os.WriteFile(filepath.Join(dir, "rules", "split.md"), []byte(`---
name: split
scope: services/payments
globs: "services/payments/a/**,services/payments/b/**"
---

Split rule.
`), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
	silence(t)

	got := explainFileJSON(t, "services/payments/a/x.go")
	it := findItem(t, got.Instructions, "rules/split.md", "")
	if it.Status != statusNotEmitted {
		t.Errorf("want not-emitted, got %+v", it)
	}
}

func TestExplainFile_HumanOutput(t *testing.T) {
	dir := setupFileContextFixture(t, "cursor")
	testutil.Chdir(t, dir)
	silence(t)

	out, err := runExplainFile(t, "--file", "services/payments/handler.go", "--target", "cursor")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"services/payments/handler.go (cursor)",
		"not a record",
		"[match] .cursor/rules/services/payments/payments-context.mdc <- rules/payments-context.md",
		"[excluded] (no cursor output) <- rules/codex-only.md",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestExplainFile_WritesNothing(t *testing.T) {
	dir := setupFileContextFixture(t, "cursor", "codex")
	testutil.Chdir(t, dir)
	silence(t)

	before := listTree(t, dir)
	if _, err := runExplainFile(t, "--file", "services/payments/handler.go", "--target", "cursor"); err != nil {
		t.Fatal(err)
	}
	after := listTree(t, dir)
	if strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Errorf("explain --file wrote files:\nbefore %v\nafter  %v", before, after)
	}
}

func listTree(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		out = append(out, p)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func TestExplainFile_InputModeErrors(t *testing.T) {
	dir := setupFileContextFixture(t, "cursor", "claude")
	testutil.Chdir(t, dir)
	silence(t)

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"spec and file", []string{"rules/root-style.md", "--file", "a.go", "--target", "cursor"}, "cannot be combined"},
		{"file without target", []string{"--file", "a.go"}, "--target"},
		{"target without file", []string{"rules/root-style.md", "--target", "cursor"}, "--file"},
		{"no input", nil, "spec"},
		{"unsupported target", []string{"--file", "a.go", "--target", "claude"}, "unsupported"},
		{"outside project", []string{"--file", "../a.go", "--target", "cursor"}, "outside the project"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := runExplainFile(t, c.args...)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
		})
	}
}

func TestExplainFile_CursorNotConfigured(t *testing.T) {
	dir := setupFileContextFixture(t, "claude")
	testutil.Chdir(t, dir)
	silence(t)

	_, err := runExplainFile(t, "--file", "a.go", "--target", "cursor")
	if err == nil || !strings.Contains(err.Error(), "not a configured target") {
		t.Fatalf("want not-configured error, got %v", err)
	}
}
