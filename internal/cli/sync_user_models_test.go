package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncUser_ResolvesModelTiersFromHomeConfig(t *testing.T) {
	home, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "agnostic-ai.yaml"),
		"version: 1\ntargets: [claude, codex]\nmodels:\n  strong: {claude: sonnet, codex: gpt-5.4}\n  fast: {claude: haiku}\n")
	writeFile(t, filepath.Join(source, "local", "agnostic-ai.yaml"),
		"version: 1\nmodels:\n  strong: {claude: opus, effort: {claude: xhigh}}\n")
	writeFile(t, filepath.Join(source, "agents", "reviewer.md"),
		"---\nname: reviewer\ndescription: Review code\nmodel: strong\n---\nReview.\n")

	_, errOut, err := runUserCheck("sync")
	if err != nil {
		t.Fatalf("sync --scope user: %v\n%s", err, errOut)
	}
	if strings.Contains(errOut, "ignoring models") {
		t.Errorf("models must be read in user scope:\n%s", errOut)
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
	if strings.Contains(string(codex), "model =") {
		t.Errorf("the local tier replaces the shared one whole, so codex keeps its default:\n%s", codex)
	}
}

func TestSyncUser_TargetFlagSkipsOnlyTheBrokenTierFile(t *testing.T) {
	home, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "agnostic-ai.yaml"),
		"version: 1\ntargets: [claude]\nmodels:\n  strong: {claude: opus}\n")
	local := filepath.Join(source, "local", "agnostic-ai.yaml")
	writeFile(t, local, "version: 1\nmodels:\n  fast: {claud: haiku}\n")
	writeFile(t, filepath.Join(source, "agents", "reviewer.md"),
		"---\nname: reviewer\ndescription: Review code\nmodel: strong\n---\nReview.\n")
	writeFile(t, filepath.Join(source, "agents", "scout.md"),
		"---\nname: scout\ndescription: Scout code\nmodel: fast\n---\nScout.\n")

	if _, _, err := runUserCheck("sync"); err == nil || !strings.Contains(err.Error(), `unknown target "claud"`) {
		t.Fatalf("a broken tier stops a plain sync --scope user, got %v", err)
	}
	_, errOut, err := runUserCheck("sync", "-t", "claude")
	if err != nil {
		t.Fatalf("sync --scope user -t claude: %v\n%s", err, errOut)
	}
	for _, want := range []string{local + `: models.fast: unknown target "claud"`, `scout.md: model "fast" names a tier`} {
		if !strings.Contains(errOut, want) {
			t.Errorf("missing warning %q:\n%s", want, errOut)
		}
	}
	reviewer, err := os.ReadFile(filepath.Join(home, ".claude", "agents", "reviewer.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(reviewer), "model: opus\n") {
		t.Errorf("tiers from the good file must still resolve:\n%s", reviewer)
	}
	scout, err := os.ReadFile(filepath.Join(home, ".claude", "agents", "scout.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(scout), "model:") {
		t.Errorf("an unresolved tier name must not become a model id:\n%s", scout)
	}
}

func TestLintUser_ChecksHomeModelTiers(t *testing.T) {
	_, source := userAgentTestHome(t)
	config := filepath.Join(source, "agnostic-ai.yaml")
	writeFile(t, config, "version: 1\ntargets: [claude, codex]\nmodels:\n  balanced: {claude: sonnet}\n")
	writeFile(t, filepath.Join(source, "agents", "reviewer.md"),
		"---\nname: reviewer\ndescription: Review code\nmodel: balanced\n---\nReview.\n")

	out, _, _ := runUserCheck("lint")
	if want := "LINT025 [warn] " + config + ": models.balanced has no codex model and no default"; !strings.Contains(out, want) {
		t.Errorf("missing %q:\n%s", want, out)
	}
}
