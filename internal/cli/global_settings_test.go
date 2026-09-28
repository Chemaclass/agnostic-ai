package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestEditTOMLRoot_SkipsBracketsInsideValues(t *testing.T) {
	in := "notify = [\n  \"say\",\n  \"[done]\",\n]\nbanner = '''\n[not a table]\n'''\n\n[tui]\nmodel = \"keep\"\n"
	got, err := editTOMLRoot("config.toml", []byte(in), []string{"model"}, map[string]any{"model": "gpt-6-luna"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "notify = [\n  \"say\",\n  \"[done]\",\n]\nbanner = '''\n[not a table]\n'''\nmodel = \"gpt-6-luna\"\n\n[tui]\nmodel = \"keep\"\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestEditTOMLRoot_TopWhenNoRootKeys(t *testing.T) {
	in := "# header\n[projects.x]\ntrust_level = \"trusted\"\n"
	got, err := editTOMLRoot("config.toml", []byte(in), []string{"model"}, map[string]any{"model": "m"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "model = \"m\"\n\n# header\n[projects.x]\ntrust_level = \"trusted\"\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEditJSONRoot_RoundTripsByteForByte(t *testing.T) {
	for _, in := range []string{
		"{\n    \"theme\": \"dark\", // mine\n    \"hooks\": {\"a\": [1, 2]}\n}\n",
		"{\"theme\": \"dark\"}",
		"{}\n",
	} {
		set, err := editJSONRoot("s.json", []byte(in), []string{"model"}, map[string]any{"model": "opus"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		values, err := settingsValues("s.json", "json", set)
		if err != nil || values["model"] != "opus" {
			t.Fatalf("set %q -> %q: %v %v", in, set, values, err)
		}
		back, err := editJSONRoot("s.json", set, nil, nil, []string{"model"})
		if err != nil {
			t.Fatal(err)
		}
		if string(back) != in {
			t.Errorf("round trip of %q gave %q via %q", in, back, set)
		}
	}
}

func TestEditJSONRoot_ReplacesValueInPlace(t *testing.T) {
	in := "{\n  \"model\": \"sonnet\", // pinned\n  \"x\": 1\n}\n"
	got, err := editJSONRoot("s.json", []byte(in), []string{"model"}, map[string]any{"model": "opus"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"model\": \"opus\", // pinned\n  \"x\": 1\n}\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLintGlobal_FlagsSettingsEffortATargetRejects(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "defaults.yaml"), "effort:\n  claude: max\n  codex: max\n")

	var out bytes.Buffer
	cmd := NewRootCmd("test")
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"lint", "--global"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("lint must fail:\n%s", out.String())
	}
	got := out.String()
	if !strings.Contains(got, "LINT014") || !strings.Contains(got, "claude: effortLevel does not accept effort max") {
		t.Errorf("lint output:\n%s", got)
	}
	if strings.Contains(got, "codex:") || strings.Contains(got, "LINT001") {
		t.Errorf("codex takes any string and a settings spec is never empty:\n%s", got)
	}
}

func TestExplainGlobal_NamesFileAndKeyPerTarget(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "defaults.yaml"), globalDefaultsSettings)
	mustWriteGlobalTest(t, filepath.Join(source, "local", "settings", "machine.yaml"), "model:\n  codex: gpt-6-mini\n")

	var out bytes.Buffer
	cmd := NewRootCmd("test")
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"explain", "--global", "settings/defaults.yaml"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("explain: %v\n%s", err, out.String())
	}
	got := out.String()
	codex := filepath.ToSlash(filepath.Join(home, ".codex", "config.toml"))
	for _, want := range []string{
		`[claude] ` + filepath.ToSlash(filepath.Join(home, ".claude", "settings.json")) + ` (key "model")`,
		`(key "effortLevel")`,
		`[codex] ` + codex + ` (key "model_reasoning_effort")`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("explain lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, codex+` (key "model")`) {
		t.Errorf("the local layer's codex model wins, so defaults.yaml must not claim it:\n%s", got)
	}
}

func TestEditJSONRoot_ReviewEdgeCases(t *testing.T) {
	cases := []struct {
		name, in, want string
		set            map[string]any
		remove         []string
	}{
		{
			name:   "brace in a comment before the only member",
			in:     "{\n  // see {docs}\n  \"model\": \"x\"\n}\n",
			remove: []string{"model"},
			want:   "{\n  // see {docs}\n\n}\n",
		},
		{
			name:   "comment after the previous value stays",
			in:     "{\"a\": 1, // c\n \"model\": \"x\"}",
			remove: []string{"model"},
			want:   "{\"a\": 1 // c\n}",
		},
		{
			name: "duplicate key edits the copy JSON reads",
			in:   `{"model":"a","x":1,"model":"b"}`,
			set:  map[string]any{"model": "c"},
			want: `{"model":"a","x":1,"model":"c"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var order []string
			for key := range tc.set {
				order = append(order, key)
			}
			got, err := editJSONRoot("s.json", []byte(tc.in), order, tc.set, tc.remove)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEditTOMLRoot_KeepsBOMAndMixedLineEndings(t *testing.T) {
	in := "\xef\xbb\xbfmodel = \"a\"\r\nb = 2\n[t]\r\nx = 1\r\n"
	got, err := editTOMLRoot("config.toml", []byte(in), []string{"model"}, map[string]any{"model": "b"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "\xef\xbb\xbfmodel = \"b\"\r\nb = 2\n[t]\r\nx = 1\r\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
	got, err = editTOMLRoot("config.toml", []byte("a = 1\r\n"), []string{"model"}, map[string]any{"model": "m"}, []string{"missing"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "a = 1\r\nmodel = \"m\"\r\n"; string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExplainGlobal_NamesEveryKindsUserPath(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude, codex, augment]\n")
	mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Review\ntargets: [claude]\n---\nReview.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "tidy", "SKILL.md"), "---\nname: tidy\ndescription: Tidy\n---\nTidy.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "rules", "safe.md"), "---\nname: safe\n---\nBe safe.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "fmt.yaml"), "event: PostToolUse\ncommand: gofmt -l .\n")

	explain := func(arg string) string {
		t.Helper()
		var out bytes.Buffer
		cmd := NewRootCmd("test")
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"explain", "--global", arg})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("explain %s: %v\n%s", arg, err, out.String())
		}
		return out.String()
	}
	p := func(parts ...string) string {
		return filepath.ToSlash(filepath.Join(append([]string{home}, parts...)...))
	}
	checks := map[string][]string{
		"agents/reviewer.md":   {"[claude] " + p(".claude", "agents", "reviewer.md") + " (full file)"},
		"skills/tidy/SKILL.md": {"[claude] " + p(".claude", "skills", "tidy", "SKILL.md"), "[codex] " + p(".agents", "skills", "tidy", "SKILL.md")},
		"rules/safe.md":        {"[claude] " + p(".claude", "CLAUDE.md") + ` (section "safe")`, "[augment] " + p(".augment", "rules", "safe.md") + " (full file)"},
		"hooks/fmt.yaml":       {"[claude] " + p(".claude", "settings.json") + ` (section "PostToolUse")`, "[codex] " + p(".codex", "hooks.json")},
	}
	for arg, wants := range checks {
		got := explain(arg)
		for _, want := range wants {
			if !strings.Contains(got, want) {
				t.Errorf("explain %s lacks %q:\n%s", arg, want, got)
			}
		}
	}
	if got := explain("agents/reviewer.md"); strings.Contains(got, "[codex]") {
		t.Errorf("a claude-only agent must not list codex:\n%s", got)
	}
}

func TestEditJSONRoot_NestedPathRoundTrips(t *testing.T) {
	for _, in := range []string{
		"{\n  \"theme\": \"dark\"\n}\n",
		"{\n  \"model\": {\n    \"maxSessionTurns\": 5\n  }\n}\n",
		"{}\n",
	} {
		set, err := editJSONRoot("s.json", []byte(in), []string{"model.name"}, map[string]any{"model.name": "gemini-3-pro"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		values, err := settingsValues("s.json", "json", set)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := settingsLookup(values, "model.name", "json"); got != "gemini-3-pro" {
			t.Fatalf("set %q -> %q", in, set)
		}
		back, err := editJSONRoot("s.json", set, nil, nil, []string{"model.name"})
		if err != nil {
			t.Fatal(err)
		}
		if string(back) != in {
			t.Errorf("round trip of %q gave %q via %q", in, back, set)
		}
	}
}

func TestEditTOMLRoot_DottedRootKeyIsNotAPlainKey(t *testing.T) {
	in := "sandbox_workspace_write.network_access = false\n"
	got, err := editTOMLRoot("c.toml", []byte(in), []string{"sandbox_workspace_write"}, map[string]any{"sandbox_workspace_write": "x"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), in) {
		t.Errorf("the dotted line must stay: %q", got)
	}
}

func TestCustomGlobalSettings_LaterSpecWinsWhole(t *testing.T) {
	f := globalTargets["claude"].settings
	entries := []spec.Entry{
		{Path: "a.yaml", Meta: map[string]any{"x-claude": map[string]any{"env": map[string]any{"FOO": "1"}, "sandbox": false}}},
		{Path: "b.yaml", Meta: map[string]any{"x-claude": map[string]any{"env": nil, "sandbox": map[string]any{"enabled": true}}}},
	}
	got := customGlobalSettings("claude", f, entries)
	if len(got) != 1 || got[0].key != "sandbox.enabled" {
		t.Errorf("got %+v, want only sandbox.enabled", got)
	}
}

func TestCustomGlobalSettings_SkipsKeysWithDots(t *testing.T) {
	f := globalTargets["gemini"].settings
	entries := []spec.Entry{{Path: "a.yaml", Meta: map[string]any{"x-gemini": map[string]any{
		"modelConfigs": map[string]any{"customAliases": map[string]any{"gemini-2.5-pro": map[string]any{"x": 1}}},
		"ui":           map[string]any{"theme": "dark"},
	}}}}
	got := customGlobalSettings("gemini", f, entries)
	if len(got) != 1 || got[0].key != "ui.theme" {
		t.Errorf("got %+v, want only ui.theme", got)
	}
}

func TestMergeGlobalSettings_RefusesInvalidTOML(t *testing.T) {
	_, err := mergeGlobalSettings("c.toml", "toml", []byte("[features]\na = true\n"),
		[]globalSetting{{target: "codex", key: "features", value: true, field: "x-codex.features", source: "s.yaml"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid TOML") {
		t.Errorf("err = %v", err)
	}
}

func TestEditTOMLTables_KeepsCommentsAndNewlineAroundOthers(t *testing.T) {
	in := "[mcp_servers.ours]\ncommand = \"x\"\n\n# notes on theirs\n[mcp_servers.theirs]\ncommand = \"y\""
	got, err := editTOMLTables("c.toml", []byte(in), "mcp_servers", nil, nil, []string{"ours"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "# notes on theirs\n[mcp_servers.theirs]\ncommand = \"y\""; string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEditJSONRoot_RemovingFirstMemberKeepsCommentAboveNext(t *testing.T) {
	in := "{\n  \"ours\": 1,\n  // my notes on mine\n  \"mine\": 2\n}\n"
	got, err := editJSONRoot("s.json", []byte(in), nil, nil, []string{"ours"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\n  // my notes on mine\n  \"mine\": 2\n}\n"; string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGlobalSettings_PermissionModeValidation(t *testing.T) {
	for _, value := range []any{"default", "manual", "acceptEdits", "plan", "auto", "dontAsk", "bypassPermissions", "invalid", "", 42, map[string]any{"claude": "plan"}} {
		entry := spec.Entry{Path: "settings/mode.yaml", Meta: map[string]any{"permissions": map[string]any{"default-mode": value}}}
		entries := []spec.Entry{entry}
		got := globalSettingsFor("claude", globalTargets["claude"], entries)
		issues := lintGlobalSettings(entries, []string{"claude", "codex"})
		valid := false
		if mode, ok := value.(string); ok {
			valid = mode != "invalid" && mode != ""
		}
		if valid {
			if len(got) != 1 || got[0].key != "permissions.defaultMode" || got[0].value != value || got[0].source != entry.Path {
				t.Errorf("%v mapping: %v", value, got)
			}
			if len(issues) != 0 {
				t.Errorf("%v lint: %v", value, issues)
			}
		} else {
			if len(got) != 0 {
				t.Errorf("invalid %v mapped: %v", value, got)
			}
			if len(issues) != 1 || issues[0].Field != "permissions.default-mode" {
				t.Errorf("%v lint: %v", value, issues)
			}
		}
		if got := globalSettingsFor("codex", globalTargets["codex"], entries); len(got) != 0 {
			t.Errorf("codex mapped %v: %v", value, got)
		}
	}
}

func TestExplainGlobal_PermissionModeNamesWinningSource(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "defaults.yaml"), "permissions:\n  default-mode: default\n")
	mustWriteGlobalTest(t, filepath.Join(source, "local", "settings", "machine.yaml"), "permissions:\n  default-mode: plan\n")
	for _, file := range []string{"settings/defaults.yaml", "local/settings/machine.yaml"} {
		var out bytes.Buffer
		cmd := NewRootCmd("test")
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"explain", "--global", file})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("explain: %v\n%s", err, out.String())
		}
		want := filepath.ToSlash(filepath.Join(home, ".claude", "settings.json")) + ` (key "permissions.defaultMode")`
		if strings.Contains(out.String(), want) != strings.HasPrefix(file, "local/") {
			t.Errorf("%s provenance:\n%s", file, out.String())
		}
	}
}

func TestGlobalSettings_PermissionModeRespectsTargetRouting(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		targets     []any
		want        bool
	}{
		{"included", "targets", []any{"claude"}, true},
		{"other target", "targets", []any{"codex"}, false},
		{"excluded", "targets-exclude", []any{"claude"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			permissions := map[string]any{"default-mode": "bypassPermissions"}
			entries := []spec.Entry{{Path: "settings/mode.yaml", Meta: map[string]any{tc.field: tc.targets, "permissions": permissions}}}
			got := globalSettingsFor("claude", globalTargets["claude"], entries)
			if (len(got) == 1) != tc.want {
				t.Errorf("routed settings = %v, want mapped %v", got, tc.want)
			}
			permissions["default-mode"] = "invalid"
			issues := lintGlobalSettings(entries, []string{"claude"})
			if (len(issues) == 1) != tc.want {
				t.Errorf("routed lint = %v, want issue %v", issues, tc.want)
			}
		})
	}
}
