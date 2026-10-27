package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

const duplicatedPassage = "Always run the full test suite before you push any branch to the remote."

func TestExplainContext_FindsExactDuplicatedStartupPassages(t *testing.T) {
	dir := budgetProject(t, "targets: [claude, codex]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "## Testing\n\n"+duplicatedPassage+"\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\n---\n## Testing\n\n"+duplicatedPassage+"\n\nKeep functions small and name them after what they return.\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "similar.md"), "---\nname: similar\n---\nAlways run the full test suite before you push a branch to the remote.\n")
	report := contextJSON(t, "claude")
	if len(report.Opportunities) != 1 {
		t.Fatalf("opportunities: %+v", report.Opportunities)
	}
	o := report.Opportunities[0]
	size := contextSize{Words: wordsIn(duplicatedPassage), Bytes: len(duplicatedPassage)}
	if o.Kind != "duplicate-passage" || o.Target != "claude" || o.Load != "startup" || o.Passage != duplicatedPassage || o.Copies != 2 || o.Size != size || o.Reduction != size {
		t.Errorf("opportunity: %+v", o)
	}
	var sources []string
	for _, s := range o.Sources {
		sources = append(sources, s.Source)
	}
	if strings.Join(sources, ",") != ".agnostic-ai/AGNOSTIC_AI.md,.agnostic-ai/rules/style.md" {
		t.Errorf("sources: %v", sources)
	}
	if !strings.Contains(o.Suggestion, "Review") {
		t.Errorf("suggestion: %q", o.Suggestion)
	}
	var startup, onDemand contextSize
	for _, c := range report.Contributions {
		switch c.Load {
		case "startup":
			startup.Words += c.Words
			startup.Bytes += c.Bytes
		case "on-demand":
			onDemand.Words += c.Words
			onDemand.Bytes += c.Bytes
		}
	}
	if startup != report.Startup || onDemand != report.OnDemand {
		t.Errorf("totals: startup %+v vs %+v, on-demand %+v vs %+v", report.Startup, startup, report.OnDemand, onDemand)
	}

	text, err := runCLI(t, "explain", "--context", "--target", "claude")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Opportunities:", "duplicate passage, 2 copies of 14 words", ".agnostic-ai/AGNOSTIC_AI.md, .agnostic-ai/rules/style.md", duplicatedPassage} {
		if !strings.Contains(text, want) {
			t.Errorf("text missing %q:\n%s", want, text)
		}
	}

	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\n---\n## Testing\n\nKeep functions small and name them after what they return.\n")
	after := contextJSON(t, "claude")
	if len(after.Opportunities) != 0 {
		t.Errorf("after removing a copy: %+v", after.Opportunities)
	}
	if report.Startup.Words-after.Startup.Words != size.Words {
		t.Errorf("startup dropped %d words, want %d", report.Startup.Words-after.Startup.Words, size.Words)
	}
	text, err = runCLI(t, "explain", "--context", "--target", "claude")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Opportunities: none found") {
		t.Errorf("text after refresh:\n%s", text)
	}
}

func TestExplainContext_IgnoresDuplicatesOutsideStartup(t *testing.T) {
	dir := budgetProject(t, "targets: [claude]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), duplicatedPassage+"\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "ship", "SKILL.md"), "---\nname: ship\ndescription: Ship a branch.\n---\n"+duplicatedPassage+"\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "go.md"), "---\nname: go\nglobs: '**/*.go'\nalwaysApply: false\n---\n"+duplicatedPassage+"\n")
	if report := contextJSON(t, "claude"); len(report.Opportunities) != 0 {
		t.Errorf("on-demand and scoped copies counted: %+v", report.Opportunities)
	}
}

func contextJSON(t *testing.T, target string) explainContextOutput {
	t.Helper()
	out, err := runCLI(t, "explain", "--context", "--target", target, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var report explainContextOutput
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.Opportunities == nil {
		t.Error("opportunities must be an array, not null")
	}
	return report
}

// VS Code reading both copilot-instructions.md and AGENTS.md loads one spec
// twice; that is a setting to change, not a copy to remove from the specs.
func TestExplainContext_CountsCopiesPerSourceNotPerGeneratedFile(t *testing.T) {
	dir := budgetProject(t, "targets: [copilot, codex]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), duplicatedPassage+"\n")
	mustWriteFile(t, filepath.Join(dir, vscodeSettings), "{\n  \"chat.useAgentsMdFile\": true\n}\n")
	report := contextJSON(t, "copilot")
	loaded := 0
	for _, c := range report.Contributions {
		if c.Source == ".agnostic-ai/AGNOSTIC_AI.md" && c.Load == "startup" {
			loaded++
		}
	}
	if loaded < 2 {
		t.Fatalf("setup: the entry point should load through both files, got %d", loaded)
	}
	if len(report.Opportunities) != 0 {
		t.Errorf("one source loaded twice reported as a copy: %+v", report.Opportunities)
	}

	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), duplicatedPassage+"\n\n"+duplicatedPassage+"\n")
	if report := contextJSON(t, "copilot"); len(report.Opportunities) != 1 || report.Opportunities[0].Copies != 2 {
		t.Errorf("a passage written twice in one file: %+v", report.Opportunities)
	}
}
