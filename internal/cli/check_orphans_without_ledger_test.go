package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Without a ledger (deleted, or a fresh checkout of a repo that commits
// its generated files), leftoverOutputs cannot read a prior output list,
// so it falls back to scanning git-tracked files for the provenance
// header. A hand-authored file, header-less, is never flagged even
// though it is tracked too.
func TestLeftoverOutputs_FallsBackToTrackedFilesWithoutLedger(t *testing.T) {
	dir, git := gitRepo(t)
	testutil.Chdir(t, dir)
	cfg := &config.Config{Targets: []string{"claude"}}

	generated := filepath.Join("services", "api", "AGENTS.md")
	mustWriteFile(t, generated, header.With("api rule body\n", header.FormatMarkdown))
	handAuthored := "README.md"
	mustWriteFile(t, handAuthored, "hand-authored, no header\n")
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	// No .sync-state at all: readStateFile(".") returns the zero value.

	got := leftoverOutputs(cfg, map[string]bool{})

	if len(got) != 1 || got[0] != filepath.ToSlash(generated) {
		t.Errorf("got %v, want [%s]", got, filepath.ToSlash(generated))
	}
}

// A file the current sync still emits is never reported, ledger or not.
func TestLeftoverOutputs_WithoutLedgerSkipsEmittedFiles(t *testing.T) {
	dir, git := gitRepo(t)
	testutil.Chdir(t, dir)
	cfg := &config.Config{Targets: []string{"claude"}}

	generated := filepath.Join("services", "api", "AGENTS.md")
	mustWriteFile(t, generated, header.With("api rule body\n", header.FormatMarkdown))
	git("add", "-A")
	git("commit", "-q", "-m", "base")

	got := leftoverOutputs(cfg, map[string]bool{filepath.ToSlash(generated): true})

	if len(got) != 0 {
		t.Errorf("emitted file reported as leftover: %v", got)
	}
}

// A user-owned path never counts as a leftover, ledger or not.
func TestLeftoverOutputs_WithoutLedgerSkipsUnmanagedFiles(t *testing.T) {
	dir, git := gitRepo(t)
	testutil.Chdir(t, dir)
	generated := filepath.Join("services", "api", "AGENTS.md")
	mustWriteFile(t, generated, header.With("api rule body\n", header.FormatMarkdown))
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	cfg := &config.Config{Targets: []string{"claude"}, Sync: config.SyncConfig{Unmanaged: []string{filepath.ToSlash(generated)}}}

	got := leftoverOutputs(cfg, map[string]bool{})

	if len(got) != 0 {
		t.Errorf("unmanaged file reported as leftover: %v", got)
	}
}

// An end-to-end repro of #1334: a scoped rule's spec and the ledger are
// both deleted after a commit. sync --check must still catch the
// leftover output and fail, instead of silently exiting 0.
func TestCollectDrift_ReportsLeftoverWithoutLedger(t *testing.T) {
	dir, git := gitRepo(t)
	testutil.Chdir(t, dir)
	silence(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
	rule := filepath.Join(dir, ".agnostic-ai", "rules", "api.md")
	mustWriteFile(t, rule, "---\nname: api\nscope: services/api\n---\napi body\n")
	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	if err := os.Remove(rule); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(stateFilePath(dir)); err != nil {
		t.Fatal(err)
	}

	reports, err := collectDrift(nil)
	if err != nil {
		t.Fatal(err)
	}

	var leftover []string
	for _, r := range reports {
		leftover = append(leftover, r.Leftover...)
	}
	if len(leftover) == 0 {
		t.Fatalf("expected the orphaned scoped rule file reported as leftover, got reports: %+v", reports)
	}
	if !strings.Contains(strings.Join(leftover, " "), "services/api") {
		t.Errorf("leftover does not name the scoped rule output: %v", leftover)
	}
}
