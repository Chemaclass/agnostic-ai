package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestLint_WarnsWhenLocalNullClearsSharedSettings(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\noutputs:\n  codex:\n    config:\n      sandbox: read-only\ngitignore:\n  enabled: true\n")
	mustWriteFile(t, "agnostic-ai.local.yaml", "outputs:\n  codex:\n    config:\ngitignore:\n")
	report, _, err := runLintJSON(t)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, finding := range report.Findings {
		if finding.Code == "LINT055" && finding.Path == "agnostic-ai.local.yaml" {
			for _, key := range []string{"outputs.codex.config", "gitignore"} {
				if strings.Contains(finding.Message, key) {
					found[key] = true
				}
			}
		}
	}
	if !found["outputs.codex.config"] || !found["gitignore"] {
		t.Errorf("missing local null warnings: %+v", report.Findings)
	}
	if _, _, err := runLintJSON(t, "--strict"); err == nil {
		t.Error("strict lint accepted null overrides")
	}
	if err := os.Remove(filepath.Join(dir, "agnostic-ai.local.yaml")); err != nil {
		t.Fatal(err)
	}
	report, _, err = runLintJSON(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range report.Findings {
		if finding.Code == "LINT055" {
			t.Errorf("warning without local file: %+v", finding)
		}
	}
}
