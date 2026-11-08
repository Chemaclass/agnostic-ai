package cli

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func dryRunDeletes(t *testing.T, args ...string) []string {
	t.Helper()
	out, err := runSyncStdout(t, args...)
	if err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
	var got struct {
		Writes []fileRecord `json:"writes"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
	var deletes []string
	for _, w := range got.Writes {
		if w.Action == "delete" {
			deletes = append(deletes, w.Path)
		}
	}
	slices.Sort(deletes)
	return deletes
}

func TestSync_AllowEmptyTargetsDryRunNamesEachRemoval(t *testing.T) {
	syncedEmptyTargetsProject(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: []\n")
	captureLogOut(t)
	before := snapshotTree(t, ".")
	state, err := os.ReadFile(emptyTargetsStatePath)
	if err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"sync", "--allow-empty-targets", "--dry-run"},
		{"sync", "--allow-empty-targets", "--dry-run", "--diff"},
	} {
		out, err := runSyncStdout(t, args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		for _, p := range emptyTargetsOutputs {
			if !strings.Contains(out, "would remove "+p+"\n") {
				t.Errorf("%v must name %s:\n%s", args, p, out)
			}
		}
	}
	deletes := dryRunDeletes(t, "sync", "--allow-empty-targets", "--dry-run", "--json")

	if after := snapshotTree(t, "."); !maps.Equal(before, after) {
		t.Error("previews must write and delete nothing")
	}
	if after, _ := os.ReadFile(emptyTargetsStatePath); !bytes.Equal(state, after) {
		t.Error("previews must record nothing")
	}

	if out, err := runCLI(t, "sync", "--allow-empty-targets"); err != nil {
		t.Fatalf("sync --allow-empty-targets: %v\n%s", err, out)
	}
	after := snapshotTree(t, ".")
	var removed []string
	for p := range before {
		if _, ok := after[p]; !ok {
			removed = append(removed, filepath.ToSlash(p))
		}
	}
	slices.Sort(removed)
	if !slices.Equal(deletes, removed) {
		t.Errorf("--dry-run --json deletes %v, the real run removed %v", deletes, removed)
	}
}

func TestSync_AllowEmptyTargetsDryRunAfterRemovalIsSilent(t *testing.T) {
	syncedEmptyTargetsProject(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: []\n")
	captureLogOut(t)
	if out, err := runCLI(t, "sync", "--allow-empty-targets"); err != nil {
		t.Fatalf("sync --allow-empty-targets: %v\n%s", err, out)
	}

	for _, args := range [][]string{
		{"sync", "--allow-empty-targets", "--dry-run"},
		{"sync", "--allow-empty-targets", "--dry-run", "--diff"},
	} {
		out, err := runSyncStdout(t, args...)
		if err != nil || strings.Contains(out, "would remove") {
			t.Errorf("%v names no removal once files are gone, got err = %v:\n%s", args, err, out)
		}
	}
	if deletes := dryRunDeletes(t, "sync", "--allow-empty-targets", "--dry-run", "--json"); len(deletes) > 0 {
		t.Errorf("--dry-run --json deletes nothing once files are gone, got %v", deletes)
	}
}

func TestSync_EmptyTargetsDryRunWithoutAllowKeepsFiles(t *testing.T) {
	syncedEmptyTargetsProject(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: []\n")
	captureLogOut(t)
	for _, args := range [][]string{
		{"sync", "--dry-run"},
		{"sync", "--dry-run", "--diff"},
	} {
		out, err := runCLI(t, args...)
		if err != nil || strings.Contains(out, "would remove") || !strings.Contains(out, "targets lists no tool") {
			t.Errorf("%v warns and names no removal, got err = %v:\n%s", args, err, out)
		}
	}
	requireEmptyTargetsOutputs(t, true)
}

// generatedPaths lists every file and link under the project, apart
// from .git, without following links.
func generatedPaths(t *testing.T) map[string]bool {
	t.Helper()
	paths := map[string]bool{}
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		paths[filepath.ToSlash(path)] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

// requirePreviewMatchesRun checks that each dry run names exactly the
// paths the real `sync --allow-empty-targets` run then removes.
func requirePreviewMatchesRun(t *testing.T, args ...string) []string {
	t.Helper()
	base := append([]string{"sync"}, args...)
	base = append(base, "--allow-empty-targets")
	before := generatedPaths(t)
	deletes := dryRunDeletes(t, append(slices.Clone(base), "--dry-run", "--json")...)
	var outs []string
	for _, extra := range [][]string{{"--dry-run"}, {"--dry-run", "--diff"}} {
		out, err := runSyncStdout(t, append(slices.Clone(base), extra...)...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", extra, err, out)
		}
		outs = append(outs, out)
	}
	if after := generatedPaths(t); !maps.Equal(before, after) {
		t.Fatal("previews must write and delete nothing")
	}
	if out, err := runCLI(t, base...); err != nil {
		t.Fatalf("%v: %v\n%s", base, err, out)
	}
	after := generatedPaths(t)
	var removed []string
	for p := range before {
		if !after[p] {
			removed = append(removed, p)
		}
	}
	slices.Sort(removed)
	if !slices.Equal(deletes, removed) {
		t.Errorf("--dry-run --json deletes %v, the real run removed %v", deletes, removed)
	}
	for _, out := range outs {
		for _, line := range strings.Split(out, "\n") {
			if p, ok := strings.CutPrefix(line, "would remove "); ok && !slices.Contains(removed, p) {
				t.Errorf("preview names %s, which the real run keeps", p)
			}
		}
		for _, p := range removed {
			if !strings.Contains(out, "would remove "+p+"\n") {
				t.Errorf("preview must name %s:\n%s", p, out)
			}
		}
	}
	return removed
}

// A -t run still writes the named target and removes the others.
func TestSync_AllowEmptyTargetsDryRunWithTargetNamesTheOthers(t *testing.T) {
	syncedEmptyTargetsProject(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: []\n")
	captureLogOut(t)
	removed := requirePreviewMatchesRun(t, "-t", "claude")
	if !slices.Contains(removed, "AGENTS.md") {
		t.Errorf("the -t claude run must remove the codex files, removed %v", removed)
	}
	if slices.Contains(removed, "CLAUDE.md") {
		t.Errorf("the -t claude run keeps the claude files, removed %v", removed)
	}
}

func TestSync_AllowEmptyTargetsDryRunNamesSharedSkillLinks(t *testing.T) {
	for _, pinned := range [][]string{nil, {"-t", "claude"}} {
		t.Run(strings.Join(pinned, " "), func(t *testing.T) {
			t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
			dir := testutil.TempCwd(t)
			writeFile(t, filepath.Join(dir, ".agnostic-ai/skills/demo/SKILL.md"), "---\nname: demo\ndescription: Demo skill.\n---\n\nBody.\n")
			writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex, cursor]\nsync:\n  shared-skills: true\n")
			captureLogOut(t)
			if out, err := runCLI(t, "sync", "--all"); err != nil {
				t.Fatalf("first sync: %v\n%s", err, out)
			}
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: []\nsync:\n  shared-skills: true\n")
			removed := requirePreviewMatchesRun(t, pinned...)
			links := slices.ContainsFunc(removed, func(p string) bool {
				return strings.HasSuffix(p, "skills/demo")
			})
			if !links {
				t.Errorf("the run must remove a shared skill link, removed %v", removed)
			}
		})
	}
}

// mergedEmptyTargetsProject syncs claude, codex, and cursor with a hook
// and an MCP server, where .claude/settings.json also holds user keys.
func mergedEmptyTargetsProject(t *testing.T) {
	t.Helper()
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	testutil.TempCwd(t)
	writeFile(t, ".agnostic-ai/AGNOSTIC_AI.md", "Root.\n")
	writeFile(t, ".agnostic-ai/rules/style.md", "---\nname: style\n---\n\nUse tabs.\n")
	writeFile(t, ".agnostic-ai/hooks/status.yaml", "name: status\ndescription: show status\non: session-start\ncommand: git status --short\n")
	writeFile(t, ".agnostic-ai/mcps/fs.yaml", "name: fs\ncommand: npx\nargs: [\"-y\", \"x\"]\n")
	writeFile(t, ".claude/settings.json", "{\n  \"model\": \"opus\",\n  \"env\": {\"MINE\": \"1\"}\n}\n")
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex, cursor]\n")
	captureLogOut(t)
	if out, err := runCLI(t, "sync", "--all"); err != nil {
		t.Fatalf("first sync: %v\n%s", err, out)
	}
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: []\n")
}

func dryRunAction(t *testing.T, path string, args ...string) string {
	t.Helper()
	out, err := runSyncStdout(t, append(append([]string{"sync"}, args...), "--allow-empty-targets", "--dry-run", "--json")...)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var got struct {
		Writes  []fileRecord `json:"writes"`
		Skipped []fileRecord `json:"skipped"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, r := range append(got.Writes, got.Skipped...) {
		if r.Path == path {
			return r.Action
		}
	}
	return ""
}

func TestSync_AllowEmptyTargetsDryRunStripsSharedSettings(t *testing.T) {
	for _, pinned := range [][]string{nil, {"-t", "cursor"}} {
		t.Run(strings.Join(pinned, " "), func(t *testing.T) {
			mergedEmptyTargetsProject(t)
			if got := dryRunAction(t, ".claude/settings.json", pinned...); got != "update" {
				t.Errorf("settings with user keys are stripped, so the action is update, got %q", got)
			}
			diff, err := runSyncStdout(t, append(append([]string{"sync"}, pinned...), "--allow-empty-targets", "--dry-run", "--diff")...)
			if err != nil || !strings.Contains(diff, "--- .claude/settings.json (on disk)") {
				t.Errorf("--diff shows the stripped settings, got err = %v:\n%s", err, diff)
			}
			removed := requirePreviewMatchesRun(t, pinned...)
			if len(removed) == 0 {
				t.Error("the run must remove the other targets' files")
			}
			data, err := os.ReadFile(".claude/settings.json")
			if err != nil || !strings.Contains(string(data), "MINE") || strings.Contains(string(data), "hooks") {
				t.Errorf("sync strips its keys and keeps the file, got %q, %v", data, err)
			}
		})
	}
}

func TestSync_AllowEmptyTargetsDryRunKeepsAnEditedHooksFile(t *testing.T) {
	mergedEmptyTargetsProject(t)
	data, err := os.ReadFile(".cursor/hooks.json")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, ".cursor/hooks.json", strings.Replace(string(data), "git status --short", "git status", 1))
	if got := dryRunAction(t, ".cursor/hooks.json"); got == "delete" {
		t.Error("an edited value keeps the file, so the preview must not delete it")
	}
	requirePreviewMatchesRun(t)
	if _, err := os.Stat(".cursor/hooks.json"); err != nil {
		t.Errorf("sync keeps the edited file: %v", err)
	}
}

// An older sync left no record of its keys in .claude/settings.json, so
// the sweep keeps it and the preview must not name it.
func TestSync_AllowEmptyTargetsDryRunKeepsAnUnrecordedSettingsFile(t *testing.T) {
	mergedEmptyTargetsProject(t)
	markMergedUnrecorded(t, ".claude/settings.json")
	if got := dryRunAction(t, ".claude/settings.json"); got != "orphan" {
		t.Errorf("--dry-run --json action = %q, want orphan", got)
	}
	requirePreviewMatchesRun(t)
	if _, err := os.Stat(".claude/settings.json"); err != nil {
		t.Errorf("sync keeps the unrecorded file: %v", err)
	}
}

// A -t run of two tools that share skills relinks their skill folders,
// so the preview must not name those links.
func TestSync_AllowEmptyTargetsDryRunKeepsLinksTheRunRelinks(t *testing.T) {
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	testutil.TempCwd(t)
	for _, name := range []string{"s1", "s2"} {
		writeFile(t, ".agnostic-ai/skills/"+name+"/SKILL.md", "---\nname: "+name+"\ndescription: Skill "+name+".\n---\n\nBody.\n")
	}
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor, opencode, gemini]\nsync:\n  shared-skills: true\n")
	captureLogOut(t)
	if out, err := runCLI(t, "sync", "--all"); err != nil {
		t.Fatalf("first sync: %v\n%s", err, out)
	}
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: []\nsync:\n  shared-skills: true\n")
	removed := requirePreviewMatchesRun(t, "-t", "cursor,opencode")
	for _, p := range removed {
		if strings.HasPrefix(p, ".opencode/skills/") {
			t.Errorf("the run relinks %s, so it is not removed", p)
		}
	}
}

// With --keep-edits the run keeps a hand-edited output as an orphan.
func TestSync_AllowEmptyTargetsDryRunKeepEditsKeepsAnEditedFile(t *testing.T) {
	syncedEmptyTargetsProject(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: []\n")
	// The edit keeps the provenance header, which alone proves ownership.
	original, err := os.ReadFile("CLAUDE.md")
	if err != nil {
		t.Fatal(err)
	}
	edited := string(original) + "\nEdited by hand.\n"
	writeFile(t, "CLAUDE.md", edited)
	captureLogOut(t)
	deletes := dryRunDeletes(t, "sync", "--allow-empty-targets", "--keep-edits", "--dry-run", "--json")
	out, err := runSyncStdout(t, "sync", "--allow-empty-targets", "--keep-edits", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(deletes, "CLAUDE.md") || strings.Contains(out, "would remove CLAUDE.md") {
		t.Errorf("--keep-edits keeps the edited CLAUDE.md, got deletes %v:\n%s", deletes, out)
	}
	if out, err := runCLI(t, "sync", "--allow-empty-targets", "--keep-edits"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if data, err := os.ReadFile("CLAUDE.md"); err != nil || string(data) != edited {
		t.Errorf("sync --keep-edits kept CLAUDE.md, got %q, %v", data, err)
	}
}

// keptOrphanRestored leaves .claude/rules/style.md as an orphan the last
// sync kept for a hand edit, then puts its generated bytes back.
func keptOrphanRestored(t *testing.T) {
	t.Helper()
	syncedEmptyTargetsProject(t)
	captureLogOut(t)
	const rule = ".claude/rules/style.md"
	generated, err := os.ReadFile(rule)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, rule, "edited by hand\n")
	if err := os.Remove(".agnostic-ai/rules/style.md"); err != nil {
		t.Fatal(err)
	}
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if _, err := os.Stat(rule); err != nil {
		t.Fatalf("sync keeps the edited rule as an orphan: %v", err)
	}
	writeFile(t, rule, string(generated))
}

func TestSync_DryRunDiffNamesARestoredOrphan(t *testing.T) {
	keptOrphanRestored(t)
	out, err := runSyncStdout(t, "sync", "--dry-run", "--diff")
	if err != nil {
		t.Fatal(err)
	}
	named := strings.Contains(out, "would remove .claude/rules/style.md")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	_, statErr := os.Stat(".claude/rules/style.md")
	if named != (statErr != nil) {
		t.Errorf("preview names the removal = %v, real run removed it = %v:\n%s", named, statErr != nil, out)
	}
}

func TestSync_AllowEmptyTargetsDryRunNamesARestoredOrphan(t *testing.T) {
	keptOrphanRestored(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: []\n")
	if removed := requirePreviewMatchesRun(t); !slices.Contains(removed, ".claude/rules/style.md") {
		t.Errorf("the run removes the restored orphan, removed %v", removed)
	}
}
