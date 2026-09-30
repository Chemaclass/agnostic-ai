package emit

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestCodexGlobRules_FallbackNamesAlwaysLoadedRule(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	for _, selector := range []string{"Dockerfile", "src/api/**/*.ts", "Dockerfile,scripts/*.sh,.kamal/**", "**"} {
		t.Run(selector, func(t *testing.T) {
			ResetCoverageNotes()
			t.Cleanup(ResetCoverageNotes)
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "deployment", Path: "rules/deployment.md", Body: "Deploy safely.", Meta: map[string]any{"globs": selector}}})
			cfg := &config.Config{Targets: []string{"codex"}, OnUnsupported: "warn"}
			out, files, err := PrepareScopedRules(b, cfg, "codex")
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != 0 || len(out.Rules) != 1 {
				t.Errorf("fallback must keep the entire rule inline: %+v, %+v", out, files)
			}
			if PendingCoverageNotesCount() == 0 {
				t.Error("fallback lacks an always-loaded coverage note")
			}
			buf := swapWarnerForNotes(t)
			_, _, _ = PrepareScopedRules(b, cfg, "codex")
			FlushCoverageNotes()
			if !strings.Contains(buf.String(), "deployment") || !strings.Contains(buf.String(), "always loaded from AGENTS.md") {
				t.Errorf("note lacks rule and destination: %s", buf.String())
			}
			cfg.OnUnsupported = "error"
			_, _, err = PrepareScopedRules(b, cfg, "codex")
			if err == nil || !strings.Contains(err.Error(), "deployment") || !strings.Contains(err.Error(), "always loaded") {
				t.Errorf("expected named always-loaded error, got %v", err)
			}
		})
	}
}

func TestCodexGlobRules_RootDeliveryRemainsIntentional(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	no := false
	for _, tc := range []struct {
		name string
		cfg  config.Config
		meta map[string]any
		note bool
	}{
		{"always apply", config.Config{Targets: []string{"codex"}}, map[string]any{"globs": "src/**", "alwaysApply": true}, false},
		{"opt out", config.Config{Targets: []string{"codex"}, Outputs: map[string]config.Output{"codex": {NestedGlobRules: &no}}}, map[string]any{"globs": "src/**"}, false},
		{"legacy rules file", config.Config{Targets: []string{"codex"}, Outputs: map[string]config.Output{"codex": {RulesFile: "legacy.md"}}}, map[string]any{"globs": "src/**"}, true},
		{"custom root file", config.Config{Targets: []string{"codex"}, Outputs: map[string]config.Output{"codex": {File: "custom.md"}}}, map[string]any{"globs": "src/**"}, true},
		{"custom cursor rules directory", config.Config{Targets: []string{"codex", "cursor"}, Outputs: map[string]config.Output{"cursor": {RulesDir: ".cursor/custom-rules"}}}, map[string]any{"globs": "src/**"}, true},
		{"shared always-on reader", config.Config{Targets: []string{"codex", "crush"}}, map[string]any{"globs": "src/**"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ResetCoverageNotes()
			t.Cleanup(ResetCoverageNotes)
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "rule", Body: "Keep convention.", Meta: tc.meta}})
			out, files, err := PrepareScopedRules(b, &tc.cfg, "codex")
			if err != nil {
				t.Fatal(err)
			}
			if len(out.Rules) != 1 || len(files) != 0 || len(EntryPointRules(b, "codex", &tc.cfg).Rules) != 1 {
				t.Errorf("root delivery changed: %+v, %+v", out, files)
			}
			if got := PendingCoverageNotesCount() > 0; got != tc.note {
				t.Errorf("note = %t, want %t", got, tc.note)
			}
		})
	}
}
