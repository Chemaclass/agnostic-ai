package cli

import (
	"strings"
	"testing"
)

func TestLint_WarnsOnRulesModeTheTargetDoesNotRead(t *testing.T) {
	cases := map[string]string{
		"typo":                 "targets: [trae]\noutputs:\n  trae:\n    rules-mode: entrypoint\n",
		"value of another one": "targets: [claude]\noutputs:\n  claude:\n    rules-mode: entry-point\n",
		"target with no modes": "targets: [codex]\noutputs:\n  codex:\n    rules-mode: import\n",
	}
	for name, config := range cases {
		t.Run(name, func(t *testing.T) {
			budgetProject(t, config)

			out, _ := runCLI(t, "lint")

			lines := findingLines(out, "LINT061")
			if len(lines) != 1 || !strings.Contains(lines[0], "rules-mode") {
				t.Errorf("want one LINT061 naming rules-mode, got:\n%s", out)
			}
		})
	}
}

func TestLint_AcceptsTheRulesModesTargetsRead(t *testing.T) {
	budgetProject(t, "targets: [claude, trae]\noutputs:\n  claude:\n    rules-mode: import\n  trae:\n    rules-mode: entry-point\n")

	out, err := runCLI(t, "lint")

	if strings.Contains(out, "LINT061") {
		t.Errorf("claude import and trae entry-point are valid:\n%s", out)
	}
	if err != nil && strings.Contains(err.Error(), "rules-mode") {
		t.Errorf("a valid rules-mode must load: %v", err)
	}
}
