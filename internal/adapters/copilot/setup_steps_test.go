package copilot

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const setupGoCommit = "d8cbb8045d7f43706b9177cceb36c1f91a6644d5"

func renderSetup(t *testing.T, options map[string]any) string {
	t.Helper()
	raw, err := renderSetupWorkflow(setupWorkflow("npm ci", options))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return string(raw)
}

func TestSetupWorkflow_DefaultCheckoutCarriesItsVersionComment(t *testing.T) {
	out := renderSetup(t, nil)
	want := "- uses: actions/checkout@" + checkoutCommit + " # " + checkoutVersion + "\n"
	if !strings.Contains(out, want) {
		t.Errorf("missing %q in:\n%s", want, out)
	}
}

func TestSetupWorkflow_KeepsVersionCommentsFromQuotedUses(t *testing.T) {
	override := "df4cb1c069e1874edd31b4311f1884172cec0e10"
	out := renderSetup(t, map[string]any{
		"checkout": override + " # v6.0.2",
		"steps": []any{
			map[string]any{
				"uses": "actions/setup-go@" + setupGoCommit + " # v7",
				"with": map[string]any{"go-version-file": "go.mod"},
			},
			map[string]any{"uses": "actions/setup-node@v4"},
			map[string]any{"run": "echo '# not a version'"},
		},
	})
	for _, want := range []string{
		"- uses: actions/checkout@" + override + " # v6.0.2\n",
		"- uses: actions/setup-go@" + setupGoCommit + " # v7\n",
		"- uses: actions/setup-node@v4\n",
		"- run: echo '# not a version'\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	var doc struct {
		Jobs map[string]struct{ Steps []map[string]any }
	}
	if err := yaml.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	steps := doc.Jobs["copilot-setup-steps"].Steps
	if got := steps[1]["uses"]; got != "actions/setup-go@"+setupGoCommit {
		t.Errorf("uses parsed as %q, want the bare ref", got)
	}
}

func TestSetupWorkflow_CheckoutOverrideWithoutVersionHasNoComment(t *testing.T) {
	override := "df4cb1c069e1874edd31b4311f1884172cec0e10"
	out := renderSetup(t, map[string]any{"checkout": override})
	if !strings.Contains(out, "- uses: actions/checkout@"+override+"\n") {
		t.Errorf("unexpected checkout line in:\n%s", out)
	}
}

func TestSetupStepsIssues_AcceptsVersionCommentForm(t *testing.T) {
	entry := func(options map[string]any) spec.Entry {
		return spec.Entry{Meta: map[string]any{"x-copilot": map[string]any{"setup-steps": options}}}
	}
	valid := map[string]any{
		"checkout": checkoutCommit + " # v7",
		"steps":    []any{map[string]any{"uses": "actions/setup-go@" + setupGoCommit + " # v7"}},
	}
	if issues := SetupStepsIssues(entry(valid)); len(issues) != 0 {
		t.Errorf("unexpected issues: %v", issues)
	}
	invalid := map[string]any{
		"checkout": "v7 # v7",
		"steps":    []any{map[string]any{"uses": "actions/setup-go@" + setupGoCommit + " #"}},
	}
	issues := SetupStepsIssues(entry(invalid))
	if len(issues) != 2 {
		t.Fatalf("want 2 issues, got %v", issues)
	}
	for i, field := range []string{"checkout", "steps"} {
		if !strings.HasPrefix(issues[i], "x-copilot.setup-steps."+field+" ") {
			t.Errorf("issue %d = %q, want %s", i, issues[i], field)
		}
	}
}
