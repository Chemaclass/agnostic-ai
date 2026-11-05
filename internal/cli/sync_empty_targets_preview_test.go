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

func runSyncStdout(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

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
			removed = append(removed, p)
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
