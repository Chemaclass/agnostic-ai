package integration

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestProjectSkillPriority_ReportsNativeLimitWithoutChangingHome(t *testing.T) {
	repository, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "agnostic-ai")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = repository
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	home, project := t.TempDir(), t.TempDir()
	source := filepath.Join(home, "source")
	for key, value := range map[string]string{
		"HOME": home, "USERPROFILE": home, "AGNOSTIC_AI_HOME": source,
		"CLAUDE_CONFIG_DIR": filepath.Join(home, ".claude"), "GEMINI_CLI_HOME": home,
		"XDG_CONFIG_HOME": filepath.Join(home, ".config"), "AGNOSTIC_AI_NO_UPDATE_CHECK": "1",
	} {
		t.Setenv(key, value)
	}
	write := func(path, body string) {
		t.Helper()
		must(t, os.MkdirAll(filepath.Dir(path), 0o755))
		must(t, os.WriteFile(path, []byte(body), 0o644))
	}
	homeSkill := "---\nname: gh-issue\ndescription: Personal issue workflow.\n---\nHOME_SKILL_SENTINEL\n"
	write(filepath.Join(source, "agnostic-ai.yaml"), "version: 1\ntargets: [claude,gemini]\n")
	write(filepath.Join(source, "skills/gh-issue/SKILL.md"), homeSkill)
	for _, tool := range []string{".claude", ".gemini"} {
		write(filepath.Join(home, tool, "skills/gh-issue/SKILL.md"), homeSkill)
		write(filepath.Join(home, tool, "settings.json"), "{\"unrelated\":\"HOME_SETTINGS_SENTINEL\"}\n")
	}
	write(filepath.Join(project, "agnostic-ai.yaml"), "version: 1\ntargets: [claude,gemini]\n")
	write(filepath.Join(project, ".agnostic-ai/skills/gh-issue/SKILL.md"), "---\nname: gh-issue\ndescription: Team issue workflow.\n---\nPROJECT_SKILL_SENTINEL\n")
	snapshot := func() map[string]string {
		t.Helper()
		state := map[string]string{}
		err := filepath.WalkDir(home, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(home, path)
			if err != nil {
				return err
			}
			value := info.Mode().String()
			if !entry.IsDir() {
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				value += string(data)
			}
			state[rel] = value
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return state
	}
	before := snapshot()
	run := func(args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Dir = project
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v: %s", args, err, out)
		}
		return string(out)
	}
	sync := run("sync")
	doctor := run("doctor")
	for _, output := range []string{sync, doctor} {
		for _, text := range []string{"claude loads the global one", "gemini loads this one", "project sync cannot select this skill instead on claude"} {
			if !strings.Contains(output, text) {
				t.Errorf("missing selection result %q: %s", text, output)
			}
		}
	}
	for _, tool := range []string{".claude", ".gemini"} {
		data, err := os.ReadFile(filepath.Join(project, tool, "skills/gh-issue/SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "PROJECT_SKILL_SENTINEL") || strings.Contains(string(data), "HOME_SKILL_SENTINEL") {
			t.Errorf("project skill changed: %s", data)
		}
	}
	write(filepath.Join(project, "agnostic-ai.local.yaml"), "sync:\n  allow-global-names: [gh-issue]\n")
	if output := run("sync"); strings.Contains(output, "also exists in") {
		t.Errorf("allowed sync still warns: %s", output)
	}
	if output := run("doctor"); !strings.Contains(output, "project sync cannot select this skill instead on claude") || !strings.Contains(output, "allowed by sync.allow-global-names") {
		t.Errorf("doctor hides allowed selection limit: %s", output)
	}
	if after := snapshot(); !reflect.DeepEqual(after, before) {
		t.Errorf("project sync/doctor changed home files: before %#v, after %#v", before, after)
	}
}
