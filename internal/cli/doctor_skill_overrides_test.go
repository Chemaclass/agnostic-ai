package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"

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
			"! cannot parse .claude/settings.local.json:",
		}},
		{name: "empty file", config: claudeOnly, settings: " \n", absent: true},
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
	report := localSkillOverrides(collectClaudeSkillOverrides(cfg, bundle, false))
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

func TestCollectClaudeSkillOverrides_MatchesTheNameClaudeSees(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWrite(t, filepath.Join(".agnostic-ai", "skills", "review", "SKILL.md"), "---\nname: review\ndescription: Reviews.\nx-claude:\n  name: claude-review\n---\n\nBody.\n")
	mustWrite(t, filepath.Join(".claude", "settings.local.json"), `{"skillOverrides":{"claude-review":"off","review":"off"}}`)
	cfg, bundle, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	report := localSkillOverrides(collectClaudeSkillOverrides(cfg, bundle, false))
	if report == nil || len(report.states) != 1 {
		t.Fatalf("report: %+v", report)
	}
	if got := report.states[0]; strings.Join(got.keys, ",") != "claude-review" || strings.Join(got.skills, ",") != "claude-review" {
		t.Errorf("finding: %+v", got)
	}
}

func TestResetClaudeSkillOverrides_RefusesRepeatedSkillOverridesObject(t *testing.T) {
	const settings = `{"skillOverrides":{"a":"off"},"skillOverrides":{"b":"off"}}`
	report := collectTestSkillOverrides(t, settings, "a", "b")
	removed, err := resetClaudeSkillOverrides(report, false, func(string) (bool, error) { return true, nil })
	if !errors.Is(err, errSkillOverridesRepeated) || removed != 0 {
		t.Fatalf("removed %d, err %v", removed, err)
	}
	got, _ := os.ReadFile(filepath.Join(".claude", "settings.local.json"))
	if string(got) != settings {
		t.Errorf("file changed: %s", got)
	}
}

func TestResetClaudeSkillOverrides_RefusesSymlinkedSettings(t *testing.T) {
	report := collectTestSkillOverrides(t, `{"skillOverrides":{"a":"off"}}`, "a")
	target, err := filepath.Abs("elsewhere.json")
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, target, `{"skillOverrides":{"a":"off"}}`)
	path := filepath.Join(".claude", "settings.local.json")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlink: %v", err)
	}
	removed, err := resetClaudeSkillOverrides(report, false, func(string) (bool, error) { return true, nil })
	if err == nil || removed != 0 {
		t.Fatalf("removed %d, err %v", removed, err)
	}
	if got, _ := os.ReadFile(target); string(got) != `{"skillOverrides":{"a":"off"}}` {
		t.Errorf("link target changed: %s", got)
	}
}

func TestResetClaudeSkillOverrides_BackupKeepsPrivateMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes")
	}
	report := collectTestSkillOverrides(t, `{"skillOverrides":{"a":"off"}}`, "a")
	path := filepath.Join(".claude", "settings.local.json")
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := resetClaudeSkillOverrides(report, true, func(string) (bool, error) { return true, nil }); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("backup mode %v, want 0600", info.Mode().Perm())
	}
}

func TestApplySkillOverrideReset_ReportsWriteFailureAndKeepsFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a directory the user cannot write")
	}
	const settings = `{"model":"opus","skillOverrides":{"a":"off"}}`
	report := collectTestSkillOverrides(t, settings, "a")
	if err := os.Chmod(".claude", 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(".claude", 0o755) })
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := applySkillOverrideReset(cmd, report, false, func(string) (bool, error) { return true, nil }); err != nil {
		t.Fatalf("doctor stopped: %v", err)
	}
	if !strings.Contains(out.String(), "! .claude") {
		t.Errorf("missing warning:\n%s", out.String())
	}
	if got, _ := os.ReadFile(filepath.Join(".claude", "settings.local.json")); string(got) != settings {
		t.Errorf("file damaged: %q", got)
	}
}

func TestResetClaudeSkillOverrides_RefusesReadOnlyFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("POSIX file modes")
	}
	const settings = `{"skillOverrides":{"a":"off"}}`
	report := collectTestSkillOverrides(t, settings, "a")
	path := filepath.Join(".claude", "settings.local.json")
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	if removed, err := resetClaudeSkillOverrides(report, false, func(string) (bool, error) { return true, nil }); err == nil || removed != 0 {
		t.Fatalf("removed %d, err %v", removed, err)
	}
	if got, _ := os.ReadFile(path); string(got) != settings {
		t.Errorf("read-only file changed: %s", got)
	}
}

func TestCollectClaudeSkillOverrides_SkipsUnmanagedSkills(t *testing.T) {
	cases := []struct{ name, config string }{
		{"directory", "sync:\n  unmanaged: [.claude/skills/a/]\n"},
		{"exact path", "sync:\n  unmanaged: [.claude/skills/a/SKILL.md]\n"},
		{"custom dir", "outputs:\n  claude:\n    dir: tools/claude\nsync:\n  unmanaged: [tools/claude/skills/a/]\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			testutil.TempCwd(t)
			silence(t)
			writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n"+c.config, "a", "b")
			dir := ".claude"
			if strings.Contains(c.config, "tools/claude") {
				dir = filepath.Join("tools", "claude")
			}
			mustWrite(t, filepath.Join(dir, "settings.local.json"), `{"skillOverrides":{"a":"off","b":"off"}}`)
			cfg, bundle, err := loadProject(".")
			if err != nil {
				t.Fatal(err)
			}
			report := localSkillOverrides(collectClaudeSkillOverrides(cfg, bundle, false))
			if report == nil || len(report.states) != 1 || strings.Join(report.states[0].skills, ",") != "b" || report.synced != 1 {
				t.Fatalf("report: %+v", report)
			}
		})
	}
}

func TestApplySkillOverrideReset_LeavesUnmanagedSettingsFile(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\nsync:\n  unmanaged: [.claude/settings.local.json]\n", "a")
	const settings = `{"skillOverrides":{"a":"off"}}`
	mustWrite(t, filepath.Join(".claude", "settings.local.json"), settings)
	cfg, bundle, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	report := localSkillOverrides(collectClaudeSkillOverrides(cfg, bundle, false))
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	reportClaudeSkillOverrides(cmd, []*skillOverrideReport{report})
	if err := applySkillOverrideReset(cmd, report, false, func(question string) (bool, error) {
		t.Errorf("asked %q for an unmanaged file", question)
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "sync.unmanaged") {
		t.Errorf("output:\n%s", out.String())
	}
}

func TestDoctor_SkillOverridesNamesFileFixCannotEdit(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", "a", "b")
	mustWrite(t, filepath.Join(".claude", "settings.local.json"), `{"skillOverrides":{"a":"off"},"skillOverrides":{"b":"off"}}`)
	cfg, bundle, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	report := localSkillOverrides(collectClaudeSkillOverrides(cfg, bundle, false))
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	reportClaudeSkillOverrides(cmd, []*skillOverrideReport{report})
	if err := applySkillOverrideReset(cmd, report, false, func(question string) (bool, error) {
		t.Errorf("asked %q for a file doctor cannot edit", question)
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "could change which overrides apply") || strings.Contains(text, "doctor --fix") {
		t.Errorf("output:\n%s", text)
	}
}

func TestCollectClaudeSkillOverrides_LeavesHardLinkedSettings(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("link counts come from POSIX stat")
	}
	collectTestSkillOverrides(t, `{"skillOverrides":{"a":"off"}}`, "a")
	if err := os.Link(filepath.Join(".claude", "settings.local.json"), "shared.json"); err != nil {
		t.Skipf("hard link: %v", err)
	}
	cfg, bundle, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	report := localSkillOverrides(collectClaudeSkillOverrides(cfg, bundle, false))
	if report == nil || report.uneditable == nil || !strings.Contains(report.uneditable.Error(), "hard link") {
		t.Fatalf("report: %+v", report)
	}
}

func TestDoctor_ReportsSkillOverridesFromEveryClaudeSettingsFile(t *testing.T) {
	const (
		local   = ".claude/settings.local.json"
		shared  = ".claude/settings.json"
		overlay = ".agnostic-ai/overlays/claude.settings.json"
		user    = "user"
	)
	cases := []struct {
		name   string
		files  map[string]string
		args   []string
		userAt string
		want   []string
		absent []string
	}{
		{
			name:  "user only",
			files: map[string]string{user: `{"skillOverrides":{"a":"off"}}`},
			want: []string{
				"! ~/.claude/settings.json turns off 1 of 3 synced skills (off), so Claude Code refuses to run them: a",
				"→ applies to every project on this machine; delete the entries from ~/.claude/settings.json",
			},
			absent: []string{"doctor --fix"},
		},
		{
			name:  "shared only",
			files: map[string]string{shared: `{"skillOverrides":{"b":"user-invocable-only"}}`},
			want: []string{
				"! .claude/settings.json hides 1 of 3 synced skills from Claude (user-invocable-only): b",
				"→ team policy: set skillOverrides in .agnostic-ai/overlays/claude.settings.json and run `agnostic-ai sync`",
			},
			absent: []string{"doctor --fix", "delete the entries"},
		},
		{
			name: "every file in Claude Code's order",
			files: map[string]string{
				user:   `{"skillOverrides":{"a":"off"}}`,
				shared: `{"skillOverrides":{"b":"user-invocable-only"}}`,
				local:  `{"skillOverrides":{"c":"off"}}`,
			},
			want: []string{
				"! .claude/settings.local.json turns off 1 of 3 synced skills (off), so Claude Code refuses to run them: c",
				"! .claude/settings.json hides 1 of 3 synced skills from Claude (user-invocable-only): b",
				"! ~/.claude/settings.json turns off 1 of 3 synced skills (off), so Claude Code refuses to run them: a",
			},
		},
		{
			name: "broken user file does not hide the local one",
			files: map[string]string{
				user:  `{"skillOverrides":`,
				local: `{"skillOverrides":{"a":"off"}}`,
			},
			want: []string{
				"! .claude/settings.local.json turns off 1 of 3 synced skills (off)",
				"! cannot parse ~/.claude/settings.json:",
			},
			absent: []string{"parse ~/.claude/settings.json: parse", "cannot read"},
		},
		{
			name:   "a higher file turning a skill back on hides the lower entry",
			files:  map[string]string{user: `{"skillOverrides":{"a":"off","b":"off"}}`, shared: `{"skillOverrides":{"A":"on"}}`, local: `{"skillOverrides":{"b":"on"}}`},
			absent: []string{"Claude skill overrides:"},
		},
		{
			name: "shared entry the overlay sets is a note",
			files: map[string]string{
				shared:  `{"skillOverrides":{"b":"off","c":"name-only"}}`,
				overlay: `{"skillOverrides":{"b":"off","c":"off"}}`,
			},
			want: []string{
				"! .claude/settings.json lists 1 of 3 synced skills without their description (name-only): c",
				"→ team policy: set skillOverrides in .agnostic-ai/overlays/claude.settings.json and run `agnostic-ai sync`",
				"~ .claude/settings.json turns off 1 of 3 synced skills (off) as .agnostic-ai/overlays/claude.settings.json sets: b",
			},
			absent: []string{"! .claude/settings.json turns off"},
		},
		{
			name:   "overlay note under project scope",
			files:  map[string]string{shared: `{"skillOverrides":{"b":"off"}}`, overlay: `{"skillOverrides":{"b":"off"}}`},
			args:   []string{"--scope", "project"},
			want:   []string{"~ .claude/settings.json turns off 1 of 3 synced skills (off) as .agnostic-ai/overlays/claude.settings.json sets: b"},
			absent: []string{"! ", "→"},
		},
		{
			name:  "entries for skills sync does not write are counted",
			files: map[string]string{user: `{"skillOverrides":{"a":"off","mine":"off","other":"on"}}`, shared: `{"skillOverrides":{"b":"off","mine":"off"}}`},
			want: []string{
				"! .claude/settings.json turns off 1 of 3 synced skills (off)",
				"~ .claude/settings.json also sets 1 entry for skills sync does not write",
				"! ~/.claude/settings.json turns off 1 of 3 synced skills (off)",
				"~ ~/.claude/settings.json also sets 2 entries for skills sync does not write",
			},
			absent: []string{"mine", "other"},
		},
		{
			name: "project scope reads only the shared file",
			files: map[string]string{
				user:   `{"skillOverrides":{"a":"off"}}`,
				shared: `{"skillOverrides":{"b":"off"}}`,
				local:  `{"skillOverrides":{"c":"off"}}`,
			},
			args:   []string{"--scope", "project"},
			want:   []string{"! .claude/settings.json turns off 1 of 3 synced skills (off)"},
			absent: []string{"settings.local.json turns off", "~/.claude/settings.json"},
		},
		{
			name:   "claude not selected",
			files:  map[string]string{user: `{"skillOverrides":{"a":"off"}}`, shared: `{"skillOverrides":{"b":"off"}}`},
			args:   []string{"--target", "codex"},
			absent: []string{"Claude skill overrides:"},
		},
		{
			name:   "config dir is the project's claude dir",
			files:  map[string]string{shared: `{"skillOverrides":{"b":"off"}}`},
			userAt: ".claude",
			want:   []string{"! .claude/settings.json turns off 1 of 3 synced skills (off)"},
			absent: []string{"every project on this machine"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			writeSkillOverrideProject(t, "version: 1\ntargets: [claude, codex]\n", "a", "b", "c")
			userDir := filepath.Join(skillOverrideHome(t), ".claude")
			if c.userAt != "" {
				userDir = filepath.Join(dir, c.userAt)
				t.Setenv("CLAUDE_CONFIG_DIR", userDir)
			}
			for path, body := range c.files {
				if path == user {
					path = filepath.Join(userDir, "settings.json")
				}
				mustWrite(t, filepath.FromSlash(path), body)
			}
			out, _ := runDoctor(t, c.args...)
			out = skillOverridesSection(out)
			last := -1
			for _, want := range c.want {
				i := strings.Index(out, want)
				if i < 0 {
					t.Errorf("missing %q:\n%s", want, out)
					continue
				}
				if i < last {
					t.Errorf("%q out of order:\n%s", want, out)
				}
				last = i
			}
			for _, absent := range c.absent {
				if strings.Contains(out, absent) {
					t.Errorf("unexpected %q:\n%s", absent, out)
				}
			}
		})
	}
}

func TestDoctor_SkillOverridesReadsClaudeConfigDir(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", "a")
	configDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	mustWrite(t, filepath.Join(configDir, "settings.json"), `{"skillOverrides":{"a":"off"}}`)
	mustWrite(t, filepath.Join(skillOverrideHome(t), ".claude", "settings.json"), `{"skillOverrides":{"a":"name-only"}}`)
	out, _ := runDoctor(t)
	if want := filepath.ToSlash(filepath.Join(configDir, "settings.json")) + " turns off 1 of 1 synced skills (off)"; !strings.Contains(out, want) {
		t.Errorf("missing %q:\n%s", want, out)
	}
	if strings.Contains(out, "name-only") {
		t.Errorf("read ~/.claude with CLAUDE_CONFIG_DIR set:\n%s", out)
	}
}

func skillOverrideGitRepo(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Claude Code keeps the local file beside the project on Windows")
	}
	isolateGit(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "-q")
	git(t, root, "commit", "-q", "--allow-empty", "-m", "init")
	return root
}

func TestDoctor_SkillOverridesReadsGitRootLocalFileForNestedProject(t *testing.T) {
	t.Run("git root", func(t *testing.T) {
		root := skillOverrideGitRepo(t)
		mustWrite(t, filepath.Join(root, ".claude", "settings.local.json"), `{"skillOverrides":{"a":"off","c":"on"}}`)
		testutil.Chdir(t, mustMkdir(t, filepath.Join(root, "sub")))
		silence(t)
		writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", "a", "b", "c")
		mustWrite(t, filepath.Join(".claude", "settings.local.json"), `{"skillOverrides":{"b":"off","c":"off"}}`)
		out, _ := runDoctor(t)
		out = skillOverridesSection(out)
		last := -1
		for _, want := range []string{
			"! ../.claude/settings.local.json (git root) turns off 1 of 3 synced skills (off), so Claude Code refuses to run them: a",
			"→ Claude Code reads it for the whole repository; review them in Claude Code with /skills, or delete the entries from ../.claude/settings.local.json",
			"! .claude/settings.local.json turns off 1 of 3 synced skills (off), so Claude Code refuses to run them: b\n",
		} {
			i := strings.Index(out, want)
			if i < 0 {
				t.Errorf("missing %q:\n%s", want, out)
				continue
			}
			if i < last {
				t.Errorf("%q out of order:\n%s", want, out)
			}
			last = i
		}
	})
	t.Run("main checkout", func(t *testing.T) {
		root := skillOverrideGitRepo(t)
		mustWrite(t, filepath.Join(root, ".claude", "settings.local.json"), `{"skillOverrides":{"a":"off"}}`)
		worktree := filepath.Join(filepath.Dir(root), filepath.Base(root)+"-wt")
		git(t, root, "worktree", "add", "-q", worktree)
		testutil.Chdir(t, worktree)
		silence(t)
		writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", "a")
		out, _ := runDoctor(t)
		if want := "../" + filepath.Base(root) + "/.claude/settings.local.json (main checkout) turns off 1 of 1 synced skills (off)"; !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	})
	t.Run("project at the root reads the file once", func(t *testing.T) {
		root := skillOverrideGitRepo(t)
		testutil.Chdir(t, root)
		silence(t)
		writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", "a")
		mustWrite(t, filepath.Join(".claude", "settings.local.json"), `{"skillOverrides":{"a":"off"}}`)
		out, _ := runDoctor(t)
		if n := strings.Count(out, "turns off 1 of 1 synced skills"); n != 1 || strings.Contains(out, "(git root)") || !strings.Contains(out, "doctor --fix` in a terminal") {
			t.Errorf("want one local line, got %d:\n%s", n, out)
		}
	})
}

func TestDoctor_SkillOverridesSkipsGitRootFileWhenRootIsHome(t *testing.T) {
	root := skillOverrideGitRepo(t)
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	mustWrite(t, filepath.Join(root, ".claude", "settings.local.json"), `{"skillOverrides":{"a":"off"}}`)
	testutil.Chdir(t, mustMkdir(t, filepath.Join(root, "sub")))
	silence(t)
	writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", "a")
	out, _ := runDoctor(t)
	if strings.Contains(out, "Claude skill overrides:") {
		t.Errorf("read the home folder's local file:\n%s", out)
	}
}

// skillOverridesSection keeps the doctor block, so hints from other
// blocks do not match.
func skillOverridesSection(out string) string {
	start := strings.Index(out, "Claude skill overrides:")
	if start < 0 {
		return ""
	}
	section := out[start:]
	if end := strings.Index(section, "\n\n"); end >= 0 {
		section = section[:end]
	}
	return section
}

// skillOverrideHome gives the test its own home folder, so a user
// settings file one test writes never reaches another.
func skillOverrideHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func mustMkdir(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func doctorSkillOverridesJSON(t *testing.T, args ...string) []map[string]any {
	t.Helper()
	out, _ := runDoctorStdout(t, append([]string{"--json"}, args...)...)
	var got struct {
		SkillOverrides []map[string]any `json:"skill_overrides"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if got.SkillOverrides == nil {
		t.Fatalf("skill_overrides missing or null:\n%s", out)
	}
	return got.SkillOverrides
}

func runDoctorStdout(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"doctor"}, args...))
	err := root.Execute()
	return out.String(), err
}

func TestDoctor_JSONListsSkillOverrides(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	home := skillOverrideHome(t)
	writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", "a", "b")
	if got := doctorSkillOverridesJSON(t); len(got) != 0 {
		t.Errorf("want [], got %+v", got)
	}
	mustWrite(t, filepath.Join(".claude", "settings.local.json"), `{"skillOverrides":{"a":"on"}}`)
	mustWrite(t, filepath.Join(".claude", "settings.json"), `{"skillOverrides":{"a":"off","b":"","mine":"off"}}`)
	userFile := filepath.Join(home, ".claude", "settings.json")
	mustWrite(t, userFile, `{"skillOverrides":`)
	entry := func(path, scope, skill, value string, synced, effective bool) map[string]any {
		return map[string]any{"path": path, "scope": scope, "skill": skill, "value": value, "synced": synced, "effective": effective}
	}
	got := doctorSkillOverridesJSON(t)
	want := []map[string]any{
		entry(".claude/settings.local.json", "local", "a", "on", true, true),
		entry(".claude/settings.json", "shared", "a", "off", true, false),
		entry(".claude/settings.json", "shared", "b", "", true, true),
		entry(".claude/settings.json", "shared", "mine", "off", false, true),
	}
	if len(got) != len(want)+1 {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("record %d: got %v, want %v", i, got[i], want[i])
		}
	}
	problem := got[len(want)]
	if problem["path"] != filepath.ToSlash(userFile) || problem["scope"] != "user" || len(problem) != 3 || !strings.HasPrefix(problem["problem"].(string), "cannot parse ") {
		t.Errorf("problem record: %v", problem)
	}
	project := doctorSkillOverridesJSON(t, "--scope", "project")
	if len(project) != 3 || project[0]["scope"] != "shared" || project[0]["effective"] != true {
		t.Errorf("--scope project: %+v", project)
	}
}

func TestDoctor_FixEditsOnlyTheLocalFile(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", "a", "b")
	const others = `{"skillOverrides":{"a":"off"}}`
	userFile := filepath.Join(skillOverrideHome(t), ".claude", "settings.json")
	mustWrite(t, filepath.Join(".claude", "settings.json"), others)
	mustWrite(t, userFile, others)
	mustWrite(t, filepath.Join(".claude", "settings.local.json"), `{"skillOverrides":{"b":"off"}}`)
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader("y\n"))
	root.SetArgs([]string{"doctor", "--fix"})
	_ = root.Execute()
	if n := strings.Count(out.String(), "~ kept skillOverrides in"); n != 1 || !strings.Contains(out.String(), "~ kept skillOverrides in .claude/settings.local.json") {
		t.Errorf("want one kept note for the local file:\n%s", out.String())
	}
	cfg, bundle, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	reports := collectClaudeSkillOverrides(cfg, bundle, false)
	if len(reports) != 3 {
		t.Fatalf("reports: %d", len(reports))
	}
	if _, err := resetClaudeSkillOverrides(localSkillOverrides(reports), false, func(string) (bool, error) { return true, nil }); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(".claude", "settings.json"), userFile} {
		if got, _ := os.ReadFile(path); string(got) != others {
			t.Errorf("%s changed: %s", path, got)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(".claude", "settings.local.json")); string(got) != `{}` {
		t.Errorf("local file: %s", got)
	}
}

func TestDoctor_SkillOverridesNamesUnreadableFileOnce(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a file the user cannot read")
	}
	testutil.TempCwd(t)
	silence(t)
	home := skillOverrideHome(t)
	writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", "a")
	path := filepath.Join(home, ".claude", "settings.json")
	mustWrite(t, path, `{"skillOverrides":{"a":"off"}}`)
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	out, _ := runDoctor(t)
	out = skillOverridesSection(out)
	if want := "! cannot read ~/.claude/settings.json: permission denied"; !strings.HasSuffix(out, want) {
		t.Errorf("missing %q:\n%s", want, out)
	}
	if strings.Count(out, "settings.json") != 1 {
		t.Errorf("path named more than once:\n%s", out)
	}
}
