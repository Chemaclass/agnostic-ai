package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func codexFileJSON(t *testing.T, file string) explainFileOutput {
	t.Helper()
	out, err := runExplainFile(t, "--file", file, "--target", "codex", "--json")
	if err != nil {
		t.Fatalf("explain codex: %v", err)
	}
	var got explainFileOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode codex report: %v", err)
	}
	return got
}

func setupCodexRootFixture(t *testing.T, outputs string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\nsources:\n  rules: rules\ntargets: [codex]\n"+outputs)
	writeFile(t, filepath.Join(dir, "rules", "root style.md"), "---\nname: root-style\n---\n\nKeep functions short.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "Project instructions.\n")
	return dir
}

func TestExplainFile_CodexUsesConfiguredOutputPathsAndProvenance(t *testing.T) {
	for _, c := range []struct{ field, output, status string }{
		{"file", "notes/project.txt", contextUnknown},
		{"file", "AGENTS.override.md", contextAlways},
		{"file", "services/payment systems/AGENTS.md", contextUnknown},
		{"rules-file", "notes/project.txt", contextUnknown},
		{"rules-file", "AGENTS.override.md", contextAlways},
		{"rules-file", "AGENTS.md", contextAlways},
	} {
		t.Run(c.field+"/"+c.output, func(t *testing.T) {
			dir := setupCodexRootFixture(t, fmt.Sprintf("outputs:\n  codex:\n    %s: %s\n    provenance-header: false\n", c.field, c.output))
			testutil.Chdir(t, dir)
			silence(t)
			got := codexFileJSON(t, "web/page.go")
			item := findItem(t, got.Instructions, "rules/root style.md", c.output)
			if item.Status != c.status {
				t.Errorf("configured output = %+v", item)
			}
			if c.field == "file" {
				findItem(t, got.Instructions, ".agnostic-ai/AGNOSTIC_AI.md", c.output)
			}
		})
	}
}

func TestExplainFile_CodexPlannedOverrideShadowsOrdinaryInstructions(t *testing.T) {
	dir := setupCodexRootFixture(t, "")
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\nsources:\n  rules: rules\ntargets: [codex, amp]\noutputs:\n  codex:\n    file: AGENTS.override.md\n")
	testutil.Chdir(t, dir)
	silence(t)
	got := codexFileJSON(t, "web/page.go")
	item := findItem(t, got.Instructions, ".agnostic-ai/AGNOSTIC_AI.md", "AGENTS.md")
	if item.Status != contextNoMatch || !strings.Contains(item.Reason, "AGENTS.override.md") {
		t.Errorf("shadowed planned file = %+v", item)
	}
	if item := findItem(t, got.Instructions, "rules/root style.md", "AGENTS.override.md"); item.Status != contextAlways {
		t.Errorf("override = %+v", item)
	}
}

func TestExplainFile_CodexAbsoluteNativeOutputAndInput(t *testing.T) {
	dir := setupCodexRootFixture(t, "")
	output := filepath.Join(dir, "AGENTS.md")
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), fmt.Sprintf("version: 1\nsources:\n  rules: rules\ntargets: [codex]\noutputs:\n  codex:\n    file: %q\n", filepath.ToSlash(output)))
	testutil.Chdir(t, dir)
	silence(t)
	got := codexFileJSON(t, filepath.Join(dir, "web", "page.go"))
	if item := findItem(t, got.Instructions, "rules/root style.md", filepath.ToSlash(output)); item.Status != contextAlways {
		t.Errorf("absolute native output = %+v", item)
	}
}

func TestExplainFile_CodexExplainsNestedGlobOutputAndRootFallback(t *testing.T) {
	dir := setupCodexRootFixture(t, "")
	writeFile(t, filepath.Join(dir, "rules", "nested.md"), "---\nname: nested\nglobs: [src/api/**]\n---\n\nAPI guidance.\n")
	writeFile(t, filepath.Join(dir, "rules", "fallback.md"), "---\nname: fallback\nglobs: ['**/*.go']\n---\n\nGo guidance.\n")
	testutil.Chdir(t, dir)
	silence(t)
	got := codexFileJSON(t, "web/page.go")
	if item := findItem(t, got.Instructions, "rules/nested.md", "src/api/AGENTS.md"); item.Status != contextUnknown {
		t.Errorf("nested glob = %+v", item)
	}
	if item := findItem(t, got.Instructions, "rules/fallback.md", "AGENTS.md"); item.Status != contextAlways {
		t.Errorf("fallback = %+v", item)
	}
}

func TestExplainFile_CodexCommandReportsRootNestedAndExcluded(t *testing.T) {
	dir := setupFileContextFixture(t, "codex")
	writeFile(t, filepath.Join(dir, "rules", "excluded.md"), "---\nname: excluded\ntarget-exclude: codex\n---\n\nExcluded guidance.\n")
	testutil.Chdir(t, dir)
	silence(t)
	before := listTree(t, dir)
	for _, file := range []string{"services/payments/handler.go", "web/page.go"} {
		got := codexFileJSON(t, file)
		if got.Target != "codex" || got.File != file || !strings.Contains(got.Note, "not a record") {
			t.Errorf("envelope = %+v", got)
		}
		root := findItem(t, got.Instructions, "rules/root-style.md", "AGENTS.md")
		if root.Status != contextAlways {
			t.Errorf("root = %+v", root)
		}
		nested := findItem(t, got.Instructions, "rules/payments-context.md", "services/payments/AGENTS.md")
		relation := "outside"
		if strings.HasPrefix(file, "services/payments/") {
			relation = "under"
		}
		if nested.Status != contextUnknown || !strings.Contains(nested.Reason, "launch directory") || !strings.Contains(nested.Reason, relation) {
			t.Errorf("nested = %+v", nested)
		}
		if item := findItem(t, got.Instructions, "rules/excluded.md", ""); item.Status != contextExcluded {
			t.Errorf("excluded = %+v", item)
		}
		text, err := runExplainFile(t, "--file", file, "--target", "codex")
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range got.Instructions {
			if !strings.Contains(text, item.Source) || !strings.Contains(text, item.Reason) || !strings.Contains(text, item.Output) {
				t.Errorf("text omits JSON item %+v: %s", item, text)
			}
		}
		if strings.Contains(text, "Keep functions short.") || strings.Contains(text, "Use integer minor units for money.") {
			t.Errorf("report exposes instruction bodies: %s", text)
		}
	}
	if after := listTree(t, dir); strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Errorf("explain changed the project tree: before %v, after %v", before, after)
	}
	for _, output := range []string{"AGENTS.md", "services/payments/AGENTS.md"} {
		if _, err := os.Stat(output); err == nil {
			t.Errorf("explain wrote %s", output)
		}
	}
}

func TestExplainFile_CodexNotConfigured(t *testing.T) {
	dir := setupFileContextFixture(t, "claude")
	testutil.Chdir(t, dir)
	silence(t)
	_, err := runExplainFile(t, "--file", "a.go", "--target", "codex")
	if err == nil || !strings.Contains(err.Error(), "not a configured target") {
		t.Errorf("expected not configured error, got %v", err)
	}
}

func TestExplainFile_CodexHelpListsSupportedTarget(t *testing.T) {
	out, err := runExplainFile(t, "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "--target string") {
			if !strings.Contains(line, "codex") {
				t.Errorf("target flag help omits Codex: %s", line)
			}
			return
		}
	}
	t.Fatal("target flag help is missing")
}

func TestExplainFile_CodexUnmanagedInstructionsAreNotEmitted(t *testing.T) {
	dir := setupCodexRootFixture(t, "sync:\n  unmanaged: [AGENTS.md]\n")
	testutil.Chdir(t, dir)
	silence(t)
	got := codexFileJSON(t, "web/page.go")
	if item := findItem(t, got.Instructions, "rules/root style.md", ""); item.Status != contextNotEmitted {
		t.Errorf("unmanaged output = %+v", item)
	}
	for _, item := range got.Instructions {
		if item.Output == "AGENTS.md" {
			t.Errorf("unmanaged output reported as planned: %+v", item)
		}
	}
}
