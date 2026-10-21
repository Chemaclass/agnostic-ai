package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func writeSkillOverrideProject(t *testing.T, config string, skills ...string) {
	t.Helper()
	mustWrite(t, "agnostic-ai.yaml", config)
	for _, name := range skills {
		mustWrite(t, filepath.Join(".agnostic-ai", "skills", name, "SKILL.md"), "---\nname: "+name+"\ndescription: Does "+name+".\n---\n\nBody.\n")
	}
}

func TestDoctor_ReportsClaudeSkillOverrides(t *testing.T) {
	const claudeOnly = "version: 1\ntargets: [claude]\n"
	cases := []struct {
		name     string
		config   string
		path     string
		settings string
		args     []string
		want     []string
		absent   bool
	}{
		{name: "no file", config: claudeOnly, absent: true},
		{name: "all on", config: claudeOnly, settings: `{"skillOverrides":{"a":"on"}}`, absent: true},
		{name: "off and hidden", config: claudeOnly, settings: `{"skillOverrides":{"a":"off","b":"user-invocable-only","c":"on"}}`, want: []string{
			"! .claude/settings.local.json turns off 1 of 3 synced skills (off), so Claude Code refuses to run them: a",
			"! .claude/settings.local.json hides 1 of 3 synced skills from Claude (user-invocable-only): b",
			"→ review them in Claude Code with /skills, run `agnostic-ai doctor --fix` in a terminal, or delete the entries from .claude/settings.local.json",
		}},
		{name: "name only", config: claudeOnly, settings: `{"skillOverrides":{"a":"name-only"}}`, want: []string{
			"! .claude/settings.local.json lists 1 of 3 synced skills without their description (name-only): a",
		}},
		{name: "skill sync does not write", config: claudeOnly, settings: `{"skillOverrides":{"zzz":"off"}}`, absent: true},
		{name: "case and width folding", config: claudeOnly, settings: `{"skillOverrides":{"A":"off","ｂ":"off"}}`, want: []string{
			"turns off 2 of 3 synced skills (off), so Claude Code refuses to run them: a, b",
		}},
		{name: "invalid JSON", config: claudeOnly, settings: `{"skillOverrides":`, want: []string{
			"! cannot read .claude/settings.local.json: parse .claude/settings.local.json:",
		}},
		{name: "JSONC comment", config: claudeOnly, settings: "// mine\n{\"skillOverrides\":{\"a\":\"off\"}}", want: []string{
			"turns off 1 of 3 synced skills (off)",
		}},
		{name: "not an object", config: claudeOnly, settings: `{"skillOverrides":"off"}`, want: []string{
			`! .claude/settings.local.json: skillOverrides is not an object, so it was not checked; Claude Code expects {"<skill>": "off"}`,
		}},
		{name: "unknown value", config: claudeOnly, settings: `{"skillOverrides":{"a":"disabled","b":false}}`, want: []string{
			`! .claude/settings.local.json sets a value Claude Code does not document ("disabled") for: a`,
			`! .claude/settings.local.json sets a value Claude Code does not document (false) for: b`,
		}},
		{name: "custom dir", config: "version: 1\ntargets: [claude]\noutputs:\n  claude:\n    dir: tools/claude\n", path: "tools/claude/settings.local.json", settings: `{"skillOverrides":{"a":"off"}}`, want: []string{
			"! tools/claude/settings.local.json turns off 1 of 3 synced skills (off)",
		}},
		{name: "claude not a target", config: "version: 1\ntargets: [codex]\n", settings: `{"skillOverrides":{"a":"off"}}`, absent: true},
		{name: "claude not selected", config: "version: 1\ntargets: [claude, codex]\n", settings: `{"skillOverrides":{"a":"off"}}`, args: []string{"--target", "codex"}, absent: true},
		{name: "project scope", config: claudeOnly, settings: `{"skillOverrides":{"a":"off"}}`, args: []string{"--scope", "project"}, absent: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			testutil.TempCwd(t)
			silence(t)
			writeSkillOverrideProject(t, c.config, "a", "b", "c")
			if c.settings != "" {
				path := c.path
				if path == "" {
					path = ".claude/settings.local.json"
				}
				mustWrite(t, filepath.FromSlash(path), c.settings)
			}
			out, _ := runDoctor(t, c.args...)
			if c.absent {
				if strings.Contains(out, "Claude skill overrides:") {
					t.Errorf("unexpected section:\n%s", out)
				}
				return
			}
			if !strings.Contains(out, "Claude skill overrides:") {
				t.Errorf("missing section:\n%s", out)
			}
			for _, want := range c.want {
				if !strings.Contains(out, want) {
					t.Errorf("missing %q:\n%s", want, out)
				}
			}
		})
	}
}

func TestDoctor_SkillOverridesListAtMostTenNames(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	names := []string{"s01", "s02", "s03", "s04", "s05", "s06", "s07", "s08", "s09", "s10", "s11", "s12"}
	writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", names...)
	entries := make([]string, len(names))
	for i, name := range names {
		entries[i] = `"` + name + `":"off"`
	}
	mustWrite(t, filepath.Join(".claude", "settings.local.json"), "{\"skillOverrides\":{"+strings.Join(entries, ",")+"}}")
	out, _ := runDoctor(t)
	if want := "turns off 12 of 12 synced skills (off), so Claude Code refuses to run them: s01, s02, s03, s04, s05, s06, s07, s08, s09, s10, and 2 more"; !strings.Contains(out, want) {
		t.Errorf("missing %q:\n%s", want, out)
	}
}

func TestDoctor_SkillOverridesLeavesSettingsUnchanged(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", "a")
	path := filepath.Join(".claude", "settings.local.json")
	const body = "{\n  // keep\n  \"skillOverrides\": {\"a\": \"off\"}\n}\n"
	mustWrite(t, path, body)
	if _, err := runDoctor(t); err != nil && !strings.Contains(err.Error(), "drift") {
		t.Fatalf("doctor: %v", err)
	}
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader("y\n"))
	root.SetArgs([]string{"doctor", "--fix"})
	_ = root.Execute()
	if !strings.Contains(out.String(), "~ kept skillOverrides in .claude/settings.local.json") {
		t.Errorf("missing kept note:\n%s", out.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Errorf("settings changed without a confirmed prompt:\n%s", got)
	}
}

func collectTestSkillOverrides(t *testing.T, settings string, skills ...string) *skillOverrideReport {
	t.Helper()
	testutil.TempCwd(t)
	silence(t)
	writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", skills...)
	mustWrite(t, filepath.Join(".claude", "settings.local.json"), settings)
	cfg, bundle, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	report := collectClaudeSkillOverrides(cfg, bundle)
	if report == nil {
		t.Fatal("no report")
	}
	return report
}

func TestResetClaudeSkillOverrides_RemovesOnlyConfirmedKeys(t *testing.T) {
	const settings = "{\n  // mine\n  \"model\": \"opus\",\n  \"skillOverrides\": {\n    \"a\": \"off\",\n    \"b\": \"user-invocable-only\",\n    \"personal\": \"off\"\n  }\n}\n"
	report := collectTestSkillOverrides(t, settings, "a", "b")
	var asked []string
	removed, err := resetClaudeSkillOverrides(report, false, func(question string) (bool, error) {
		asked = append(asked, question)
		return strings.Contains(question, "turned off"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Errorf("removed %d, want 1", removed)
	}
	wantAsked := []string{
		"Re-enable 1 synced skill turned off in .claude/settings.local.json (a)?",
		"Re-enable 1 synced skill hidden from Claude in .claude/settings.local.json (b)?",
	}
	if strings.Join(asked, "\n") != strings.Join(wantAsked, "\n") {
		t.Errorf("asked:\n%s\nwant:\n%s", strings.Join(asked, "\n"), strings.Join(wantAsked, "\n"))
	}
	got, err := os.ReadFile(filepath.Join(".claude", "settings.local.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  // mine\n  \"model\": \"opus\",\n  \"skillOverrides\": {\n    \"b\": \"user-invocable-only\",\n    \"personal\": \"off\"\n  }\n}\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if _, err := os.Stat(filepath.Join(".claude", "settings.local.json.bak")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("backup written without --backup: %v", err)
	}
}

func TestResetClaudeSkillOverrides_DropsEmptyObjectAndBacksUp(t *testing.T) {
	const settings = "{\n  \"model\": \"opus\",\n  \"skillOverrides\": {\n    \"A\": \"off\"\n  }\n}\n"
	report := collectTestSkillOverrides(t, settings, "a")
	removed, err := resetClaudeSkillOverrides(report, true, func(string) (bool, error) { return true, nil })
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Errorf("removed %d, want 1", removed)
	}
	got, err := os.ReadFile(filepath.Join(".claude", "settings.local.json"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\n  \"model\": \"opus\"\n}\n"; string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	bak, err := os.ReadFile(filepath.Join(".claude", "settings.local.json.bak"))
	if err != nil {
		t.Fatal(err)
	}
	if string(bak) != settings {
		t.Errorf("backup:\n%s", bak)
	}
	out, _ := runDoctor(t)
	if strings.Contains(out, "Claude skill overrides:") {
		t.Errorf("section remains after reset:\n%s", out)
	}
}

func TestResetClaudeSkillOverrides_DeclinedLeavesFile(t *testing.T) {
	const settings = `{"skillOverrides":{"a":"off"}}`
	report := collectTestSkillOverrides(t, settings, "a")
	removed, err := resetClaudeSkillOverrides(report, true, func(string) (bool, error) { return false, nil })
	if err != nil || removed != 0 {
		t.Fatalf("removed %d, err %v", removed, err)
	}
	got, _ := os.ReadFile(filepath.Join(".claude", "settings.local.json"))
	if string(got) != settings {
		t.Errorf("file changed: %s", got)
	}
	if _, err := os.Stat(filepath.Join(".claude", "settings.local.json.bak")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("backup written for a declined reset: %v", err)
	}
}

func TestResetClaudeSkillOverrides_RefusesFileChangedAfterRead(t *testing.T) {
	report := collectTestSkillOverrides(t, `{"skillOverrides":{"a":"off"}}`, "a")
	const edited = `{"skillOverrides":{"a":"off","b":"off"}}`
	removed, err := resetClaudeSkillOverrides(report, false, func(string) (bool, error) {
		mustWrite(t, filepath.Join(".claude", "settings.local.json"), edited)
		return true, nil
	})
	if !errors.Is(err, errSkillOverridesChanged) || removed != 0 {
		t.Fatalf("removed %d, err %v", removed, err)
	}
	got, _ := os.ReadFile(filepath.Join(".claude", "settings.local.json"))
	if string(got) != edited {
		t.Errorf("file overwritten: %s", got)
	}
}

func TestResetClaudeSkillOverrides_RemovesDuplicateKeys(t *testing.T) {
	report := collectTestSkillOverrides(t, `{"skillOverrides":{"a":"off","a":"off","b":"on"}}`, "a", "b")
	if _, err := resetClaudeSkillOverrides(report, false, func(string) (bool, error) { return true, nil }); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(".claude", "settings.local.json"))
	if want := `{"skillOverrides":{"b":"on"}}`; string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
