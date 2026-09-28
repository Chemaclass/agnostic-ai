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
		{Kind: spec.KindEnvironment, Path: "environments/null.yaml", Meta: map[string]any{"dev-commands": nil}},
		{Kind: spec.KindEnvironment, Path: "environments/clear.yaml", Meta: map[string]any{"dev-commands": []any{}}},
		{Kind: spec.KindEnvironment, Path: "environments/scalar.yaml", Meta: map[string]any{"dev-commands": "npm run dev"}},
		{Kind: spec.KindEnvironment, Path: "environments/fields.yaml", Meta: map[string]any{
			"x-claude": map[string]any{"dev-commands": []any{
				map[string]any{"name": "Blank", "command": "   "},
				map[string]any{"name": "Web", "command": "npm start", "port": "web", "env": map[string]any{"PORT": 3000, "LIST": []any{"a"}}},
				map[string]any{"name": "Flat", "command": "npm start", "env": "NODE_ENV=dev"},
				map[string]any{"name": "Nested", "command": []any{"npm", []any{"run"}}},
				map[string]any{"name": "Typos", "command": "npm start", "cwd": []any{"a"}, "autoport": true, "auto-port": "yes"},
			}},
		}},
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
		"environments/null.yaml: `dev-commands` is empty; write `dev-commands: []` to clear an earlier spec's list",
		"environments/scalar.yaml: `dev-commands` must be a list of commands",
		"environments/fields.yaml: dev command 1 has no `command:`",
		"environments/fields.yaml: dev command 2 sets `port: web`; use a number",
		"environments/fields.yaml: dev command 2 sets `env.LIST` to a list or mapping; use a string",
		"environments/fields.yaml: dev command 3 sets `env` to something other than a mapping of names to values",
		"environments/fields.yaml: dev command 4 has no `command:`",
		"environments/fields.yaml: dev command 5 sets `auto-port` to a value of the wrong type",
		"environments/fields.yaml: dev command 5 sets `autoport`, which no target reads",
		"environments/fields.yaml: dev command 5 sets `cwd` to a value of the wrong type",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("findings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
