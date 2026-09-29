package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// mergedLeftovers folds the leftover reports into one, for tests that
// only ask what is reported.
func mergedLeftovers(cfg *config.Config, emitted map[string]bool) driftReport {
	merged := driftReport{Target: ledgerReport}
	for _, r := range leftoverReports(cfg, emitted) {
		merged.Leftover = append(merged.Leftover, r.Leftover...)
		merged.Orphaned = append(merged.Orphaned, r.Orphaned...)
		merged.Unledgered = merged.Unledgered || r.Unledgered
	}
	return merged
}

// Without a ledger (deleted, or a fresh checkout of a repo that commits
// its generated files), leftoverReport cannot read a prior output list,
// so it falls back to scanning git-tracked files for the provenance
// header. A hand-authored file, header-less, is never flagged even
// though it is tracked too. A scope's AGENTS.md is reported for manual
// removal, since nothing proves sync wrote it there.
func TestLeftoverReport_FallsBackToTrackedFilesWithoutLedger(t *testing.T) {
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

	got := mergedLeftovers(cfg, map[string]bool{})

	if !got.Unledgered || len(got.Leftover) != 0 || len(got.Orphaned) != 1 || got.Orphaned[0] != generated {
		t.Errorf("got %+v, want only %s, for manual removal", got, generated)
	}
}

// A file the current sync still emits is never reported, ledger or not.
func TestLeftoverReport_WithoutLedgerSkipsEmittedFiles(t *testing.T) {
	dir, git := gitRepo(t)
	testutil.Chdir(t, dir)
	cfg := &config.Config{Targets: []string{"codex"}}

	generated := filepath.Join("services", "api", "AGENTS.md")
	mustWriteFile(t, generated, header.With("api rule body\n", header.FormatMarkdown))
	git("add", "-A")
	git("commit", "-q", "-m", "base")

	got := mergedLeftovers(cfg, map[string]bool{generated: true})

	if got.hasDrift() {
		t.Errorf("emitted file reported as leftover: %v", got)
	}
}

// A user-owned path never counts as a leftover, ledger or not.
func TestLeftoverReport_WithoutLedgerSkipsUnmanagedFiles(t *testing.T) {
	dir, git := gitRepo(t)
	testutil.Chdir(t, dir)
	generated := filepath.Join("services", "api", "AGENTS.md")
	mustWriteFile(t, generated, header.With("api rule body\n", header.FormatMarkdown))
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	cfg := &config.Config{Targets: []string{"codex"}, Sync: config.SyncConfig{Unmanaged: []string{filepath.ToSlash(generated)}}}

	got := mergedLeftovers(cfg, map[string]bool{})

	if got.hasDrift() {
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

	var stranded []string
	for _, r := range reports {
		stranded = append(stranded, r.Leftover...)
		stranded = append(stranded, r.Orphaned...)
	}
	want := filepath.Join("services", "api", "AGENTS.md")
	if !slices.Contains(stranded, want) {
		t.Errorf("drift does not name the scoped rule output %s: %+v", want, reports)
	}
}

// Tracked files that only mention the marker, like the header package's
// own source or a docs page quoting the header, are not outputs. Neither
// are fixture copies of real outputs: a Go testdata tree or a nested
// project's own tree. Without a ledger none of them may be reported, and
// doctor --fix must not delete them.
func TestLeftoverReport_WithoutLedgerTrustsOnlyRealOutputHeaders(t *testing.T) {
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

	if got := mergedLeftovers(&config.Config{Targets: []string{"codex"}}, map[string]bool{}); got.hasDrift() {
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
func TestLeftoverReport_EmptyLedgerDoesNotFallBack(t *testing.T) {
	dir, git := gitRepo(t)
	testutil.Chdir(t, dir)
	cfg := &config.Config{Targets: []string{"codex"}}
	generated := filepath.Join("services", "api", "AGENTS.md")
	mustWriteFile(t, generated, header.With("api rule body\n", header.FormatMarkdown))
	mustWriteFile(t, stateFilePath("."), "{\"version\":4,\"outputs\":[]}\n")
	git("add", "-A")
	git("commit", "-q", "-m", "base")

	if got := mergedLeftovers(cfg, map[string]bool{}); got.hasDrift() || got.Unledgered {
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
	generated := filepath.Join(".codex", "agents", "old.toml")
	mustWriteFile(t, generated, header.With("name = \"old\"\n", header.FormatTOML))

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
func TestLeftoverReport_WithoutLedgerReportsToolDirLeftover(t *testing.T) {
	dir, git := gitRepo(t)
	testutil.Chdir(t, dir)
	cfg := &config.Config{Targets: []string{"codex"}}
	generated := filepath.Join(".codex", "agents", "old.toml")
	mustWriteFile(t, generated, header.With("name = \"old\"\n", header.FormatTOML))
	git("add", "-A")
	git("commit", "-q", "-m", "base")

	got := mergedLeftovers(cfg, map[string]bool{})

	if !got.Unledgered || len(got.Orphaned) != 0 || len(got.Leftover) != 1 || got.Leftover[0] != generated {
		t.Errorf("got %+v, want Leftover [%s]", got, generated)
	}
}

// Sync keeps a leftover no ledger proves it wrote, so the hint names the
// command that removes it. With a ledger, sync's sweep removes it.
func TestReportCheckDrift_NamesDoctorFixForLeftoverWithoutLedger(t *testing.T) {
	leftover := filepath.Join(".codex", "agents", "old.toml")
	for _, tc := range []struct {
		name       string
		unledgered bool
		want, not  []string
	}{
		{"no ledger", true, []string{"run `agnostic-ai doctor --fix` to remove", "run agnostic-ai doctor --fix to remove it", "to reconcile, run: agnostic-ai doctor --fix"}, []string{"run `agnostic-ai sync` to remove", "run agnostic-ai sync to remove it"}},
		{"ledger", false, []string{"run `agnostic-ai sync` to remove", "run agnostic-ai sync to remove it", "to reconcile, run: agnostic-ai sync\n"}, []string{"doctor --fix"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reports := []driftReport{{Target: ledgerReport, Leftover: []string{leftover}, Unledgered: tc.unledgered}}
			var out bytes.Buffer
			for _, format := range []string{checkFormatHuman, checkFormatGitHub} {
				cmd := &cobra.Command{}
				cmd.SetOut(&out)
				cmd.SetErr(&out)
				prev := logOut
				logOut = &out
				err := reportCheckDrift(cmd, reports, format, false)
				logOut = prev
				if err == nil {
					t.Errorf("%s: leftover drift must fail the check", format)
				}
			}
			for _, w := range tc.want {
				if !strings.Contains(out.String(), w) {
					t.Errorf("output lacks %q:\n%s", w, out.String())
				}
			}
			for _, n := range tc.not {
				if strings.Contains(out.String(), n) {
					t.Errorf("output has %q:\n%s", n, out.String())
				}
			}
		})
	}
}

// A scope's AGENTS.md found only through tracked files may be a copy, such
// as docs/guide/AGENTS.md pasted from a generated one, or a vendored file.
// It is reported with a hint to delete it by hand, and doctor --fix leaves
// it on disk.
func TestDoctorFix_WithoutLedgerLeavesScopedDocumentForManualRemoval(t *testing.T) {
	dir, git := gitRepo(t)
	testutil.Chdir(t, dir)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	copied := filepath.Join("docs", "guide", "AGENTS.md")
	mustWriteFile(t, copied, header.With("copied from a generated file\n", header.FormatMarkdown))
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	var out bytes.Buffer
	prev := logOut
	logOut = &out
	defer func() { logOut = prev }()

	checkOut, checkErr := runCLI(t, "sync", "--check")
	_, fixErr := runCLI(t, "doctor", "--fix")

	all := out.String() + checkOut
	if checkErr == nil {
		t.Error("sync --check passed with a stranded scoped document")
	}
	if !strings.Contains(all, filepath.ToSlash(copied)) || !strings.Contains(all, "delete them by hand if stale") {
		t.Errorf("output does not name %s with the manual hint:\n%s", copied, all)
	}
	if !fileExists(copied) {
		t.Errorf("doctor --fix deleted %s", copied)
	}
	if fixErr == nil {
		t.Error("doctor --fix passed while a file still needs manual removal")
	}
}

// When the only drift is a scope document no ledger proves sync wrote,
// neither sync nor doctor --fix can settle it, so doctor's closing advice
// and its error name the file and the manual step instead (#1392).
func TestDoctor_UnledgeredScopeDocumentAdvisesManualRemoval(t *testing.T) {
	dir, git := gitRepo(t)
	testutil.Chdir(t, dir)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	syncProject(t)
	if err := os.Remove(filepath.Join(".agnostic-ai", ".sync-state")); err != nil {
		t.Fatal(err)
	}
	copied := filepath.Join("docs", "guide", "AGENTS.md")
	mustWriteFile(t, copied, header.With("copied from a generated file\n", header.FormatMarkdown))
	git("add", "-A")
	git("commit", "-q", "-m", "base")

	out, err := runCLI(t, "doctor")

	if err == nil || !strings.Contains(err.Error(), "by hand") || strings.Contains(err.Error(), "doctor --fix") {
		t.Errorf("error = %v, want the manual step and no doctor --fix advice", err)
	}
	_, next, _ := strings.Cut(out, "Next step:")
	for _, want := range []string{filepath.ToSlash(copied), "by hand if stale", "sync.unmanaged"} {
		if !strings.Contains(next, want) {
			t.Errorf("next step does not say %q:\n%s", want, next)
		}
	}
	for _, unwanted := range []string{"agnostic-ai sync", "doctor --fix"} {
		if strings.Contains(next, unwanted) {
			t.Errorf("next step advises %q, which cannot remove the file:\n%s", unwanted, next)
		}
	}
}

// The check footer names what settles the unledgered drift found:
// doctor --fix for a removable leftover, and a manual step for a scope
// document, which doctor --fix leaves in place (#1362).
func TestReportCheckDrift_FooterMatchesUnledgeredFindings(t *testing.T) {
	removable := filepath.Join(".codex", "agents", "old.toml")
	scoped := filepath.Join("services", "api", "AGENTS.md")
	const manual = "delete the files that look generated, listed above, by hand if stale, or list them under sync.unmanaged"
	for _, tc := range []struct {
		name     string
		report   driftReport
		want     string
		notWants []string
	}{
		{"only scope documents", driftReport{Orphaned: []string{scoped}},
			"to reconcile, " + manual + "\n", []string{"doctor --fix", "run: agnostic-ai"}},
		{"mixed", driftReport{Leftover: []string{removable}, Orphaned: []string{scoped}},
			"to reconcile, run: agnostic-ai doctor --fix, then " + manual + "\n", nil},
		{"only removable", driftReport{Leftover: []string{removable}},
			"to reconcile, run: agnostic-ai doctor --fix\n", []string{"by hand"}},
	} {
		for _, format := range []string{checkFormatHuman, checkFormatGitHub} {
			t.Run(tc.name+"/"+format, func(t *testing.T) {
				rep := tc.report
				rep.Target, rep.Unledgered = unledgeredReportTarget, true
				var stdout, stderr bytes.Buffer
				cmd := &cobra.Command{}
				cmd.SetOut(&stdout)
				cmd.SetErr(&stderr)
				prev := logOut
				logOut = &stdout
				defer func() { logOut = prev }()

				if err := reportCheckDrift(cmd, []driftReport{rep}, format, false); err == nil {
					t.Error("unledgered drift must fail the check")
				}

				if !strings.Contains(stderr.String(), tc.want) {
					t.Errorf("footer lacks %q:\n%s", tc.want, stderr.String())
				}
				for _, n := range tc.notWants {
					if strings.Contains(stderr.String(), n) {
						t.Errorf("footer has %q:\n%s", n, stderr.String())
					}
				}
			})
		}
	}
}
