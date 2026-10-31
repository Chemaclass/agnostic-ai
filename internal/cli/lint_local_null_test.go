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

func TestLint_LocalNullMessageFollowsSharedValueKind(t *testing.T) {
	cases := []struct {
		name, base, local string
		want, wantNot     string
	}{
		{"group", "outputs:\n  codex:\n    config:\n      sandbox: read-only\n", "outputs:\n", "ignored", "replaces"},
		{"list", "gitignore:\n  commit: [claude:instructions]\n", "gitignore:\n  commit:\n", "replaces the shared value", "ignored"},
		{"single value", "gitignore:\n  enabled: true\n", "gitignore:\n  enabled:\n", "replaces the shared value", "ignored"},
		{"model tier", "models:\n  fast: {codex: gpt-5}\n", "models:\n  fast:\n", "remove the key", "{}"},
		{"null in base too", "gitignore:\n  commit:\n", "gitignore:\n  commit:\n", "", "LINT055"},
		{"requires clears the check", "requires: \">=99.0.0\"\n", "requires:\n", "", "LINT055"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n"+tc.base)
			mustWriteFile(t, "agnostic-ai.local.yaml", tc.local)
			var messages []string
			report, _, _ := runLintJSON(t)
			for _, finding := range report.Findings {
				if finding.Code == "LINT055" {
					messages = append(messages, finding.Message)
				}
			}
			joined := strings.Join(messages, "\n")
			if tc.want == "" && len(messages) > 0 {
				t.Fatalf("unexpected warning: %s", joined)
			}
			if tc.want != "" && !strings.Contains(joined, tc.want) {
				t.Errorf("want %q in %q", tc.want, joined)
			}
			if tc.wantNot != "" && strings.Contains(joined, tc.wantNot) {
				t.Errorf("did not want %q in %q", tc.wantNot, joined)
			}
		})
	}
}

func TestLint_LocalNullReadsLegacyBaseConfig(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic.config.yaml", "version: 1\ntargets: [codex]\ngitignore:\n  commit: [codex:instructions]\n")
	mustWriteFile(t, "agnostic-ai.local.yaml", "gitignore:\n  commit:\n")
	report, _, _ := runLintJSON(t)
	for _, finding := range report.Findings {
		if finding.Code == "LINT055" {
			return
		}
	}
	t.Errorf("no LINT055 for legacy base: %+v", report.Findings)
}
