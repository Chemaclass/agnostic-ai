package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// setRunningVersion stands in for the version Go stamps into a build.
func setRunningVersion(t *testing.T, version string) {
	t.Helper()
	prev := runningVersion
	runningVersion = version
	t.Cleanup(func() { runningVersion = prev })
}

func runAsVersion(t *testing.T, version string, args ...string) (string, string, error) {
	t.Helper()
	setRunningVersion(t, version)
	var out, errOut bytes.Buffer
	cmd := NewRootCmd("test")
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// requiresProject writes a claude-only project with one rule whose
// config sets requires.
func requiresProject(t *testing.T, requires string) {
	t.Helper()
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	dir := testutil.TempCwd(t)
	setRequires(t, requires)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "safe.md"), "---\nname: safe\n---\nBe safe.\n")
}

func setRequires(t *testing.T, requires string) {
	t.Helper()
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nrequires: \""+requires+"\"\n")
}

func assertUnmetRequires(t *testing.T, err error, config, required, installed string) {
	t.Helper()
	if errs.CodeOf(err) != errs.CodeRequiresUnmet {
		t.Fatalf("want %s, got %v", errs.CodeRequiresUnmet, err)
	}
	for _, want := range []string{config + " requires agnostic-ai " + required, "but " + installed + " is installed", "`agnostic-ai upgrade"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message misses %q: %v", want, err)
		}
	}
}

func assertAbsent(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("an unmet requires left %s written: %v", p, err)
		}
	}
}

func TestBuildVersion_OnlyATaggedBuildIsARelease(t *testing.T) {
	t.Parallel()
	req, err := config.ParseRequirement(">=0.69.0")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		info    *debug.BuildInfo
		ok      bool
		release bool
	}{
		{"no build info", nil, false, false},
		{"go run", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, true, false},
		{"tagged release", &debug.BuildInfo{Main: debug.Module{Version: "v0.69.0"}}, true, true},
		{"tagged release the tidy hook dirtied", &debug.BuildInfo{Main: debug.Module{Version: "v0.69.0+dirty"}}, true, true},
		{"commit after a tag", &debug.BuildInfo{Main: debug.Module{Version: "v0.69.1-0.20260927082917-5d5f7ecd4f49"}}, true, false},
		{"dirty checkout", &debug.BuildInfo{Main: debug.Module{Version: "v0.69.1-0.20260927082917-5d5f7ecd4f49+dirty"}}, true, false},
	}
	for _, c := range cases {
		version := buildVersion(c.info, c.ok)
		if _, release := req.Allows(version); release != c.release {
			t.Errorf("%s: %q release = %v, want %v", c.name, version, release, c.release)
		}
		if c.release && version != "v0.69.0" {
			t.Errorf("%s: version = %q, want v0.69.0", c.name, version)
		}
	}
}

func TestProjectCommands_StopWhenInstalledVersionIsBelowRequires(t *testing.T) {
	requiresProject(t, ">=0.70.0")

	for _, args := range [][]string{{"sync"}, {"sync", "--check"}, {"lint"}, {"validate"}, {"doctor", "--fix"}, {"cleanup"}} {
		_, _, err := runAsVersion(t, "v0.69.0", args...)
		assertUnmetRequires(t, err, "agnostic-ai.yaml", ">=0.70.0", "0.69.0")
	}
	assertAbsent(t, "CLAUDE.md", "AGENTS.md", ".claude", ".agnostic-ai/.sync-state")
}

func TestProjectCommands_ExactRequiresStopsOlderAndNewerBinaries(t *testing.T) {
	for _, pin := range []string{"0.73.0", "=0.73.0"} {
		requiresProject(t, pin)
		silence(t)

		for _, args := range [][]string{{"sync"}, {"sync", "--check"}, {"lint"}, {"validate"}, {"doctor", "--fix"}, {"cleanup"}} {
			for _, installed := range []string{"v0.72.0", "v0.74.0"} {
				_, _, err := runAsVersion(t, installed, args...)
				assertUnmetRequires(t, err, "agnostic-ai.yaml", "0.73.0", strings.TrimPrefix(installed, "v"))
				if !strings.Contains(err.Error(), "`agnostic-ai upgrade --version v0.73.0`") {
					t.Errorf("%s on %s: the message must name the pinned release to install: %v", pin, installed, err)
				}
			}
		}
		assertAbsent(t, "CLAUDE.md", "AGENTS.md", ".claude", ".agnostic-ai/.sync-state")

		if _, _, err := runAsVersion(t, "v0.73.0", "sync"); err != nil {
			t.Fatalf("%s: sync on 0.73.0: %v", pin, err)
		}
		if _, _, err := runAsVersion(t, "v0.73.0", "sync", "--check"); err != nil {
			t.Errorf("%s: sync --check on 0.73.0: %v", pin, err)
		}
	}
}

func TestProjectCommands_RangeRequiresStopsBinariesOutsideIt(t *testing.T) {
	requiresProject(t, ">=0.73.0 <0.74.0")
	silence(t)

	for _, installed := range []string{"v0.72.9", "v0.74.0"} {
		_, _, err := runAsVersion(t, installed, "sync")
		assertUnmetRequires(t, err, "agnostic-ai.yaml", ">=0.73.0 <0.74.0", strings.TrimPrefix(installed, "v"))
	}
	for _, installed := range []string{"v0.73.0", "v0.73.4"} {
		if _, _, err := runAsVersion(t, installed, "sync"); err != nil {
			t.Errorf("sync on %s: %v", installed, err)
		}
	}
}

func TestProjectCommands_MinimumRequiresKeepsItsOldMessage(t *testing.T) {
	requiresProject(t, ">=0.70.0")

	_, _, err := runAsVersion(t, "v0.69.0", "sync")
	want := "agnostic-ai.yaml requires agnostic-ai >=0.70.0, but 0.69.0 is installed; run `agnostic-ai upgrade`"
	if err == nil || !strings.HasSuffix(err.Error(), want) {
		t.Errorf("want the message to end %q, got %v", want, err)
	}
}

func TestProjectCommands_UpperBoundOnlyRequiresAsksForARelease(t *testing.T) {
	requiresProject(t, "<0.74.0")

	_, _, err := runAsVersion(t, "v0.74.0", "sync")
	assertUnmetRequires(t, err, "agnostic-ai.yaml", "<0.74.0", "0.74.0")
	if !strings.Contains(err.Error(), "`agnostic-ai upgrade --version vX.Y.Z`") {
		t.Errorf("the message must show how to install a release inside the range: %v", err)
	}
}

func TestSync_ExactRequiresSkipsSourceBuilds(t *testing.T) {
	requiresProject(t, "0.73.0")
	silence(t)

	_, errOut, err := runAsVersion(t, "v0.73.1-0.20260927082917-5d5f7ecd4f49", "sync")
	if err != nil {
		t.Fatalf("a source build must not be blocked: %v", err)
	}
	if !strings.Contains(errOut, "requires 0.73.0, but 0.73.1-0.20260927082917-5d5f7ecd4f49 is not a release build; not checked") {
		t.Errorf("want the not-a-release warning, got:\n%s", errOut)
	}
}

func TestRevert_StopsWhenInstalledVersionIsBelowRequires(t *testing.T) {
	requiresProject(t, ">=0.70.0")
	silence(t)
	if _, _, err := runAsVersion(t, "v0.70.0", "sync"); err != nil {
		t.Fatalf("sync on 0.70.0: %v", err)
	}
	setRequires(t, ">=0.71.0")

	_, _, err := runAsVersion(t, "v0.70.0", "revert", "--force")
	assertUnmetRequires(t, err, "agnostic-ai.yaml", ">=0.71.0", "0.70.0")
	if _, err := os.Stat(filepath.Join(".claude", "rules", "safe.md")); err != nil {
		t.Errorf("revert removed output despite the unmet requires: %v", err)
	}
}

func TestWatchResync_StopsOnceRequiresIsRaised(t *testing.T) {
	requiresProject(t, ">=0.69.0")
	silence(t)
	captureLog(t)
	setRunningVersion(t, "v0.69.0")
	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	emitted := filepath.Join(".claude", "rules", "safe.md")
	before := readFile(t, emitted)

	setRequires(t, ">=0.70.0")
	rule := filepath.Join(".agnostic-ai", "rules", "safe.md")
	mustWriteFile(t, rule, "---\nname: safe\n---\nBe very safe.\n")

	err := resyncForChanges(".", nil, []string{rule}, false, false, "off", 1)
	assertUnmetRequires(t, err, "agnostic-ai.yaml", ">=0.70.0", "0.69.0")
	if got := readFile(t, emitted); got != before {
		t.Errorf("watch re-synced past an unmet requires:\n%s", got)
	}
}

func TestSync_RunsWhenInstalledVersionMeetsRequires(t *testing.T) {
	requiresProject(t, ">=0.70.0")
	silence(t)

	if _, _, err := runAsVersion(t, "v0.70.0", "sync"); err != nil {
		t.Fatalf("sync on 0.70.0: %v", err)
	}
	if _, err := os.Stat(filepath.Join(".claude", "rules", "safe.md")); err != nil {
		t.Errorf("expected the rule written: %v", err)
	}
	if _, _, err := runAsVersion(t, "v0.70.1", "sync", "--check"); err != nil {
		t.Errorf("sync --check on 0.70.1: %v", err)
	}
}

func TestSync_SourceBuildWarnsOnceAndSkipsRequires(t *testing.T) {
	requiresProject(t, ">=0.70.0")
	silence(t)

	_, errOut, err := runAsVersion(t, "v0.69.1-0.20260927082917-5d5f7ecd4f49+dirty", "sync")
	if err != nil {
		t.Fatalf("a source build must not be blocked: %v", err)
	}
	want := "warning: agnostic-ai.yaml: requires >=0.70.0, but 0.69.1-0.20260927082917-5d5f7ecd4f49+dirty is not a release build; not checked\n"
	if n := strings.Count(errOut, want); n != 1 {
		t.Errorf("want %q once, got %d in:\n%s", want, n, errOut)
	}
}

func TestSync_InvalidRequiresNamesFileAndKey(t *testing.T) {
	requiresProject(t, "0.70")

	_, _, err := runAsVersion(t, "v0.69.0", "sync")
	if errs.CodeOf(err) != errs.CodeConfigDecode || !strings.Contains(err.Error(), "agnostic-ai.yaml: requires:") {
		t.Errorf("want AAI-004 naming agnostic-ai.yaml and requires, got %v", err)
	}
}

func TestGlobalCommands_StopWhenInstalledVersionIsBelowRequires(t *testing.T) {
	home, source := globalConfigTestHome(t)
	config := filepath.Join(source, "agnostic-ai.yaml")
	mustWriteGlobalTest(t, config, "requires: \">=0.70.0\"\ntargets: [claude]\n")

	for _, args := range [][]string{{"sync"}, {"sync", "--check"}, {"sync", "-t", "claude"}, {"lint"}, {"validate"}, {"list"}} {
		_, _, err := runAsVersion(t, "v0.69.0", append(args, "--global")...)
		assertUnmetRequires(t, err, config, ">=0.70.0", "0.69.0")
	}
	assertGlobalInstructions(t, home, map[string]bool{".claude/CLAUDE.md": false})

	_, errOut, err := runAsVersion(t, "v0.70.0", "sync", "--global")
	if err != nil {
		t.Fatalf("sync --global on 0.70.0: %v", err)
	}
	if strings.Contains(errOut, "ignoring") {
		t.Errorf("the home config must accept requires, got:\n%s", errOut)
	}
	assertGlobalInstructions(t, home, map[string]bool{".claude/CLAUDE.md": true})
}

func TestGlobalSync_LocalHomeConfigRequiresReplacesShared(t *testing.T) {
	_, source := globalConfigTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "requires: \">=0.60.0\"\n")
	local := filepath.Join(source, "local", "agnostic-ai.yaml")
	mustWriteGlobalTest(t, local, "requires: \">=0.70.0\"\n")

	_, _, err := runAsVersion(t, "v0.69.0", "sync", "--global")
	assertUnmetRequires(t, err, local, ">=0.70.0", "0.69.0")
}

func TestGlobalSync_NullLocalRequiresClearsShared(t *testing.T) {
	home, source := globalConfigTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "requires: \">=0.70.0\"\ntargets: [claude]\n")
	mustWriteGlobalTest(t, filepath.Join(source, "local", "agnostic-ai.yaml"), "requires:\n")

	if _, _, err := runAsVersion(t, "v0.69.0", "sync", "--global"); err != nil {
		t.Fatalf("a null local requires must clear the shared one, as in a project: %v", err)
	}
	assertGlobalInstructions(t, home, map[string]bool{".claude/CLAUDE.md": true})
}

func TestGlobalSync_TargetFlagWarnsOnUnparsableHomeConfig(t *testing.T) {
	home, source := globalConfigTestHome(t)
	config := filepath.Join(source, "agnostic-ai.yaml")
	mustWriteGlobalTest(t, config, "targets: [claude\n")

	_, errOut, err := runAsVersion(t, "v0.69.0", "sync", "--global", "-t", "claude")
	if err != nil {
		t.Fatalf("-t must not be blocked by a home config it bypasses: %v", err)
	}
	if !strings.Contains(errOut, "warning: [AAI-004] parse "+config) || !strings.Contains(errOut, "; skipping it") {
		t.Errorf("expected a warning naming %s, got:\n%s", config, errOut)
	}
	assertGlobalInstructions(t, home, map[string]bool{".claude/CLAUDE.md": true})

	mustWriteGlobalTest(t, config, "requires: \">=0.70.0\"\n")
	mustWriteGlobalTest(t, filepath.Join(source, "local", "agnostic-ai.yaml"), "targets: [claude\n")
	_, _, err = runAsVersion(t, "v0.69.0", "sync", "--global", "-t", "claude")
	assertUnmetRequires(t, err, config, ">=0.70.0", "0.69.0")
}

func TestGlobalSync_InvalidRequiresNamesFileAndKey(t *testing.T) {
	_, source := globalConfigTestHome(t)
	config := filepath.Join(source, "agnostic-ai.yaml")
	mustWriteGlobalTest(t, config, "requires: [\">=0.70.0\"]\n")

	_, _, err := runAsVersion(t, "v0.69.0", "sync", "--global")
	if errs.CodeOf(err) != errs.CodeConfigDecode || !strings.Contains(err.Error(), config+": requires:") {
		t.Errorf("want AAI-004 naming %s and requires, got %v", config, err)
	}
}

func TestDoctor_NextStepGivesTheConfigErrorFix(t *testing.T) {
	requiresProject(t, ">=0.70.0")

	out, _, err := runAsVersion(t, "v0.69.0", "doctor")
	if errs.CodeOf(err) != errs.CodeRequiresUnmet {
		t.Fatalf("want %s, got %v", errs.CodeRequiresUnmet, err)
	}
	entry, _ := errs.Lookup(errs.CodeRequiresUnmet)
	if !strings.Contains(out, "Next step:\n  "+entry.Fix+"\n") || strings.Contains(out, "agnostic-ai init") {
		t.Errorf("want the AAI-005 fix as the next step, got:\n%s", out)
	}

	testutil.TempCwd(t)
	out, _, _ = runAsVersion(t, "v0.69.0", "doctor")
	if !strings.Contains(out, "Next step:\n  No agnostic-ai.yaml found. Run: agnostic-ai init\n") {
		t.Errorf("a missing config must still point at init, got:\n%s", out)
	}
}

func TestListGlobal_WarnsOnUnparsableHomeConfig(t *testing.T) {
	_, source := globalConfigTestHome(t)
	config := filepath.Join(source, "agnostic-ai.yaml")
	mustWriteGlobalTest(t, config, "targets: [claude\n")
	mustWriteGlobalTest(t, filepath.Join(source, "rules", "safe.md"), "---\nname: safe\n---\nBe safe.\n")

	out, errOut, err := runAsVersion(t, "v0.69.0", "list", "--global")
	if err != nil {
		t.Fatalf("list --global reads no targets, so a broken home config must not stop it: %v", err)
	}
	if !strings.Contains(errOut, "warning: [AAI-004] parse "+config) {
		t.Errorf("expected a warning naming %s, got:\n%s", config, errOut)
	}
	if !strings.Contains(out, "rule\tsafe\tglobal") {
		t.Errorf("expected the global rule listed, got:\n%s", out)
	}
}
