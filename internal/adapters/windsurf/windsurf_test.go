package windsurf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_WritesRulesAndAgents(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	a := New()
	if a.Name() != "windsurf" {
		t.Errorf("expected windsurf, got %s", a.Name())
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule"},
		{Kind: spec.KindAgent, Name: "ag1", Body: "agent"},
		{Kind: spec.KindSkill, Name: "sk1", Body: "skill"},
	}
	if err := a.Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{".windsurf/rules/r1.md", ".windsurf/rules/agent-ag1.md", ".windsurf/rules/skill-sk1.md"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("missing %s", p)
		}
	}
}

func TestEmit_WorkflowsDirEmitsAgentsAsWorkflows(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent,
			Name: "ship-it",
			Meta: map[string]any{"description": "open and merge a PR"},
			Body: "Run the release.",
		},
		{Kind: spec.KindRule, Name: "r1", Body: "rule body"},
	}
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"windsurf": {WorkflowsDir: ".windsurf/workflows"},
		},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}

	wfPath := filepath.Join(dir, ".windsurf/workflows/ship-it.md")
	got, err := os.ReadFile(wfPath)
	if err != nil {
		t.Fatalf("missing workflow: %v", err)
	}
	body := string(got)
	if !strings.Contains(body, "description: open and merge a PR") {
		t.Errorf("workflow missing description frontmatter: %q", body)
	}
	if !strings.Contains(body, "Run the release.") {
		t.Errorf("workflow missing body: %q", body)
	}

	// Rule-form emission still happens for back-compat.
	if _, err := os.Stat(filepath.Join(dir, ".windsurf/rules/agent-ship-it.md")); err != nil {
		t.Errorf("rule-form agent missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".windsurf/rules/r1.md")); err != nil {
		t.Errorf("rule missing: %v", err)
	}
}

func TestEmit_NoWorkflowsDirNoEmit(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "ag1", Body: "agent"}}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".windsurf/workflows")); !os.IsNotExist(err) {
		t.Errorf("expected no workflows dir; err=%v", err)
	}
}
