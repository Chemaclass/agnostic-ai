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
	for _, f := range lintDevCommands(envs, []string{"claude"}) {
		if f.Code != "LINT016" || f.Severity != lintError {
			t.Errorf("finding %+v, want LINT016 error", f)
		}
		got = append(got, f.Path+": "+f.Message)
	}
	want := []string{
		"environments/bad.yaml: dev command 1 has no `name:`",
		`environments/bad.yaml: dev command "Api" appears twice; names must be unique`,
		"environments/bad.yaml: dev command 3 has no `command:` or `url:`",
		"environments/bad.yaml: dev command 4 is not a mapping with `name:` and `command:`",
		"environments/null.yaml: `dev-commands` is empty; write `dev-commands: []` to clear an earlier spec's list",
		"environments/scalar.yaml: `dev-commands` must be a list of commands",
		"environments/fields.yaml: dev command 1 has no `command:` or `url:`",
		"environments/fields.yaml: dev command 2 sets `port: web`; use a number",
		"environments/fields.yaml: dev command 2 sets `env.LIST` to a list or mapping; use a string",
		"environments/fields.yaml: dev command 3 sets `env` to something other than a mapping of names to values",
		"environments/fields.yaml: dev command 4 has no `command:` or `url:`",
		"environments/fields.yaml: dev command 5 sets `auto-port` to a value of the wrong type",
		"environments/fields.yaml: dev command 5 sets `autoport`, which no target reads",
		"environments/fields.yaml: dev command 5 sets `cwd` to a value of the wrong type",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("findings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// A dev command with a url and no command is valid for Claude, which
// attaches to a server already running. Only an enabled target that
// needs a command reports it, in the list that target keeps (#2579).
func TestLintDevCommands_URLWithoutCommandFailsOnlyWhereACommandIsNeeded(t *testing.T) {
	commands := []any{map[string]any{"name": "app", "url": "https://app.localhost:3000"}}
	codexOwn := map[string]any{"dev-commands": []any{map[string]any{"name": "web", "command": "npm run dev"}}}
	dev := spec.Entry{Kind: spec.KindEnvironment, Path: "environments/dev.yaml", Meta: map[string]any{"dev-commands": commands}}
	claudeOnly := spec.Entry{Kind: spec.KindEnvironment, Path: "environments/claude.yaml", Meta: map[string]any{"targets": []any{"claude"}, "dev-commands": commands}}
	split := spec.Entry{Kind: spec.KindEnvironment, Path: "environments/split.yaml", Meta: map[string]any{"dev-commands": commands, "x-codex": codexOwn}}

	if got := lintDevCommands([]spec.Entry{dev, claudeOnly}, []string{"claude"}); len(got) != 0 {
		t.Errorf("claude only: findings %+v, want none", got)
	}
	if got := lintDevCommands([]spec.Entry{dev, split}, []string{"claude", "codex"}); len(got) != 0 {
		t.Errorf("a later spec gives codex its own list: findings %+v, want none", got)
	}
	got := lintDevCommands([]spec.Entry{split, dev, claudeOnly}, []string{"claude", "codex"})
	if len(got) != 1 {
		t.Fatalf("codex keeps the url-only list: findings %+v, want one", got)
	}
	want := "dev command 1 has a `url:` and no `command:`, which codex needs; add a command, or give codex its own list under `x-codex.dev-commands`"
	if got[0].Code != "LINT016" || got[0].Severity != lintError || got[0].Path != "environments/dev.yaml" || got[0].Message != want {
		t.Errorf("finding %+v\nwant LINT016 error on environments/dev.yaml: %s", got[0], want)
	}
}
