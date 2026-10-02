package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Paths lostLedgerProject leaves behind: two leftovers that open with the
// provenance header and no spec produces, and two files at output
// locations that do not open with it.
var (
	unledgeredToolFile  = filepath.Join(".codex", "agents", "old.toml")
	unledgeredScopedDoc = filepath.Join("services", "api", "AGENTS.md")
	headerlessToolFile  = filepath.Join(".codex", "agents", "hand.toml")
	mentionScopedDoc    = filepath.Join("services", "web", "AGENTS.md")
)

// lostLedgerProject builds a repo that commits its generated files, then
// loses .sync-state after a scoped rule was deleted (#1354).
func lostLedgerProject(t *testing.T) {
	t.Helper()
	lostLedgerProjectFor(t, "codex")
}

// lostLedgerProjectFor is lostLedgerProject with a comma-separated
// target list, which must include codex.
func lostLedgerProjectFor(t *testing.T, targets string) {
	t.Helper()
	dir, git := gitRepo(t)
	testutil.Chdir(t, dir)
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+targets+"]\n")
	rule := filepath.Join(".agnostic-ai", "rules", "api.md")
	mustWriteFile(t, rule, "---\nname: api\nscope: services/api\n---\napi body\n")
	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	if !fileExists(unledgeredScopedDoc) {
		t.Fatalf("sync did not write %s", unledgeredScopedDoc)
	}
	mustWriteFile(t, unledgeredToolFile, header.With("name = \"old\"\n", header.FormatTOML))
	mustWriteFile(t, headerlessToolFile, "name = \"hand\"\n")
	mustWriteFile(t, mentionScopedDoc, "# Web\n\n"+header.Line(header.FormatMarkdown))
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	if err := os.Remove(rule); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(stateFilePath(".")); err != nil {
		t.Fatal(err)
	}
}

func assertOnDisk(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if !fileExists(p) {
			t.Errorf("%s was deleted", p)
		}
	}
}

func assertNamesLeftovers(t *testing.T, out string) {
	t.Helper()
	for _, want := range []string{
		filepath.ToSlash(unledgeredToolFile), "agnostic-ai doctor --fix",
		filepath.ToSlash(unledgeredScopedDoc), "by hand if stale",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	for _, not := range []string{filepath.ToSlash(headerlessToolFile), filepath.ToSlash(mentionScopedDoc)} {
		if strings.Contains(out, not) {
			t.Errorf("output names %s, which does not open with the header:\n%s", not, out)
		}
	}
}

// Without a ledger, sync keeps the leftovers, names each with how to
// remove it, and records them, so a later check and sync still see them.
func TestSync_WithoutLedgerReportsLeftoversAndKeepsThem(t *testing.T) {
	lostLedgerProject(t)
	out := captureLog(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	assertOnDisk(t, unledgeredToolFile, unledgeredScopedDoc, headerlessToolFile, mentionScopedDoc)
	assertNamesLeftovers(t, out.String())

	out.Reset()
	checkOut, err := runCLI(t, "sync", "--check")
	if err == nil {
		t.Error("sync --check passed after sync without a ledger kept the leftovers")
	}
	assertNamesLeftovers(t, out.String()+checkOut)

	out.Reset()
	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	assertOnDisk(t, unledgeredToolFile, unledgeredScopedDoc)
	assertNamesLeftovers(t, out.String())
}

// After sync records the leftovers, doctor --fix removes the tool
// directory one and leaves the scope document for the user.
func TestDoctorFix_AfterSyncWithoutLedgerRemovesToolDirLeftover(t *testing.T) {
	lostLedgerProject(t)
	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	_, _ = runCLI(t, "doctor", "--fix")

	if fileExists(unledgeredToolFile) {
		t.Errorf("doctor --fix kept %s", unledgeredToolFile)
	}
	assertOnDisk(t, unledgeredScopedDoc, headerlessToolFile, mentionScopedDoc)
	reports, err := collectDrift(nil)
	if err != nil {
		t.Fatal(err)
	}
	var leftover, orphaned []string
	for _, r := range reports {
		leftover = append(leftover, r.Leftover...)
		orphaned = append(orphaned, r.Orphaned...)
	}
	if len(leftover) != 0 || !slices.Equal(orphaned, []string{unledgeredScopedDoc}) {
		t.Errorf("after doctor --fix: leftover %v, orphaned %v; want only %s for manual removal", leftover, orphaned, unledgeredScopedDoc)
	}
}

// A recorded leftover whose header was removed since is no longer proven
// generated: neither check nor doctor --fix treat it as one.
func TestDoctorFix_AfterSyncWithoutLedgerKeepsRecordedFileWithoutHeader(t *testing.T) {
	lostLedgerProject(t)
	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, unledgeredToolFile, "name = \"old\"\n")

	_, _ = runCLI(t, "doctor", "--fix")

	assertOnDisk(t, unledgeredToolFile)
	reports, err := collectDrift(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range reports {
		if slices.Contains(r.Leftover, unledgeredToolFile) {
			t.Errorf("%s reported as a leftover without its header", unledgeredToolFile)
		}
	}
}

type pathAction struct{ target, path, action string }

func pathActions(records []fileRecord, actions ...string) []pathAction {
	var out []pathAction
	for _, r := range records {
		if slices.Contains(actions, r.Action) {
			out = append(out, pathAction{r.Target, filepath.ToSlash(r.Path), r.Action})
		}
	}
	slices.SortFunc(out, func(a, b pathAction) int { return strings.Compare(a.path, b.path) })
	return out
}

// The previews list the leftovers the way sync then reports them: kept,
// never as a delete sync does not make.
func TestSyncPreviews_WithoutLedgerAgreeWithSync(t *testing.T) {
	lostLedgerProject(t)
	want := []pathAction{
		{unledgeredReportTarget, filepath.ToSlash(unledgeredToolFile), "leftover"},
		{unledgeredReportTarget, filepath.ToSlash(unledgeredScopedDoc), "orphan"},
	}

	for _, args := range [][]string{{"--plan", "--json"}, {"--dry-run", "--json"}, {"--json"}, {"--plan", "--json"}} {
		out := runSyncJSONArgs(t, args...)
		if got := pathActions(out.Writes, "delete"); len(got) != 0 {
			t.Errorf("sync %v lists deletes %v", args, got)
		}
		if got := pathActions(out.Skipped, "leftover", "orphan"); !slices.Equal(got, want) {
			t.Errorf("sync %v skipped %v, want %v", args, got, want)
		}
	}
	plan, err := runCLI(t, "sync", "--plan")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plan, "removed:") || !strings.Contains(plan, "leftover: 1") {
		t.Errorf("sync --plan does not show the kept leftover:\n%s", plan)
	}
}

// A run that skips a configured target, with no ledger to carry forward,
// still records every leftover candidate, so a later check and full sync
// report them. A text and a JSON sync both do.
func TestSync_PartialRunWithoutLedgerRecordsLeftovers(t *testing.T) {
	for name, partialSync := range map[string]func(t *testing.T){
		"text": func(t *testing.T) {
			if err := runSyncOnce(".", []string{"codex"}, false, false, "off", 1); err != nil {
				t.Fatal(err)
			}
		},
		"json": func(t *testing.T) { runSyncJSONArgs(t, "--json", "--only", "codex") },
	} {
		t.Run(name, func(t *testing.T) {
			lostLedgerProjectFor(t, "codex, claude")

			partialSync(t)

			recorded := readStateFile(".").Unledgered
			for _, p := range []string{unledgeredToolFile, unledgeredScopedDoc} {
				if !slices.Contains(recorded, p) {
					t.Errorf("partial sync did not record %s: %v", p, recorded)
				}
			}
			reports, err := collectDrift(nil)
			if err != nil {
				t.Fatal(err)
			}
			merged := map[string]bool{}
			for _, r := range reports {
				for _, p := range append(append([]string{}, r.Leftover...), r.Orphaned...) {
					merged[p] = true
				}
			}
			if !merged[unledgeredToolFile] || !merged[unledgeredScopedDoc] {
				t.Errorf("check after a partial sync lost the leftovers: %+v", reports)
			}
			if merged["CLAUDE.md"] || merged[headerlessToolFile] || merged[mentionScopedDoc] {
				t.Errorf("check reports a file that is not a leftover: %+v", reports)
			}
			out := captureLog(t)
			if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
				t.Fatal(err)
			}
			assertNamesLeftovers(t, out.String())
			assertOnDisk(t, unledgeredToolFile, unledgeredScopedDoc, headerlessToolFile, mentionScopedDoc)
		})
	}
}

// The ledger report holds what the sweep removes and the unledgered one
// what sync keeps, so each prints and serializes under its own target.
func TestCollectDrift_UnledgeredReportHasItsOwnTarget(t *testing.T) {
	lostLedgerProject(t)

	reports, err := collectDrift(nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, r := range reports {
		if r.Unledgered && r.Target != unledgeredReportTarget || r.Target == ledgerReport && r.Unledgered {
			t.Errorf("unledgered drift under target %q: %+v", r.Target, r)
		}
	}
}

// A run that skips a configured target cannot rescan, so it keeps the
// recorded leftovers it did not write again.
func TestKeepUnledgered_PartialRunCarriesRecordForward(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, stateFilePath("."), "{\"version\":5,\"synced_at\":\"2026-01-01T00:00:00Z\"}\n")
	prev := syncStateFile{Unledgered: []string{unledgeredToolFile, unledgeredScopedDoc}}
	var ledger syncLedger

	rep := keepUnledgered(&config.Config{Targets: []string{"codex"}}, prev, &ledger, map[string]string{unledgeredScopedDoc: ""}, false)

	if rep.hasDrift() || !slices.Equal(ledger.unledgered, []string{unledgeredToolFile}) {
		t.Errorf("got report %+v and record %v, want record [%s]", rep, ledger.unledgered, unledgeredToolFile)
	}
}
