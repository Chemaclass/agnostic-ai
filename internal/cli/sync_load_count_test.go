package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// countProjectLoads counts the project loads that reach the spec loader.
func countProjectLoads(t *testing.T) *int {
	t.Helper()
	loads := 0
	original := loadSpecLayers
	loadSpecLayers = func(layers []spec.Layer) (spec.Bundle, error) {
		loads++
		return original(layers)
	}
	t.Cleanup(func() { loadSpecLayers = original })
	return &loads
}

func TestSync_LoadsTheProjectOnce(t *testing.T) {
	for _, args := range [][]string{
		{"sync"},
		{"sync", "--json"},
		{"sync", "--dry-run"},
		{"sync", "--check"},
		{"sync", "--plan"},
		{"sync", "--plan", "--check"},
		{"sync", "--dry-run", "--diff"},
		{"sync", "--json", "--dry-run"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			syncedFixture(t)
			loads := countProjectLoads(t)

			if out, err := runCLI(t, args...); err != nil {
				t.Fatalf("%v: %v\n%s", args, err, out)
			}

			if *loads != 1 {
				t.Errorf("%v loaded the project %d times, want 1", args, *loads)
			}
		})
	}
}

// The picker saves the chosen targets to the config, so the pass must
// run on the config as saved, and a check right after finds no drift.
func TestSync_PickedTargetsSyncFromTheSavedConfig(t *testing.T) {
	for _, args := range [][]string{{"sync"}, {"sync", "--json"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			dir := setupSharedSkillsFixture(t, "version: 1\ntargets:\n"+listTargets(allTargetNames()))
			testutil.Chdir(t, dir)
			writeFile(t, ".agnostic-ai/rules/r1.md", "---\nname: r1\n---\nrule body\n")
			stdout, stderr := captureLogOut(t), captureNotes(t)
			root := NewRootCmd("test")
			root.SetOut(stdout)
			root.SetErr(stderr)
			root.SetIn(strings.NewReader("claude,cursor\n"))
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatalf("%v: %v\n%s", args, err, stderr)
			}
			loads := countProjectLoads(t)

			if out, err := runCLI(t, "sync", "--check"); err != nil {
				t.Fatalf("sync --check after the pick: %v\n%s", err, out)
			}
			if *loads != 1 {
				t.Errorf("sync --check loaded the project %d times, want 1", *loads)
			}
		})
	}
}
