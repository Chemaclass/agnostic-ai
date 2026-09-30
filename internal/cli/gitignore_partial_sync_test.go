package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const (
	partialSyncConfig         = "version: 1\ntargets: [claude, codex]\ngitignore:\n  enabled: true\n"
	partialSyncManifestConfig = partialSyncConfig + "sync:\n  output-manifest: true\n"

	partialSyncStyleRule = "---\nname: style\ndescription: Style.\n---\nKeep it short.\n"
	// Only codex routes this rule into a scope, inside a dot folder that holds
	// the user's own files.
	partialSyncScopedRule = "---\nname: wf\ndescription: Workflows.\nx-codex:\n  scope: .github/workflows\n---\nPin actions.\n"
)

func partialSyncRun(t *testing.T, args ...string) {
	t.Helper()
	if out, err := runCLI(t, append([]string{"sync"}, args...)...); err != nil {
		t.Fatalf("sync %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// partialSyncProject runs a full sync of a claude and codex project and
// returns the files that carry the managed block.
func partialSyncProject(t *testing.T, configYAML, rule string) map[string]string {
	t.Helper()
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), configYAML)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), rule)
	partialSyncRun(t, "--all")
	return partialSyncBlockFiles(t)
}

func partialSyncBlockFiles(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{}
	for _, name := range []string{".gitignore", worktreeIncludeFile} {
		files[name] = readFile(t, name)
	}
	return files
}

func partialSyncAssertUnchanged(t *testing.T, full map[string]string, args []string) {
	t.Helper()
	run := "sync " + strings.Join(args, " ")
	for name, got := range partialSyncBlockFiles(t) {
		if got != full[name] {
			t.Errorf("%s rewrote %s:\n%s", run, name,
				labeledDiff("full sync", run, splitLines(full[name]), splitLines(got), diffBodyMax))
		}
	}
}

// The managed block lists the same paths whichever targets a run emitted:
// every configured target keeps its ignore-only paths, and the committed
// output manifest is never ignored, not even by sync --json.
func TestSync_ManagedBlockDoesNotDependOnTheTargetsRun(t *testing.T) {
	runs := []struct {
		name string
		args []string
	}{
		{"only", []string{"--only", "codex"}},
		{"except", []string{"--except", "claude"}},
		{"target", []string{"-t", "codex"}},
		{"only json", []string{"--only", "codex", "--json"}},
		{"full json", []string{"--all", "--json"}},
	}
	for _, run := range runs {
		t.Run(run.name, func(t *testing.T) {
			full := partialSyncProject(t, partialSyncManifestConfig, partialSyncStyleRule)
			block := full[".gitignore"]
			if !fileExists(outputManifestPath) {
				t.Fatalf("full sync wrote no %s", outputManifestPath)
			}
			if strings.Contains(block, "outputs.lock") {
				t.Fatalf("full sync ignores the committed manifest:\n%s", block)
			}
			if !strings.Contains(block, "/.claude/settings.local.json\n") {
				t.Fatalf("full sync lacks claude's ignore-only paths:\n%s", block)
			}

			partialSyncRun(t, run.args...)

			partialSyncAssertUnchanged(t, full, run.args)
		})
	}
}

// A sync of some targets records every path in the ledger, and the ledger
// lists the manifest, so the block itself must keep it out of the ignores.
func TestBuildManagedBlock_NeverIgnoresTheOutputManifest(t *testing.T) {
	block := buildManagedBlock(&config.Config{}, []string{outputManifestPath, "./" + outputManifestPath}, nil)

	for _, e := range block {
		if strings.Contains(e, "outputs.lock") {
			t.Errorf("managed block ignores the committed manifest %q, got %v", e, block)
		}
	}
}

// A scope only one target resolves stays visible when a run leaves that
// target out, or the ignore widens to the whole directory and hides the
// user's own files in it.
func TestSync_PartialRunKeepsScopesOfTargetsItLeftOut(t *testing.T) {
	full := partialSyncProject(t, partialSyncConfig, partialSyncScopedRule)
	if !strings.Contains(full[".gitignore"], "/.github/workflows/AGENTS.md\n") {
		t.Fatalf("full sync does not list the scoped file:\n%s", full[".gitignore"])
	}

	args := []string{"--only", "claude"}
	partialSyncRun(t, args...)

	partialSyncAssertUnchanged(t, full, args)
}
