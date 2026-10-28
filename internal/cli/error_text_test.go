package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

func TestErrorText_AddsTheRegistryFixToACodedError(t *testing.T) {
	err := errs.Coded(errs.CodeConfigMissing, "read config: no agnostic-ai.yaml in .")

	got := ErrorText(err)

	if !strings.HasPrefix(got, "[AAI-003] read config") || !strings.Contains(got, "\n  fix: Run `agnostic-ai init`") {
		t.Errorf("got %q", got)
	}
}

func TestErrorText_LeavesAPlainErrorAlone(t *testing.T) {
	if got := ErrorText(errors.New("boom")); got != "boom" {
		t.Errorf("got %q", got)
	}
}

func TestErrorText_RequiresKeepsOnlyTheContextualFix(t *testing.T) {
	setRunningVersion(t, "v0.89.0")
	cases := []struct {
		name     string
		requires string
		global   bool
		fix      string
	}{
		{"newer project binary", "0.88.0", false, "`agnostic-ai upgrade --requires`"},
		{"older project binary", ">=0.90.0", false, "`agnostic-ai upgrade`"},
		{"newer global binary", "0.88.0", true, "update `requires` and the schema"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := requireVersionInScope("agnostic-ai.yaml", c.requires, c.global)
			if err == nil {
				t.Fatal("expected an unmet version requirement")
			}
			got := ErrorText(err)
			if got != err.Error() {
				t.Errorf("appended generic guidance to a contextual fix:\n%s", got)
			}
			if strings.Count(got, c.fix) != 1 {
				t.Errorf("want one contextual fix %q, got:\n%s", c.fix, got)
			}
			for _, unwanted := range []string{"PATH shadows", "For an older binary", "global home config"} {
				if strings.Contains(got, unwanted) {
					t.Errorf("unrelated guidance %q in:\n%s", unwanted, got)
				}
			}
		})
	}
}
