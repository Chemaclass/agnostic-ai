package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncGlobal_ResolvesModelTiersFromHomeConfig(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"),
		"version: 1\ntargets: [claude, codex]\nmodels:\n  strong: {claude: sonnet, codex: gpt-5.4}\n  fast: {claude: haiku}\n")
	mustWriteGlobalTest(t, filepath.Join(source, "local", "agnostic-ai.yaml"),
		"version: 1\nmodels:\n  strong: {claude: opus, codex: gpt-5.5, effort: {claude: xhigh}}\n")
	mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"),
		"---\nname: reviewer\ndescription: Review code\nmodel: strong\n---\nReview.\n")

	_, errOut, err := runGlobalCheck("sync")
	if err != nil {
		t.Fatalf("sync --global: %v\n%s", err, errOut)
	}
	if strings.Contains(errOut, "ignoring models") {
		t.Errorf("models must be read in global mode:\n%s", errOut)
	}
	claude, err := os.ReadFile(filepath.Join(home, ".claude", "agents", "reviewer.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"model: opus\n", "effort: xhigh\n"} {
		if !strings.Contains(string(claude), want) {
			t.Errorf("claude agent lacks %q:\n%s", want, claude)
		}
	}
	codex, err := os.ReadFile(filepath.Join(home, ".codex", "agents", "reviewer.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(codex), `model = "gpt-5.5"`) || strings.Contains(string(codex), "strong") {
		t.Errorf("codex agent must get the local tier's model:\n%s", codex)
	}
}
