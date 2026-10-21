package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func runExplain(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs(append([]string{"explain"}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("explain %v: %v", args, err)
	}
	return out.String()
}

func explainOverridesBlock(out string) string {
	i := strings.Index(out, "claude overrides:")
	if i < 0 {
		return ""
	}
	return out[i:]
}

const deploySkill = ".agnostic-ai/skills/deploy/SKILL.md"

func TestExplain_SkillShowsLocalOverride(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	skillOverrideHome(t)
	writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", "deploy")
	mustWrite(t, filepath.Join(".claude", "settings.local.json"), `{"skillOverrides":{"deploy":"off"}}`)
	want := "claude overrides:\n  .claude/settings.local.json: off (Claude Code refuses to run it; change it in /skills)\n"
	if got := explainOverridesBlock(runExplain(t, deploySkill)); got != want {
		t.Errorf("block:\n%s\nwant:\n%s", got, want)
	}
}

func TestExplain_SkillWithoutOverridesPrintsNoBlock(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	skillOverrideHome(t)
	writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", "deploy", "other")
	mustWrite(t, filepath.Join(".claude", "settings.local.json"), `{"skillOverrides":{"other":"off"}}`)
	if out := runExplain(t, deploySkill); strings.Contains(out, "claude overrides:") {
		t.Errorf("unexpected block:\n%s", out)
	}
	if out := runExplain(t, deploySkill, "--json"); strings.Contains(out, `"overrides"`) {
		t.Errorf("unexpected overrides key:\n%s", out)
	}
}

func TestExplain_SkillOverridesMatchTheNameClaudeSees(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	skillOverrideHome(t)
	mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWrite(t, filepath.Join(".agnostic-ai", "skills", "deploy", "SKILL.md"), "---\nname: deploy\ndescription: Deploys.\nx-claude:\n  name: claude-deploy\n---\n\nBody.\n")
	mustWrite(t, filepath.Join(".claude", "settings.local.json"), `{"skillOverrides":{"Claude-Deploy":"off","deploy":"name-only"}}`)
	want := "claude overrides:\n  .claude/settings.local.json: off (Claude Code refuses to run it; change it in /skills)\n"
	if got := explainOverridesBlock(runExplain(t, deploySkill)); got != want {
		t.Errorf("block:\n%s\nwant:\n%s", got, want)
	}
}

func TestExplain_SkillOverridesListEveryFileInPrecedenceOrder(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	home := skillOverrideHome(t)
	writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", "deploy")
	mustWrite(t, filepath.Join(home, ".claude", "settings.json"), `{"skillOverrides":{"deploy":"name-only"}}`)
	mustWrite(t, filepath.Join(".claude", "settings.json"), `{"skillOverrides":{"deploy":"user-invocable-only"}}`)
	mustWrite(t, filepath.Join(".claude", "settings.local.json"), `{"skillOverrides":{"deploy":"off"}}`)
	want := "claude overrides:\n" +
		"  .claude/settings.local.json: off (Claude Code refuses to run it; change it in /skills)\n" +
		"  .claude/settings.json: user-invocable-only (not applied: a higher file sets it)\n" +
		"  ~/.claude/settings.json: name-only (not applied: a higher file sets it)\n"
	if got := explainOverridesBlock(runExplain(t, deploySkill)); got != want {
		t.Errorf("block:\n%s\nwant:\n%s", got, want)
	}
}

func TestExplain_SkillOverridesNameEachScope(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			name:  "shared",
			files: map[string]string{".claude/settings.json": `{"skillOverrides":{"deploy":"user-invocable-only"}}`},
			want:  "  .claude/settings.json: user-invocable-only (hidden from Claude; team policy, set it in .agnostic-ai/overlays/claude.settings.json)\n",
		},
		{
			name: "shared set by the overlay",
			files: map[string]string{
				".claude/settings.json":                      `{"skillOverrides":{"deploy":"off"}}`,
				".agnostic-ai/overlays/claude.settings.json": `{"skillOverrides":{"deploy":"off"}}`,
			},
			want: "  .claude/settings.json: off (Claude Code refuses to run it; set by .agnostic-ai/overlays/claude.settings.json)\n",
		},
		{
			name:  "user",
			files: map[string]string{"user": `{"skillOverrides":{"deploy":"name-only"}}`},
			want:  "  ~/.claude/settings.json: name-only (listed without its description; applies to every project)\n",
		},
		{
			name:  "turned back on",
			files: map[string]string{".claude/settings.local.json": `{"skillOverrides":{"deploy":"on"}}`},
			want:  "  .claude/settings.local.json: on (runs as usual; change it in /skills)\n",
		},
		{
			name:  "undocumented value",
			files: map[string]string{".claude/settings.local.json": `{"skillOverrides":{"deploy":false}}`},
			want:  "  .claude/settings.local.json: false (rejected by Claude Code, which may skip the whole file; change it in /skills)\n",
		},
		{
			name:  "broken user file",
			files: map[string]string{"user": `{"skillOverrides":`},
			want:  "  ~/.claude/settings.json: Claude Code skips this file (",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			testutil.TempCwd(t)
			silence(t)
			home := skillOverrideHome(t)
			writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", "deploy")
			for path, body := range c.files {
				if path == "user" {
					path = filepath.Join(home, ".claude", "settings.json")
				}
				mustWrite(t, filepath.FromSlash(path), body)
			}
			got := explainOverridesBlock(runExplain(t, deploySkill))
			if !strings.HasPrefix(got, "claude overrides:\n"+c.want) {
				t.Errorf("block:\n%s\nwant:\n%s", got, c.want)
			}
		})
	}
}

func TestExplain_SkillOverridesJSON(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	home := skillOverrideHome(t)
	writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", "deploy")
	mustWrite(t, filepath.Join(home, ".claude", "settings.json"), `{"skillOverrides":{"deploy":"name-only"}}`)
	mustWrite(t, filepath.Join(".claude", "settings.local.json"), `{"skillOverrides":{"deploy":"off"}}`)
	out := runExplain(t, deploySkill, "--json")
	var got explainOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode explain: %v\n%s", err, out)
	}
	userPath := filepath.ToSlash(filepath.Join(home, ".claude", "settings.json"))
	if real, err := filepath.EvalSymlinks(home); err == nil {
		if !strings.Contains(out, userPath) {
			userPath = filepath.ToSlash(filepath.Join(real, ".claude", "settings.json"))
		}
	}
	want := []explainOverride{
		{Target: "claude", Path: ".claude/settings.local.json", Scope: "local", Value: "off", Effective: true},
		{Target: "claude", Path: userPath, Scope: "user", Value: "name-only"},
	}
	if !reflect.DeepEqual(got.Overrides, want) {
		t.Errorf("overrides = %+v, want %+v", got.Overrides, want)
	}
}

func TestExplain_SkillOverridesJSONProblemHasNoValue(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	home := skillOverrideHome(t)
	writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", "deploy")
	mustWrite(t, filepath.Join(home, ".claude", "settings.json"), `{"skillOverrides":`)
	out := runExplain(t, deploySkill, "--json")
	var raw struct {
		Overrides []map[string]any `json:"overrides"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("decode explain: %v\n%s", err, out)
	}
	if len(raw.Overrides) != 1 {
		t.Fatalf("overrides = %+v, want one problem", raw.Overrides)
	}
	record := raw.Overrides[0]
	if problem, _ := record["problem"].(string); !strings.HasPrefix(problem, "~/.claude/settings.json: Claude Code skips this file (") {
		t.Errorf("problem = %q", problem)
	}
	for _, key := range []string{"value", "effective"} {
		if _, ok := record[key]; ok {
			t.Errorf("problem record has %q: %+v", key, record)
		}
	}
}

func TestExplain_SkillOverridesSkipped(t *testing.T) {
	cases := []struct {
		name, config, spec, skill string
	}{
		{"rule with the same name", "version: 1\ntargets: [claude]\n", ".agnostic-ai/rules/deploy.md", ""},
		{"claude not a target", "version: 1\ntargets: [codex]\n", deploySkill, "deploy"},
		{"skill not written for claude", "version: 1\ntargets: [claude, codex]\n", deploySkill, "---\nname: deploy\ndescription: Deploys.\ntargets: [codex]\n---\n\nBody.\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			testutil.TempCwd(t)
			silence(t)
			skillOverrideHome(t)
			writeSkillOverrideProject(t, c.config, "other")
			switch {
			case c.skill == "":
				mustWrite(t, filepath.FromSlash(c.spec), "---\nname: deploy\ndescription: Deploy rule.\n---\n\nBody.\n")
			case strings.HasPrefix(c.skill, "---"):
				mustWrite(t, filepath.FromSlash(c.spec), c.skill)
			default:
				writeSkillOverrideProject(t, c.config, c.skill, "other")
			}
			mustWrite(t, filepath.Join(".claude", "settings.local.json"), `{"skillOverrides":{"deploy":"off"}}`)
			if out := runExplain(t, c.spec); strings.Contains(out, "claude overrides:") {
				t.Errorf("unexpected block:\n%s", out)
			}
		})
	}
}

func TestExplain_SkillOverridesNameTheGitRootLocalFile(t *testing.T) {
	root := skillOverrideGitRepo(t)
	mustWrite(t, filepath.Join(root, ".claude", "settings.local.json"), `{"skillOverrides":{"deploy":"off"}}`)
	testutil.Chdir(t, mustMkdir(t, filepath.Join(root, "sub")))
	silence(t)
	skillOverrideHome(t)
	writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", "deploy")
	want := "claude overrides:\n  ../.claude/settings.local.json (git root): off (Claude Code refuses to run it; read for the whole repository; change it in /skills)\n"
	if got := explainOverridesBlock(runExplain(t, deploySkill)); got != want {
		t.Errorf("block:\n%s\nwant:\n%s", got, want)
	}
}

func TestExplain_SkillOverridesTreatSettingsAsStrictJSON(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	home := skillOverrideHome(t)
	writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", "deploy")
	mustWrite(t, filepath.Join(".claude", "settings.local.json"), "{\n  // mine\n  \"skillOverrides\": {\"deploy\": \"off\"}\n}\n")
	mustWrite(t, filepath.Join(home, ".claude", "settings.json"), `{"skillOverrides":{"deploy":"name-only"}}`)
	got := explainOverridesBlock(runExplain(t, deploySkill))
	for _, want := range []string{
		"  .claude/settings.local.json: Claude Code skips this file (invalid character '/'",
		"  ~/.claude/settings.json: name-only (listed without its description; applies to every project)\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}

func TestExplain_SkillOverridesRejectedValueOverrulesNothing(t *testing.T) {
	cases := []struct{ value, shown string }{
		{`"Off"`, `"Off"`},
		{`false`, `false`},
		{`null`, `null`},
	}
	for _, c := range cases {
		t.Run(c.value, func(t *testing.T) {
			testutil.TempCwd(t)
			silence(t)
			home := skillOverrideHome(t)
			writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", "deploy")
			mustWrite(t, filepath.Join(".claude", "settings.local.json"), `{"skillOverrides":{"deploy":`+c.value+`}}`)
			mustWrite(t, filepath.Join(home, ".claude", "settings.json"), `{"skillOverrides":{"deploy":"name-only"}}`)
			want := "claude overrides:\n" +
				"  .claude/settings.local.json: " + c.shown + " (rejected by Claude Code, which may skip the whole file; change it in /skills)\n" +
				"  ~/.claude/settings.json: name-only (listed without its description; applies to every project)\n"
			if got := explainOverridesBlock(runExplain(t, deploySkill)); got != want {
				t.Errorf("block:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestExplain_SkillOverridesNeedThisSkillWrittenForClaude(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	skillOverrideHome(t)
	mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor]\n")
	mustWrite(t, filepath.FromSlash(deploySkill), "---\nname: deploy\ndescription: Deploys.\ntargets: [cursor]\n---\n\nBody.\n")
	mustWrite(t, filepath.Join(".agnostic-ai", "skills", "other", "SKILL.md"), "---\nname: other\ndescription: Other.\nx-claude:\n  name: Deploy\n---\n\nBody.\n")
	mustWrite(t, filepath.Join(".claude", "settings.local.json"), `{"skillOverrides":{"deploy":"off"}}`)
	if out := runExplain(t, deploySkill); strings.Contains(out, "claude overrides:") {
		t.Errorf("unexpected block for a skill not written for claude:\n%s", out)
	}
	if out := runExplain(t, ".agnostic-ai/skills/other/SKILL.md"); !strings.Contains(out, "claude overrides:\n  .claude/settings.local.json: off") {
		t.Errorf("missing block for the skill Claude sees as Deploy:\n%s", out)
	}
}

func TestExplain_SkillOverridesSecondKeyInOneFileIsNotApplied(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	skillOverrideHome(t)
	writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", "deploy")
	mustWrite(t, filepath.Join(".claude", "settings.local.json"), `{"skillOverrides":{"deploy":"off","Deploy":"off"}}`)
	want := "claude overrides:\n" +
		"  .claude/settings.local.json: off (Claude Code refuses to run it; change it in /skills)\n" +
		"  .claude/settings.local.json: off (not applied: another key in this file names the skill)\n"
	if got := explainOverridesBlock(runExplain(t, deploySkill)); got != want {
		t.Errorf("block:\n%s\nwant:\n%s", got, want)
	}
}

func TestExplain_SkillOverridesMarksConflictingKeysInOneFile(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	home := skillOverrideHome(t)
	writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", "deploy")
	mustWrite(t, filepath.Join(".claude", "settings.local.json"), `{"skillOverrides":{"Deploy":"on","deploy":"off"}}`)
	mustWrite(t, filepath.Join(home, ".claude", "settings.json"), `{"skillOverrides":{"deploy":"name-only"}}`)
	want := "claude overrides:\n" +
		"  .claude/settings.local.json: on (conflicts with another key in this file)\n" +
		"  .claude/settings.local.json: off (conflicts with another key in this file)\n" +
		"  ~/.claude/settings.json: name-only (not applied: a higher file sets it)\n"
	if got := explainOverridesBlock(runExplain(t, deploySkill)); got != want {
		t.Errorf("block:\n%s\nwant:\n%s", got, want)
	}
	var got explainOutput
	if err := json.Unmarshal([]byte(runExplain(t, deploySkill, "--json")), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Overrides) != 3 || !got.Overrides[0].Conflict || !got.Overrides[1].Conflict || got.Overrides[0].Effective || got.Overrides[2].Effective {
		t.Errorf("overrides = %+v", got.Overrides)
	}
}

func TestExplain_SkillOverridesFileWithRejectedValueIsUncertain(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	home := skillOverrideHome(t)
	writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", "deploy")
	mustWrite(t, filepath.Join(".claude", "settings.local.json"), `{"skillOverrides":{"deploy":"off","mine":"Off"}}`)
	mustWrite(t, filepath.Join(home, ".claude", "settings.json"), `{"skillOverrides":{"deploy":"name-only"}}`)
	want := "claude overrides:\n" +
		"  .claude/settings.local.json: off (Claude Code refuses to run it; this file has a rejected value, so Claude Code may skip it)\n" +
		"  ~/.claude/settings.json: name-only (listed without its description; applies to every project)\n"
	if got := explainOverridesBlock(runExplain(t, deploySkill)); got != want {
		t.Errorf("block:\n%s\nwant:\n%s", got, want)
	}
	var got explainOutput
	if err := json.Unmarshal([]byte(runExplain(t, deploySkill, "--json")), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Overrides) != 2 || !got.Overrides[0].Uncertain || got.Overrides[1].Uncertain || !got.Overrides[1].Effective {
		t.Errorf("overrides = %+v", got.Overrides)
	}
}

func TestExplain_SkillOverridesConflictingKeysStillOverruleLowerFiles(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	skillOverrideHome(t)
	writeSkillOverrideProject(t, "version: 1\ntargets: [claude]\n", "deploy")
	mustWrite(t, filepath.Join(".claude", "settings.json"), `{"skillOverrides":{"deploy":"off"}}`)
	mustWrite(t, filepath.Join(".claude", "settings.local.json"), `{"skillOverrides":{"Deploy":"on","deploy":"name-only"}}`)
	want := "claude overrides:\n" +
		"  .claude/settings.local.json: on (conflicts with another key in this file)\n" +
		"  .claude/settings.local.json: name-only (conflicts with another key in this file)\n" +
		"  .claude/settings.json: off (not applied: a higher file sets it)\n"
	if got := explainOverridesBlock(runExplain(t, deploySkill)); got != want {
		t.Errorf("block:\n%s\nwant:\n%s", got, want)
	}
}
