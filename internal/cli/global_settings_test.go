package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditTOMLRoot_SkipsBracketsInsideValues(t *testing.T) {
	in := "notify = [\n  \"say\",\n  \"[done]\",\n]\nbanner = '''\n[not a table]\n'''\n\n[tui]\nmodel = \"keep\"\n"
	got, err := editTOMLRoot("config.toml", []byte(in), []string{"model"}, map[string]string{"model": "gpt-6-luna"}, nil)
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
	got, err := editTOMLRoot("config.toml", []byte(in), []string{"model"}, map[string]string{"model": "m"}, nil)
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
		set, err := editJSONRoot("s.json", []byte(in), []string{"model"}, map[string]string{"model": "opus"}, nil)
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
	got, err := editJSONRoot("s.json", []byte(in), []string{"model"}, map[string]string{"model": "opus"}, nil)
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
		set            map[string]string
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
			set:  map[string]string{"model": "c"},
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
	got, err := editTOMLRoot("config.toml", []byte(in), []string{"model"}, map[string]string{"model": "b"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "\xef\xbb\xbfmodel = \"b\"\r\nb = 2\n[t]\r\nx = 1\r\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
	got, err = editTOMLRoot("config.toml", []byte("a = 1\r\n"), []string{"model"}, map[string]string{"model": "m"}, []string{"missing"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "a = 1\r\nmodel = \"m\"\r\n"; string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
