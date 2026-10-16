package integration

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSiteDocs_ExportsDisclosureBodiesWithoutTemplateMarkers(t *testing.T) {
	outputDir := t.TempDir()
	command := exec.Command("bash", "./scripts/build-llm-docs.sh", outputDir)
	command.Dir = "../.."
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build reference export: %v\n%s", err, output)
	}
	full := readBuiltFile(t, filepath.Join(outputDir, "llms-full.txt"))
	for _, text := range []string{
		"When `<path>.bak` already exists",
		"An earlier backup is never replaced.",
		"LINT011 for the Codex AGENTS.md chain",
		"Codex also reads each scoped `AGENTS.md`",
	} {
		if !strings.Contains(full, text) {
			t.Errorf("full export lost disclosure content %q", text)
		}
	}
	for _, marker := range []string{"{% <details", "{% </details> %}", "\n<details>\n", "\n</details>\n", "<summary>", "<span id=", ".legacy-anchor"} {
		if strings.Contains(full, marker) {
			t.Errorf("full export leaks %q", marker)
		}
	}
	for _, title := range []string{"# Global configuration", "# Hook reference", "# Hook recipes", "# MCP import credentials", "# Lint codes"} {
		if !strings.Contains(full, "\n"+title+"\n") {
			t.Errorf("full export omitted reference %q", title)
		}
	}
}

func TestSiteDocs_IndexesDeepReferenceSections(t *testing.T) {
	outputDir := t.TempDir()
	command := exec.Command(lookupPinnedZola(t), "--root", "../../docs/site", "build", "--force", "--output-dir", outputDir)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build searchable references: %v\n%s", err, output)
	}
	var entries []struct {
		URL  string `json:"url"`
		Body string `json:"body"`
	}
	index := readBuiltFile(t, filepath.Join(outputDir, "search-index", "index.html"))
	if err := json.Unmarshal([]byte(index), &entries); err != nil {
		t.Fatalf("parse reference search index: %v", err)
	}
	for _, section := range []struct{ fragment, text string }{
		{"/cli-reference/hook-diagnostics/#sample-event-inputs", "placeholder values"},
		{"/cli-reference/hook-diagnostics/#cline-sample-events", "model whose id holds"},
		{"/cli-reference/hook-diagnostics/#matcher-and-plugin-limits", "does not simulate their runtime"},
		{"/spec-format/environments/#claude-cleanup-limits", "1.5-second budget"},
		{"/memory/#claude-code-s-own-memory", "autoMemoryDirectory"},
	} {
		found := false
		for _, entry := range entries {
			if strings.HasSuffix(entry.URL, section.fragment) && strings.Contains(entry.Body, section.text) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("search lost %q from %s", section.text, section.fragment)
		}
	}
}
