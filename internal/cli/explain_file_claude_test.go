package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func claudeFileJSON(t *testing.T, file string) explainFileOutput {
	t.Helper()
	out, err := runExplainFile(t, "--file", file, "--target", "claude", "--json")
	if err != nil {
		t.Fatalf("Claude file context: %v", err)
	}
	var report explainFileOutput
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("parse Claude file context: %v", err)
	}
	return report
}

func TestExplainFile_ClaudeReportsRootScopeAndExclusionsWithoutWrites(t *testing.T) {
	dir := setupFileContextFixture(t, "claude")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "Project instructions.\n")
	testutil.Chdir(t, dir)
	silence(t)
	before := listTree(t, dir)
	inside := claudeFileJSON(t, filepath.Join(dir, "services", "payments", "handler.go"))
	outside := claudeFileJSON(t, "web/page.go")
	if inside.Target != "claude" || inside.File != "services/payments/handler.go" || inside.Note != fileContextNote {
		t.Errorf("report = %+v", inside)
	}
	for _, c := range []struct{ source, output, status string }{
		{".agnostic-ai/AGNOSTIC_AI.md", "CLAUDE.md", contextAlways},
		{"rules/root-style.md", ".claude/rules/root-style.md", contextAlways},
		{"rules/payments-context.md", ".claude/rules/services/payments/payments-context.md", contextMatch},
		{"rules/web-context.md", ".claude/rules/web/web-context.md", contextNoMatch},
		{"rules/codex-only.md", "", contextExcluded},
	} {
		item := findItem(t, inside.Instructions, c.source, c.output)
		if item.Status != c.status || item.Reason == "" {
			t.Errorf("%s = %+v, want %s with a reason", c.source, item, c.status)
		}
	}
	if got := findItem(t, outside.Instructions, "rules/payments-context.md", ".claude/rules/services/payments/payments-context.md"); got.Status != contextNoMatch {
		t.Errorf("outside scope = %+v", got)
	}
	text, err := runExplainFile(t, "--file", inside.File, "--target", "claude")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range inside.Instructions {
		if !strings.Contains(text, "["+item.Status+"]") || !strings.Contains(text, item.Source) || !strings.Contains(text, item.Reason) {
			t.Errorf("text lacks JSON item %+v:\n%s", item, text)
		}
	}
	if strings.Join(before, "\n") != strings.Join(listTree(t, dir), "\n") {
		t.Error("Claude file context wrote files")
	}
}

func TestExplainFile_ClaudeUsesPlannedPathsAndNativeOutputOverrides(t *testing.T) {
	dir := setupFileContextFixture(t, "claude")
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\nsources:\n  rules: rules\ntargets: [claude]\noutputs:\n  claude:\n    rules-dir: .claude/rules/custom\n")
	writeFile(t, filepath.Join(dir, "rules", "typed.md"), "---\nname: typed\nglobs: src/**/*.go\nx-claude:\n  paths: [web/**]\n---\nBody.\n")
	testutil.Chdir(t, dir)
	silence(t)
	got := claudeFileJSON(t, "src/app.go")
	item := findItem(t, got.Instructions, "rules/typed.md", ".claude/rules/custom/typed.md")
	if item.Status != contextNoMatch || !strings.Contains(item.Selector, "web/**") {
		t.Errorf("native paths override = %+v", item)
	}
}

func TestExplainFile_ClaudeNonNativeOutputIsUnknown(t *testing.T) {
	dir := setupFileContextFixture(t, "claude")
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\nsources:\n  rules: rules\ntargets: [claude]\noutputs:\n  claude:\n    file: notes/instructions.md\n    rules-dir: notes/rules\n")
	testutil.Chdir(t, dir)
	silence(t)
	got := claudeFileJSON(t, "services/payments/handler.go")
	for _, c := range []struct{ source, output string }{
		{".agnostic-ai/AGNOSTIC_AI.md", "notes/instructions.md"},
		{"rules/root-style.md", "notes/rules/root-style.md"},
	} {
		item := findItem(t, got.Instructions, c.source, c.output)
		if item.Status != contextUnknown {
			t.Errorf("custom output = %+v", item)
		}
	}
}

func TestExplainFile_ClaudeNestedEntryPointNeedsSessionStartForOtherFiles(t *testing.T) {
	dir := setupFileContextFixture(t, "claude")
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\nsources:\n  rules: rules\ntargets: [claude]\noutputs:\n  claude:\n    file: services/payment systems/CLAUDE.md\n")
	testutil.Chdir(t, dir)
	silence(t)
	inside := claudeFileJSON(t, "services/payment systems/handler.go")
	outside := claudeFileJSON(t, "web/page.go")
	item := findItem(t, inside.Instructions, ".agnostic-ai/AGNOSTIC_AI.md", "services/payment systems/CLAUDE.md")
	if item.Status != contextMatch {
		t.Errorf("nested entry point = %+v", item)
	}
	item = findItem(t, outside.Instructions, ".agnostic-ai/AGNOSTIC_AI.md", "services/payment systems/CLAUDE.md")
	if item.Status != contextUnknown || !strings.Contains(item.Reason, "launch directory") {
		t.Errorf("session-dependent entry point = %+v", item)
	}
}

func TestExplainFile_ClaudeLegacyImportUsesActualMergedOutput(t *testing.T) {
	dir := setupFileContextFixture(t, "claude")
	writeUnscopedClaudeFixtureRules(t, dir)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\nsources:\n  rules: rules\ntargets: [claude]\noutputs:\n  claude:\n    rules-file: instructions/project.md\n")
	testutil.Chdir(t, dir)
	silence(t)
	got := claudeFileJSON(t, "web/page.go")
	item := findItem(t, got.Instructions, "rules/payments-context.md", "instructions/project.md")
	if item.Status != contextUnknown || !strings.Contains(item.Reason, "import") {
		t.Errorf("imported merged rule = %+v, want unknown session import conditions", item)
	}
}

func TestExplainFile_ClaudeLegacyEntryPointDoesNotInventCanonicalBody(t *testing.T) {
	dir := setupFileContextFixture(t, "claude")
	writeUnscopedClaudeFixtureRules(t, dir)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\nsources:\n  rules: rules\ntargets: [claude]\noutputs:\n  claude:\n    rules-file: CLAUDE.md\n")
	testutil.Chdir(t, dir)
	silence(t)
	got := claudeFileJSON(t, "web/page.go")
	item := findItem(t, got.Instructions, "rules/payments-context.md", "CLAUDE.md")
	if item.Status != contextAlways {
		t.Errorf("merged rule = %+v", item)
	}
	for _, item := range got.Instructions {
		if item.Source == ".agnostic-ai/AGNOSTIC_AI.md" {
			t.Errorf("canonical body is not emitted in legacy-owned entry point: %+v", item)
		}
	}
}

func TestExplainFile_ClaudeNativeEntryPointOverrideAndUncertainPattern(t *testing.T) {
	dir := setupFileContextFixture(t, "claude")
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\nsources:\n  rules: rules\ntargets: [claude]\noutputs:\n  claude:\n    file: .claude/rules/project.md\n")
	writeFile(t, filepath.Join(dir, "rules", "complex.md"), "---\nname: complex\npaths: [\"src/**/*.{go,ts}\"]\n---\nBody.\n")
	testutil.Chdir(t, dir)
	silence(t)
	got := claudeFileJSON(t, "src/app.go")
	if item := findItem(t, got.Instructions, ".agnostic-ai/AGNOSTIC_AI.md", ".claude/rules/project.md"); item.Status != contextAlways {
		t.Errorf("native rules entry point = %+v", item)
	}
	if item := findItem(t, got.Instructions, "rules/complex.md", ".claude/rules/complex.md"); item.Status != contextUnknown {
		t.Errorf("unevaluated paths syntax = %+v", item)
	}
}

func TestExplainFile_ClaudeBuiltinSourceProvenance(t *testing.T) {
	setupMemoryProject(t, "claude")
	got := claudeFileJSON(t, "main.go")
	item := findItem(t, got.Instructions, "builtin:shared-memory-policy", ".claude/rules/shared-memory-policy.md")
	if item.Status != contextAlways {
		t.Errorf("builtin = %+v", item)
	}
}

func TestExplainFile_ClaudeNotConfigured(t *testing.T) {
	dir := setupFileContextFixture(t, "cursor")
	testutil.Chdir(t, dir)
	silence(t)
	_, err := runExplainFile(t, "--file", "main.go", "--target", "claude")
	if err == nil || !strings.Contains(err.Error(), "not a configured target") {
		t.Errorf("error = %v", err)
	}
}

func TestExplainFile_ClaudeNativeRulesStillApplyWhenAlsoImported(t *testing.T) {
	dir := setupFileContextFixture(t, "claude")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "@.claude/rules/root-style.md\n@.claude/rules/services/payments/payments-context.md\n")
	testutil.Chdir(t, dir)
	silence(t)
	inside := claudeFileJSON(t, "services/payments/handler.go")
	outside := claudeFileJSON(t, "web/page.go")
	if item := findItem(t, outside.Instructions, "rules/root-style.md", ".claude/rules/root-style.md"); item.Status != contextAlways {
		t.Errorf("unconditional native rule = %+v", item)
	}
	if item := findItem(t, inside.Instructions, "rules/payments-context.md", ".claude/rules/services/payments/payments-context.md"); item.Status != contextMatch {
		t.Errorf("matching native rule = %+v", item)
	}
	if item := findItem(t, outside.Instructions, "rules/payments-context.md", ".claude/rules/services/payments/payments-context.md"); item.Status != contextUnknown || !strings.Contains(item.Reason, "import") {
		t.Errorf("additional imported route = %+v", item)
	}
}

func writeUnscopedClaudeFixtureRules(t *testing.T, dir string) {
	t.Helper()
	for _, name := range []string{"payments-context", "web-context"} {
		writeFile(t, filepath.Join(dir, "rules", name+".md"), "---\nname: "+name+"\n---\nBody.\n")
	}
}

func TestExplainFile_ClaudeProseImportCanWidenNativeScope(t *testing.T) {
	dir := setupFileContextFixture(t, "claude")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "Read @.claude/rules/services/payments/payments-context.md for payment checks.\n")
	testutil.Chdir(t, dir)
	silence(t)
	got := claudeFileJSON(t, "web/page.go")
	item := findItem(t, got.Instructions, "rules/payments-context.md", ".claude/rules/services/payments/payments-context.md")
	if item.Status != contextUnknown || !strings.Contains(item.Reason, "import") {
		t.Errorf("prose import = %+v", item)
	}
}

func TestExplainFile_ClaudeEscapedSpaceImportCanWidenNativeScope(t *testing.T) {
	dir := setupFileContextFixture(t, "claude")
	writeFile(t, filepath.Join(dir, "rules", "payments-context.md"), "---\nname: payments-context\nscope: services/payment systems\n---\nBody.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "Read @.claude/rules/services/payment\\ systems/payments-context.md for payment checks.\n")
	testutil.Chdir(t, dir)
	silence(t)
	got := claudeFileJSON(t, "web/page.go")
	item := findItem(t, got.Instructions, "rules/payments-context.md", ".claude/rules/services/payment systems/payments-context.md")
	if item.Status != contextUnknown || !strings.Contains(item.Reason, "import") {
		t.Errorf("escaped space import = %+v", item)
	}
}

func TestExplainFile_ClaudeImportExamplesDoNotWidenNativeScope(t *testing.T) {
	dir := setupFileContextFixture(t, "claude")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "Example `@.claude/rules/services/payments/payments-context.md`.\n\n```md\n@.claude/rules/services/payments/payments-context.md\n```\n")
	testutil.Chdir(t, dir)
	silence(t)
	got := claudeFileJSON(t, "web/page.go")
	item := findItem(t, got.Instructions, "rules/payments-context.md", ".claude/rules/services/payments/payments-context.md")
	if item.Status != contextNoMatch {
		t.Errorf("literal import examples = %+v", item)
	}
}

func TestExplainFile_ClaudeKeepsNonMarkdownMergedOutput(t *testing.T) {
	dir := setupFileContextFixture(t, "claude")
	writeUnscopedClaudeFixtureRules(t, dir)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\nsources:\n  rules: rules\ntargets: [claude]\noutputs:\n  claude:\n    rules-file: instructions/project.txt\n")
	testutil.Chdir(t, dir)
	silence(t)
	got := claudeFileJSON(t, "main.go")
	item := findItem(t, got.Instructions, "rules/root-style.md", "instructions/project.txt")
	if item.Status != contextUnknown || !strings.Contains(item.Reason, "import") {
		t.Errorf("non-Markdown merged output = %+v", item)
	}
}

func TestExplainFile_ClaudeKeepsNonMarkdownEntryPoint(t *testing.T) {
	dir := setupFileContextFixture(t, "claude")
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\nsources:\n  rules: rules\ntargets: [claude]\noutputs:\n  claude:\n    file: instructions/project.txt\n")
	testutil.Chdir(t, dir)
	silence(t)
	got := claudeFileJSON(t, "main.go")
	item := findItem(t, got.Instructions, ".agnostic-ai/AGNOSTIC_AI.md", "instructions/project.txt")
	if item.Status != contextUnknown {
		t.Errorf("non-Markdown entry point = %+v", item)
	}
}

func TestExplainFile_ClaudeNonMarkdownEntryPointInsideRulesIsUnknown(t *testing.T) {
	dir := setupFileContextFixture(t, "claude")
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\nsources:\n  rules: rules\ntargets: [claude]\noutputs:\n  claude:\n    file: .claude/rules/project.txt\n")
	testutil.Chdir(t, dir)
	silence(t)
	got := claudeFileJSON(t, "main.go")
	item := findItem(t, got.Instructions, ".agnostic-ai/AGNOSTIC_AI.md", ".claude/rules/project.txt")
	if item.Status != contextUnknown {
		t.Errorf("non-Markdown file inside rules directory = %+v", item)
	}
}

func TestExplainFile_ClaudeNonMarkdownMergedOutputInsideRulesIsUnknown(t *testing.T) {
	dir := setupFileContextFixture(t, "claude")
	writeUnscopedClaudeFixtureRules(t, dir)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\nsources:\n  rules: rules\ntargets: [claude]\noutputs:\n  claude:\n    rules-file: .claude/rules/project.txt\n")
	testutil.Chdir(t, dir)
	silence(t)
	got := claudeFileJSON(t, "main.go")
	item := findItem(t, got.Instructions, "rules/root-style.md", ".claude/rules/project.txt")
	if item.Status != contextUnknown || !strings.Contains(item.Reason, "import") {
		t.Errorf("non-Markdown merged rules = %+v", item)
	}
}

func TestExplainFile_ClaudeAbsoluteProseImportCanWidenNativeScope(t *testing.T) {
	dir := setupFileContextFixture(t, "claude")
	ref := filepath.ToSlash(filepath.Join(dir, ".claude", "rules", "services", "payments", "payments-context.md"))
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "Read @"+strings.ReplaceAll(ref, " ", "\\ ")+" for payment checks.\n")
	testutil.Chdir(t, dir)
	silence(t)
	got := claudeFileJSON(t, "web/page.go")
	item := findItem(t, got.Instructions, "rules/payments-context.md", ".claude/rules/services/payments/payments-context.md")
	if item.Status != contextUnknown || !strings.Contains(item.Reason, "import") {
		t.Errorf("absolute prose import = %+v", item)
	}
}

func TestExplainFile_ClaudeAbsoluteRulesOutputKeepsNativeDiscovery(t *testing.T) {
	dir := setupFileContextFixture(t, "claude")
	rulesDir := filepath.Join(dir, ".claude", "rules")
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), fmt.Sprintf("version: 1\nsources:\n  rules: rules\ntargets: [claude]\noutputs:\n  claude:\n    rules-dir: %q\n", filepath.ToSlash(rulesDir)))
	testutil.Chdir(t, dir)
	silence(t)
	output := filepath.ToSlash(filepath.Join(rulesDir, "services", "payments", "payments-context.md"))
	inside := claudeFileJSON(t, "services/payments/handler.go")
	outside := claudeFileJSON(t, "web/page.go")
	if item := findItem(t, inside.Instructions, "rules/payments-context.md", output); item.Status != contextMatch {
		t.Errorf("absolute native output inside scope = %+v", item)
	}
	if item := findItem(t, outside.Instructions, "rules/payments-context.md", output); item.Status != contextNoMatch {
		t.Errorf("absolute native output outside scope = %+v", item)
	}
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "Read @"+strings.ReplaceAll(output, " ", "\\ ")+" for payment checks.\n")
	got := claudeFileJSON(t, "web/page.go")
	if item := findItem(t, got.Instructions, "rules/payments-context.md", output); item.Status != contextUnknown {
		t.Errorf("absolute output and absolute import = %+v", item)
	}
}

func TestExplainFile_ClaudeRevisitsImportsReachedByAShallowerRoute(t *testing.T) {
	dir := setupFileContextFixture(t, "claude")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "@.claude/rules/a.md\n@.claude/rules/d.md\n")
	next := map[string]string{"a": "b", "b": "c", "c": "d", "d": "e"}
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		body := "Leaf instructions.\n"
		if ref, ok := next[name]; ok {
			body = "Read @" + ref + ".md for context.\n"
		}
		writeFile(t, filepath.Join(dir, "rules", name+".md"), "---\nname: "+name+"\npaths: [src/**]\n---\n"+body)
	}
	testutil.Chdir(t, dir)
	silence(t)
	got := claudeFileJSON(t, "web/page.go")
	item := findItem(t, got.Instructions, "rules/e.md", ".claude/rules/e.md")
	if item.Status != contextUnknown || !strings.Contains(item.Reason, "import") {
		t.Errorf("leaf reached by shorter import route = %+v", item)
	}
}
