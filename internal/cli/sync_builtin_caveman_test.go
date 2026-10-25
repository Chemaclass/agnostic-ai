package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

const cavemanTestTargets = "[claude, codex, copilot, cursor, gemini, windsurf, kiro, opencode]"

// writeCavemanSourceSkill stands in for an earlier release that wrote
// upstream license and notice files beside the caveman skill.
func writeCavemanSourceSkill(t *testing.T, skills string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(cavemanTestPackageDir, "..", "builtins", "data", "caveman", "skills", "caveman", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	mustWriteGlobalTest(t, filepath.Join(skills, "caveman", "SKILL.md"), string(body))
	mustWriteGlobalTest(t, filepath.Join(skills, "caveman", "LICENSE"), "Upstream license text.\n")
	mustWriteGlobalTest(t, filepath.Join(skills, "caveman", "NOTICE"), "Upstream notice text.\n")
}

var cavemanTestPackageDir = func() string {
	wd, _ := os.Getwd()
	return wd
}()

func cavemanSkillDirs(t *testing.T, root string) []string {
	t.Helper()
	var dirs []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == ".agnostic-ai" || entry.IsDir() && entry.Name() == "source" {
			return filepath.SkipDir
		}
		if !entry.IsDir() && entry.Name() == "SKILL.md" && filepath.Base(filepath.Dir(path)) == "caveman" {
			dirs = append(dirs, filepath.Dir(path))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(dirs)
	return dirs
}

func assertCavemanSkillOnly(t *testing.T, dirs []string) {
	t.Helper()
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 || entries[0].Name() != "SKILL.md" {
			var names []string
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			t.Errorf("%s holds %v, want only SKILL.md", dir, names)
		}
	}
}

func TestSyncBuiltinCaveman_RemovesPreviouslyOwnedUpstreamFiles(t *testing.T) {
	silence(t)
	root := t.TempDir()
	chdir(t, root)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	writeCavemanSourceSkill(t, filepath.Join(root, ".agnostic-ai", "skills"))
	mustWriteGlobalTest(t, "agnostic-ai.yaml", "version: 1\ntargets: "+cavemanTestTargets+"\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	before := cavemanSkillDirs(t, root)
	if len(before) < 3 {
		t.Fatalf("caveman skill dirs = %v, want one per skills folder", before)
	}
	for _, dir := range before {
		if _, err := os.Stat(filepath.Join(dir, "LICENSE")); err != nil {
			t.Fatalf("setup did not own the upstream files: %v", err)
		}
	}

	if err := os.RemoveAll(filepath.Join(root, ".agnostic-ai", "skills", "caveman")); err != nil {
		t.Fatal(err)
	}
	mustWriteGlobalTest(t, "agnostic-ai.yaml", "version: 1\ntargets: "+cavemanTestTargets+"\nbuiltins: [caveman]\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync with the built-in: %v\n%s", err, out)
	}
	if after := cavemanSkillDirs(t, root); !slices.Equal(after, before) {
		t.Errorf("caveman skill dirs = %v, want %v", after, before)
	}
	assertCavemanSkillOnly(t, before)

	handWritten := filepath.Join(before[0], "LICENSE")
	mustWriteGlobalTest(t, handWritten, "A license I wrote.\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync after a hand-written file: %v\n%s", err, out)
	}
	if body, err := os.ReadFile(handWritten); err != nil || string(body) != "A license I wrote.\n" {
		t.Errorf("sync changed a hand-written file: %q %v", body, err)
	}
}

func TestSyncGlobalBuiltinCaveman_RemovesPreviouslyOwnedUpstreamFiles(t *testing.T) {
	home, source := globalAgentTestHome(t)
	writeCavemanSourceSkill(t, filepath.Join(source, "skills"))
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: "+cavemanTestTargets+"\n")
	if _, warnings, err := runGlobalAgentTest(); err != nil {
		t.Fatalf("sync --global: %v\n%s", err, warnings)
	}
	before := cavemanSkillDirs(t, home)
	if len(before) < 3 {
		t.Fatalf("caveman skill dirs = %v, want one per skills folder", before)
	}
	for _, dir := range before {
		if _, err := os.Stat(filepath.Join(dir, "LICENSE")); err != nil {
			t.Fatalf("setup did not own the upstream files: %v", err)
		}
	}

	if err := os.RemoveAll(filepath.Join(source, "skills", "caveman")); err != nil {
		t.Fatal(err)
	}
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: "+cavemanTestTargets+"\nbuiltins: [caveman]\n")
	if _, warnings, err := runGlobalAgentTest(); err != nil {
		t.Fatalf("sync --global with the built-in: %v\n%s", err, warnings)
	}
	if after := cavemanSkillDirs(t, home); !slices.Equal(after, before) {
		t.Errorf("caveman skill dirs = %v, want %v", after, before)
	}
	assertCavemanSkillOnly(t, before)

	handWritten := filepath.Join(before[0], "NOTICE")
	mustWriteGlobalTest(t, handWritten, "A notice I wrote.\n")
	if _, warnings, err := runGlobalAgentTest(); err != nil {
		t.Fatalf("sync --global after a hand-written file: %v\n%s", err, warnings)
	}
	if body, err := os.ReadFile(handWritten); err != nil || string(body) != "A notice I wrote.\n" {
		t.Errorf("sync --global changed a hand-written file: %q %v", body, err)
	}
}
