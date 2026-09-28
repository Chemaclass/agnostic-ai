package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestLintDevCommands(t *testing.T) {
	envs := []spec.Entry{
		{Kind: spec.KindEnvironment, Path: "environments/ok.yaml", Meta: map[string]any{
			"dev-commands": []any{map[string]any{"name": "Web", "command": "npm run dev"}},
		}},
		{Kind: spec.KindEnvironment, Path: "environments/bad.yaml", Meta: map[string]any{
			"dev-commands": []any{
				map[string]any{"command": "npm run dev"},
				map[string]any{"name": "Api", "command": []any{"go", "run", "."}},
				map[string]any{"name": "Api"},
				"npm start",
			},
		}},
		{Kind: spec.KindEnvironment, Path: "environments/scalar.yaml", Meta: map[string]any{"dev-commands": "npm run dev"}},
	}
	var got []string
	for _, f := range lintDevCommands(envs) {
		if f.Code != "LINT016" || f.Severity != lintError {
			t.Errorf("finding %+v, want LINT016 error", f)
		}
		got = append(got, f.Path+": "+f.Message)
	}
	want := []string{
		"environments/bad.yaml: dev command 1 has no `name:`",
		`environments/bad.yaml: dev command "Api" appears twice; names must be unique`,
		"environments/bad.yaml: dev command 3 has no `command:`",
		"environments/bad.yaml: dev command 4 is not a mapping with `name:` and `command:`",
		"environments/scalar.yaml: `dev-commands` must be a list of commands",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("findings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
