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
	cfg := &config.Config{Targets: []string{"codex"}}

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
	cfg := &config.Config{Targets: []string{"codex"}}

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
	cfg := &config.Config{Targets: []string{"codex"}, Sync: config.SyncConfig{Unmanaged: []string{filepath.ToSlash(generated)}}}

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

// Tracked files that only mention the marker, like the header package's
// own source or a docs page quoting the header, are not outputs. Neither
// are fixture copies of real outputs: a Go testdata tree or a nested
// project's own tree. Without a ledger none of them may be reported, and
// doctor --fix must not delete them.
func TestLeftoverOutputs_WithoutLedgerTrustsOnlyRealOutputHeaders(t *testing.T) {
	dir, git := gitRepo(t)
	testutil.Chdir(t, dir)
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	notOutputs := map[string]string{
		"internal/adapters/header/header.go":        "// Package header\npackage header\n\nconst Marker = \"" + header.Marker + "\"\n",
		"docs/site/content/docs/targets/cx.md":      "---\ntitle: Codex\n---\n\nEach file opens with `" + strings.TrimSpace(header.Line(header.FormatMarkdown)) + "`.\n",
		"services/api/AGENTS.md":                    "# API\n\n" + header.Line(header.FormatMarkdown),
		".codex/agents/notes.md":                    "Notes. " + header.Marker + " files are not edited by hand.\n",
		"internal/x/testdata/golden/AGENTS.md":      header.With("fixture\n", header.FormatMarkdown),
		"tests/fixtures/agnostic-ai.yaml":           "version: 1\ntargets: [codex]\n",
		"tests/fixtures/codex/AGENTS.md":            header.With("fixture\n", header.FormatMarkdown),
		"tests/fixtures/codex/.codex/agents/a.toml": header.With("name = \"a\"\n", header.FormatTOML),
	}
	for p, body := range notOutputs {
		mustWriteFile(t, filepath.FromSlash(p), body)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "base")

	if got := leftoverOutputs(&config.Config{Targets: []string{"codex"}}, map[string]bool{}); len(got) != 0 {
		t.Errorf("files that are not outputs reported as leftover: %v", got)
	}

	_, _ = runCLI(t, "doctor", "--fix")
	for p := range notOutputs {
		if !fileExists(filepath.FromSlash(p)) {
			t.Errorf("doctor --fix deleted %s", p)
		}
	}
}

// A ledger that records no outputs is a real record, not a missing one:
// the scan does not fall back to tracked files.
func TestLeftoverOutputs_EmptyLedgerDoesNotFallBack(t *testing.T) {
	dir, git := gitRepo(t)
	testutil.Chdir(t, dir)
	cfg := &config.Config{Targets: []string{"codex"}}
	generated := filepath.Join("services", "api", "AGENTS.md")
	mustWriteFile(t, generated, header.With("api rule body\n", header.FormatMarkdown))
	mustWriteFile(t, stateFilePath("."), "{\"version\":4,\"outputs\":[]}\n")
	git("add", "-A")
	git("commit", "-q", "-m", "base")

	if got := leftoverOutputs(cfg, map[string]bool{}); len(got) != 0 {
		t.Errorf("empty ledger fell back to tracked files: %v", got)
	}
}

// With no ledger, doctor --fix removes a leftover only while the header
// still opens it, even if a report names a file that just mentions the
// marker.
func TestFixDrift_WithoutLedgerKeepsFileWithoutLeadingHeader(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mention := filepath.Join("internal", "adapters", "header", "header.go")
	mustWriteFile(t, mention, "// Package header\npackage header\n\nconst Marker = \""+header.Marker+"\"\n")
	generated := filepath.Join("services", "api", "AGENTS.md")
	mustWriteFile(t, generated, header.With("api rule body\n", header.FormatMarkdown))

	if _, err := fixDrift([]driftReport{{Target: ledgerReport, Leftover: []string{mention, generated}}}, false); err != nil {
		t.Fatal(err)
	}

	if !fileExists(mention) {
		t.Errorf("fixDrift deleted %s, which only mentions the marker", mention)
	}
	if fileExists(generated) {
		t.Errorf("fixDrift kept %s, which opens with the header", generated)
	}
}

// A leftover in a tool directory counts too, even when the current sync
// writes nothing else there: the target's native locations name it.
func TestLeftoverOutputs_WithoutLedgerReportsToolDirLeftover(t *testing.T) {
	dir, git := gitRepo(t)
	testutil.Chdir(t, dir)
	cfg := &config.Config{Targets: []string{"codex"}}
	generated := filepath.Join(".codex", "agents", "old.toml")
	mustWriteFile(t, generated, header.With("name = \"old\"\n", header.FormatTOML))
	git("add", "-A")
	git("commit", "-q", "-m", "base")

	got := leftoverOutputs(cfg, map[string]bool{})

	if len(got) != 1 || got[0] != generated {
		t.Errorf("got %v, want [%s]", got, generated)
	}
}
