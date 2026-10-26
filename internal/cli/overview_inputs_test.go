package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/overview"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// writeBareReadProject writes a claude project whose only spec allows
// the bare read capability, which LINT038 reports.
func writeBareReadProject(t *testing.T, dir string) {
	t.Helper()
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "settings", "permissions.yaml"), "permissions:\n  allow: [read]\n")
}

func driftByPath(t *testing.T, findings []overview.Finding) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, f := range findings {
		if f.Source != overview.SourceDrift {
			continue
		}
		if f.Target != "claude" {
			t.Errorf("drift finding for %q, want claude: %+v", f.Target, f)
		}
		out[f.Path] = f.Message
	}
	return out
}

func TestRunOverviewChecks_ReportsTheBareReadRuleAndNotGeneratedDrift(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeBareReadProject(t, dir)
	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}

	findings := runOverviewChecks(".", cfg, b).findings

	var bare []overview.Finding
	for _, f := range findings {
		if f.Source == overview.SourceLint && f.Code == "LINT038" {
			bare = append(bare, f)
		}
	}
	if len(bare) != 1 {
		t.Fatalf("LINT038 findings = %+v, all = %+v", bare, findings)
	}
	if bare[0].Path != ".agnostic-ai/settings/permissions.yaml" || !strings.Contains(bare[0].Message, "bare read") || bare[0].Severity != "warn" {
		t.Errorf("LINT038 finding = %+v", bare[0])
	}
	if bare[0].Explanation != lintCodes["LINT038"].Cause {
		t.Errorf("explanation = %q, want the explain cause", bare[0].Explanation)
	}
	if bare[0].Fix != lintCodes["LINT038"].Fix {
		t.Errorf("fix = %q, want the explain fix", bare[0].Fix)
	}
	drift := driftByPath(t, findings)
	for _, path := range []string{".claude/settings.json", "CLAUDE.md"} {
		if drift[path] != driftNotGenerated {
			t.Errorf("drift[%s] = %q, want %q (all drift: %v)", path, drift[path], driftNotGenerated, drift)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude")); !os.IsNotExist(err) {
		t.Errorf("collecting findings wrote .claude: %v", err)
	}
}

func TestRunOverviewChecks_LeavesNoWarningOrNoteBuffered(t *testing.T) {
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	// Claude Code reads no review spec, so rendering it raises a
	// capability warning.
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "reviews", "checklist.md"), "---\nname: checklist\ndescription: Check the diff.\n---\n\nCheck the diff against the intent.\n")
	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	buffered := adapters.PendingCapabilityWarningsCount() + adapters.PendingCoverageNotesCount()

	runOverviewChecks(".", cfg, b)

	if n := adapters.PendingCapabilityWarningsCount() + adapters.PendingCoverageNotesCount(); n != buffered {
		t.Errorf("collecting findings changed the buffered warnings and notes from %d to %d", buffered, n)
	}
}

func TestRunOverviewChecks_ReportsAHandEditedFileAsStale(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	captureLogOut(t)
	writeBareReadProject(t, dir)
	if err := runSyncOnce(dir, nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	if drift := driftByPath(t, runOverviewChecks(".", cfg, b).findings); len(drift) != 0 {
		t.Fatalf("drift right after sync = %v", drift)
	}
	mustWrite(t, filepath.Join(dir, ".claude", "settings.json"), "{}\n")

	drift := driftByPath(t, runOverviewChecks(".", cfg, b).findings)
	if len(drift) != 1 || drift[".claude/settings.json"] != driftStale {
		t.Errorf("drift = %v, want only .claude/settings.json stale", drift)
	}
}

func TestOverviewInputs_BuildsTheSnapshotFromTheProject(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeBareReadProject(t, dir)

	in, err := overviewInputs(".")
	if err != nil {
		t.Fatal(err)
	}
	snap := in.Snapshot
	if !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(snap.Revision) {
		t.Errorf("revision = %q, want 12 hex digits", snap.Revision)
	}
	if snap.Project.Name != filepath.Base(dir) || snap.Project.Root != "." {
		t.Errorf("project = %+v", snap.Project)
	}
	if !slices.Equal(snap.Targets, []string{"claude"}) || !slices.Equal(snap.ConfigFiles, []string{"agnostic-ai.yaml"}) {
		t.Errorf("targets = %v, config files = %v", snap.Targets, snap.ConfigFiles)
	}
	if len(snap.Specs) != 1 || snap.Specs[0].Path != ".agnostic-ai/settings/permissions.yaml" || snap.Specs[0].Kind != "settings" || snap.Specs[0].Layer != "project" {
		t.Errorf("specs = %+v", snap.Specs)
	}
	if snap.Evidence.Declared != "loaded" || snap.Evidence.Installed != "checked" || snap.Evidence.Runtime != "not-observed" || !strings.Contains(snap.Evidence.Note, "claude") {
		t.Errorf("evidence = %+v", snap.Evidence)
	}
	if in.Cfg == nil || len(in.Bundle.Settings) != 1 || len(in.Findings) == 0 {
		t.Errorf("inputs = %+v", in)
	}
	if !slices.Equal(in.Compared, []string{"claude"}) {
		t.Errorf("compared = %v, want claude", in.Compared)
	}

	again, err := overviewInputs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if again.Snapshot.Revision != snap.Revision {
		t.Errorf("revision changed with nothing edited: %s, %s", snap.Revision, again.Snapshot.Revision)
	}
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "settings", "permissions.yaml"), "permissions:\n  allow: [read(src/**)]\n")
	edited, err := overviewInputs(".")
	if err != nil {
		t.Fatal(err)
	}
	if edited.Snapshot.Revision == snap.Revision {
		t.Errorf("revision %s survived a spec edit", snap.Revision)
	}
}

func TestOverviewInputs_RefusesARootOtherThanTheWorkingDirectory(t *testing.T) {
	testutil.TempCwd(t)
	other := t.TempDir()
	writeBareReadProject(t, other)
	if _, err := overviewInputs(other); err == nil || !strings.Contains(err.Error(), "not the working directory") {
		t.Errorf("err = %v, want a refusal naming the working directory", err)
	}
}

func TestNewOverviewHandler_ServesTheProjectInTheWorkingDirectory(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeBareReadProject(t, dir)
	h := newOverviewHandler(".", true)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+overview.OverviewPath, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var view overview.View
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if got := itemWithFinding(view, "LINT038"); got != "permissions" {
		t.Errorf("LINT038 sits on %q, want the permissions settings item", got)
	}
	for _, section := range view.Sections {
		for _, item := range section.Items {
			for _, finding := range item.Findings {
				if finding.Code == "LINT038" && finding.Fix != lintCodes["LINT038"].Fix {
					t.Errorf("served fix = %q, want the explain fix", finding.Fix)
				}
			}
		}
	}
	if len(view.Targets) != 1 || view.Targets[0].Sync.State != overview.SyncDrifted || len(view.Targets[0].Sync.Files) == 0 {
		t.Errorf("targets = %+v, want claude drifted: nothing is generated yet", view.Targets)
	}
}

// itemWithFinding names the first item that carries a finding with
// code, or "" when none does.
func itemWithFinding(v overview.View, code string) string {
	for _, s := range v.Sections {
		for _, it := range s.Items {
			for _, f := range it.Findings {
				if f.Code == code {
					return it.Name
				}
			}
		}
	}
	return ""
}

func TestCompareInstalled_PresentBesideMissingIsNotInSync(t *testing.T) {
	testutil.TempCwd(t)
	mustWrite(t, "a.txt", "same\n")
	state, err := compareInstalled([]adapters.CapturedFile{{Path: "a.txt", Content: "same\n"}, {Path: "b.txt", Content: "missing\n"}})
	if err != nil {
		t.Fatal(err)
	}
	if state.State != authority.InstalledStale {
		t.Errorf("state = %s, want stale: one generated file is missing", state.State)
	}
	want := []authority.InstalledFile{{Path: "a.txt", State: authority.InstalledInSync}, {Path: "b.txt", State: authority.InstalledNotGenerated}}
	if !reflect.DeepEqual(state.Files, want) {
		t.Errorf("files = %+v, want %+v", state.Files, want)
	}
}

func TestRunOverviewChecks_GivesAFindingOnAFileItsAbsolutePath(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeBareReadProject(t, dir)
	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}

	findings := runOverviewChecks(".", cfg, b).findings

	want, err := filepath.Abs(filepath.Join(".agnostic-ai", "settings", "permissions.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		if f.Code == "LINT038" && f.OpenPath != filepath.ToSlash(want) {
			t.Errorf("LINT038 open path = %q, want %q", f.OpenPath, filepath.ToSlash(want))
		}
		if f.Source == overview.SourceDrift && f.OpenPath != "" {
			t.Errorf("drift finding %s has open path %q, want none", f.Path, f.OpenPath)
		}
	}
}

func TestFindingOpenPath_NamesOnlyAFileOnDiskInsideTheConfiguration(t *testing.T) {
	base := t.TempDir()
	mustWrite(t, filepath.Join(base, "rules with spaces", "ågent#1.md"), "x\n")
	outside := filepath.Join(t.TempDir(), "outside.md")
	mustWrite(t, outside, "x\n")

	for _, c := range []struct{ path, want string }{
		{"rules with spaces/ågent#1.md", filepath.ToSlash(filepath.Join(base, "rules with spaces", "ågent#1.md"))},
		{filepath.ToSlash(filepath.Join(base, "rules with spaces", "ågent#1.md")), filepath.ToSlash(filepath.Join(base, "rules with spaces", "ågent#1.md"))},
		{filepath.ToSlash(outside), ""},
		{"rules with spaces", ""},
		{"missing.md", ""},
		{"", ""},
	} {
		if got := findingOpenPath(base, c.path); got != c.want {
			t.Errorf("findingOpenPath(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}
