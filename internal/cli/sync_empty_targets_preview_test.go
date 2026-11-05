package cli

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"
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

// A -t run still writes the named target, so its files are not removals.
func TestSync_AllowEmptyTargetsDryRunKeepsANamedTarget(t *testing.T) {
	syncedEmptyTargetsProject(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: []\n")
	captureLogOut(t)
	for _, p := range dryRunDeletes(t, "sync", "-t", "claude", "--allow-empty-targets", "--dry-run", "--json") {
		if p == "CLAUDE.md" || strings.HasPrefix(p, ".claude/") {
			t.Errorf("a -t claude preview must not remove %s", p)
		}
	}
}
