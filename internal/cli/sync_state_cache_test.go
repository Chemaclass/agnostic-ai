package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestHoldStateFile_ServesOneCopyUntilReleased(t *testing.T) {
	dir := testutil.TempCwd(t)
	path := filepath.Join(dir, ".agnostic-ai", ".sync-state")
	mustWriteFile(t, path, `{"output_sums":{"a.md":"one"}}`)

	release := holdStateFile(".")
	mustWriteFile(t, path, `{"output_sums":{"a.md":"two"}}`)
	if got := priorStateFile().OutputSums["a.md"]; got != "one" {
		t.Errorf("while held: sum %q, want the held one", got)
	}
	release()
	if got := priorStateFile().OutputSums["a.md"]; got != "two" {
		t.Errorf("after release: sum %q, want two from disk", got)
	}
}

func TestHoldPriorState_ServesTheGivenCopy(t *testing.T) {
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", ".sync-state"), `{"output_sums":{"a.md":"disk"}}`)

	release := holdPriorState(".", syncStateFile{OutputSums: map[string]string{"a.md": "held"}})
	if got := priorStateFile().OutputSums["a.md"]; got != "held" {
		t.Errorf("while held: sum %q, want held", got)
	}
	release()
	if got := priorStateFile().OutputSums["a.md"]; got != "disk" {
		t.Errorf("after release: sum %q, want disk", got)
	}
}

func TestHoldPriorState_OtherRootHoldsNothing(t *testing.T) {
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", ".sync-state"), `{"output_sums":{"a.md":"disk"}}`)

	release := holdPriorState(t.TempDir(), syncStateFile{OutputSums: map[string]string{"a.md": "other"}})
	defer release()
	if got := priorStateFile().OutputSums["a.md"]; got != "disk" {
		t.Errorf("sum %q, want disk: the hooks read the working directory", got)
	}
}

func TestRunSyncOnce_ReleasesTheHeldState(t *testing.T) {
	setupUnmanagedFixture(t)
	silence(t)
	captureLog(t)
	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	mustWriteFile(t, filepath.Join(".agnostic-ai", ".sync-state"), `{"output_sums":{"a.md":"after"}}`)
	if got := priorStateFile().OutputSums["a.md"]; got != "after" {
		t.Errorf("sum %q, want after: sync must not hold the state once it returns", got)
	}
}

func TestPriorStateFile_ReadsTheDiskWhenNothingIsHeld(t *testing.T) {
	dir := testutil.TempCwd(t)
	path := filepath.Join(dir, ".agnostic-ai", ".sync-state")
	mustWriteFile(t, path, `{"output_sums":{"a.md":"one"}}`)
	if got := priorStateFile().OutputSums["a.md"]; got != "one" {
		t.Fatalf("first read: sum %q, want one", got)
	}
	mustWriteFile(t, path, `{"output_sums":{"a.md":"two"}}`)
	if got := priorStateFile().OutputSums["a.md"]; got != "two" {
		t.Errorf("same-size rewrite: sum %q, want two", got)
	}
}

func TestHoldState_NestedOwnersReleaseOnlyTheirOwnFrames(t *testing.T) {
	testutil.TempCwd(t)
	state := func(value string) syncStateFile { return syncStateFile{OutputSums: map[string]string{"probe": value}} }
	outer := holdState(state("outer"))
	defer outer()
	inner := holdState(state("inner"))
	defer inner()
	outer()
	if got := priorStateFile().OutputSums["probe"]; got != "inner" {
		t.Errorf("outer release cleared child: %q", got)
	}
	inner()
	next := holdState(state("next"))
	defer next()
	outer()
	inner()
	if got := priorStateFile().OutputSums["probe"]; got != "next" {
		t.Errorf("old release cleared new owner: %q", got)
	}
	next()
	outer = holdState(state("outer"))
	inner = holdState(state("inner"))
	inner()
	if got := priorStateFile().OutputSums["probe"]; got != "outer" {
		t.Errorf("inner release lost parent: %q", got)
	}
	outer()
}

func TestRenderRef_DoesNotBorrowLiveMergedLedger(t *testing.T) {
	dir, git := launchGitRepo(t)
	testutil.Chdir(t, dir)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\noutputs: {claude: {dir: .agnostic-ai/native}}\n")
	mustWriteFile(t, ".agnostic-ai/settings/model.yaml", "name: model\nmodel: sonnet\n")
	mustWriteFile(t, ".agnostic-ai/native/settings.json", "{\"statusLine\":\"old\"}\n")
	git("add", "-A")
	git("commit", "-q", "-m", "historical settings")
	render := func() (map[string]string, error) {
		return renderRef(dir, "", "HEAD", filepath.Join(t.TempDir(), "tree"), nil)
	}
	expected, err := render()
	if err != nil {
		t.Fatal(err)
	}
	state := syncStateFile{OutputSums: map[string]string{"probe": "outer"}, Merged: map[string]mergedOutput{".agnostic-ai/native/settings.json": {Keys: []adapters.MergedKey{{Path: []string{"statusLine"}, Sum: adapters.ContentSum(`"old"`)}}}}}
	release := holdState(state)
	defer release()
	got, err := render()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("live ledger changed historical bytes:\nwant %v\ngot %v", expected, got)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	physical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cwd != physical {
		t.Errorf("CWD=%s, want %s", cwd, physical)
	}
	if priorStateFile().OutputSums["probe"] != "outer" {
		t.Error("historical render lost live owner")
	}
	mustWriteFile(t, "agnostic-ai.yaml", "version: [\n")
	git("add", "-A")
	git("commit", "-q", "-m", "invalid settings")
	if _, err := render(); err == nil {
		t.Error("invalid historical settings did not fail")
	}
	if priorStateFile().OutputSums["probe"] != "outer" {
		t.Error("failed historical render lost live owner")
	}
	if after, err := os.Getwd(); err != nil || after != physical {
		t.Errorf("failed historical capture did not restore CWD: %s, %v", after, err)
	}
}

func TestHoldCaptureState_BorrowsOwnerAndIsolationMasksIt(t *testing.T) {
	testutil.TempCwd(t)
	mustWriteFile(t, stateFilePath("."), `{"output_sums":{"probe":"disk"}}`)
	outer := holdState(syncStateFile{OutputSums: map[string]string{"probe": "outer"}})
	defer outer()
	borrowed := holdCaptureState(readStateFile("."))
	borrowed()
	borrowed()
	if priorStateFile().OutputSums["probe"] != "outer" {
		t.Error("borrow released outer")
	}
	mask := isolatePriorState()
	defer mask()
	if priorStateFile().OutputSums["probe"] != "disk" {
		t.Error("mask leaked outer")
	}
	scratch := holdCaptureState(syncStateFile{OutputSums: map[string]string{"probe": "scratch"}})
	if priorStateFile().OutputSums["probe"] != "scratch" {
		t.Error("mask prevented scratch owner")
	}
	outer()
	scratch()
	mask()
	mask()
	if priorStateFile().OutputSums["probe"] != "disk" {
		t.Error("mask revived released outer")
	}
	mustRemove := os.Remove(stateFilePath("."))
	if mustRemove != nil {
		t.Fatal(mustRemove)
	}
	outer = holdState(syncStateFile{OutputSums: map[string]string{"probe": "outer"}})
	defer outer()
	mask = isolatePriorState()
	if len(priorStateFile().OutputSums) != 0 {
		t.Error("absent scratch ledger leaked outer")
	}
	mask()
	if priorStateFile().OutputSums["probe"] != "outer" {
		t.Error("mask lost active outer")
	}
}

func restorePriorMergedKeys() {
	adapters.SetPriorMergedKeys(func(path string) []adapters.MergedKey { return slices.Clone(priorStateFile().Merged[path].Keys) })
}

func TestCapturePhases_ReuseLoadedLedgerForActualCallbacks(t *testing.T) {
	for _, mode := range []string{"status", "drift", "partial-orphans"} {
		t.Run(mode, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			writeBenchProject(t, dir, 1)
			const orphan = ".trae/agents/agent-0000.md"
			mustWriteFile(t, orphan, "old native agent\n")
			ledger := func(value string) string {
				return `{"output_sums":{"probe":"` + value + `"},"orphans":["` + orphan + `"]}`
			}
			mustWriteFile(t, stateFilePath("."), ledger("one"))
			cfg, b, err := loadProject(".")
			if err != nil {
				t.Fatal(err)
			}
			var seen []string
			adapters.SetPriorMergedKeys(func(path string) []adapters.MergedKey {
				current := priorStateFile()
				seen = append(seen, current.OutputSums["probe"])
				if len(seen) == 1 {
					mustWriteFile(t, stateFilePath("."), ledger("two"))
				}
				return slices.Clone(current.Merged[path].Keys)
			})
			defer restorePriorMergedKeys()
			if mode == "status" {
				_, _, err = captureAllAndDiff(cfg.Targets, cfg, b)
			} else {
				targets := cfg.Targets
				if mode == "partial-orphans" {
					targets = []string{"claude"}
				}
				var reports []driftReport
				reports, err = collectLoadedDrift(cfg, b, targets, nil, "")
				if mode == "partial-orphans" {
					for _, rep := range reports {
						if slices.Contains(rep.Orphaned, orphan) {
							t.Error("unselected producer's current path remains orphaned")
						}
					}
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(seen) < 2 {
				t.Fatalf("callbacks=%d, want repeated actual captures", len(seen))
			}
			for _, value := range seen {
				if value != "one" {
					t.Errorf("capture reread changed ledger: %v", seen)
					break
				}
			}
			if priorStateFile().OutputSums["probe"] != "two" {
				t.Error("phase retained ledger after return")
			}
		})
	}
}

func TestCapturePhases_KeepNativeBodyReadsFresh(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [trae]\n")
	mustWriteFile(t, ".agnostic-ai/agents/reviewer.md", "---\nname: reviewer\n---\n\nbody one\n")
	syncProject(t)
	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	const native = ".trae/agents/reviewer.md"
	original := readFile(t, native)
	release := holdStateFile(".")
	defer release()
	for _, body := range []string{strings.Replace(original, "body one", "body two", 1), original + "appended\n"} {
		mustWriteFile(t, native, body)
		drift, _, err := captureAllAndDiff(cfg.Targets, cfg, b)
		if err != nil {
			t.Fatal(err)
		}
		if drift == 0 {
			t.Error("status hid native edit")
		}
		reports, err := collectLoadedDrift(cfg, b, nil, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, rep := range reports {
			for _, file := range rep.Edited {
				found = found || filepath.ToSlash(file.Path) == native
			}
		}
		if !found {
			t.Error("drift hid native edit or changed its Edited classification")
		}
	}
	current := readStateFile(".")
	current.OutputSums[filepath.FromSlash(native)] = adapters.ContentSum(readFile(t, native))
	encoded, err := json.Marshal(current)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, stateFilePath("."), string(encoded))
	reports, err := collectLoadedDrift(cfg, b, nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	foundStale := false
	for _, rep := range reports {
		for _, file := range rep.Stale {
			foundStale = foundStale || filepath.ToSlash(file.Path) == native
		}
	}
	if !foundStale {
		t.Error("classification borrowed outer sums instead of reading current disk ledger")
	}
}

func TestCapturePhases_ReleaseBeforeDeferredHistory(t *testing.T) {
	for _, mode := range []string{"status", "drift"} {
		t.Run(mode, func(t *testing.T) {
			dir, git := launchGitRepo(t)
			testutil.Chdir(t, dir)
			silence(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\ngitignore: {enabled: false}\n")
			mustWriteFile(t, ".agnostic-ai/settings/model.yaml", "name: model\nmodel: sonnet\n")
			syncProject(t)
			git("add", "-A")
			git("commit", "-q", "-m", "generated settings")
			if err := os.Remove(stateFilePath(".")); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(".agnostic-ai/settings/model.yaml"); err != nil {
				t.Fatal(err)
			}
			cfg, b, err := loadProject(".")
			if err != nil {
				t.Fatal(err)
			}
			called := false
			adapters.SetPriorMergedKeys(func(path string) []adapters.MergedKey {
				cwd, err := os.Getwd()
				if err != nil {
					t.Fatal(err)
				}
				physical, err := filepath.EvalSymlinks(dir)
				if err != nil {
					t.Fatal(err)
				}
				if cwd != physical {
					called = true
					heldState.Lock()
					frame := heldState.frame
					isolated := frame != nil && frame.state == nil && frame.previous == nil
					heldState.Unlock()
					if !isolated {
						t.Error("history inherited capture-owned frame")
					}
				}
				return slices.Clone(priorStateFile().Merged[path].Keys)
			})
			defer restorePriorMergedKeys()
			if mode == "status" {
				_, _, err = captureAllAndDiff(cfg.Targets, cfg, b)
			} else {
				_, err = collectLoadedDrift(cfg, b, nil, nil, "")
			}
			if err != nil {
				t.Fatal(err)
			}
			if !called {
				t.Error("deferred leftover history never captured")
			}
			heldState.Lock()
			retained := heldState.frame != nil
			heldState.Unlock()
			if retained {
				t.Error("capture or history frame leaked")
			}
		})
	}
}

func TestCapturePhases_ReleaseOnCaptureAndNativeReadErrors(t *testing.T) {
	for _, mode := range []string{"status", "drift"} {
		t.Run(mode, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			writeBenchProject(t, dir, 1)
			cfg, b, err := loadProject(".")
			if err != nil {
				t.Fatal(err)
			}
			mustWriteFile(t, stateFilePath("."), `{"output_sums":{"probe":"before"}}`)
			cfg.Targets = []string{"claude"}
			cfg.Outputs = map[string]config.Output{"claude": {Dir: "native"}}
			if err := os.MkdirAll("native/settings.json", 0755); err != nil {
				t.Fatal(err)
			}
			if mode == "status" {
				_, _, err = captureAllAndDiff(cfg.Targets, cfg, b)
			} else {
				_, err = collectLoadedDrift(cfg, b, nil, nil, "")
			}
			if err == nil {
				t.Error("invalid native settings did not fail capture")
			}
			mustWriteFile(t, stateFilePath("."), `{"output_sums":{"probe":"after"}}`)
			if priorStateFile().OutputSums["probe"] != "after" {
				t.Error("failed capture retained ledger")
			}
			cfg.Targets = []string{"trae"}
			cfg.Outputs = map[string]config.Output{"trae": {AgentsDir: "native-agents"}}
			if err := os.MkdirAll("native-agents/agent-0000.md", 0755); err != nil {
				t.Fatal(err)
			}
			if mode == "status" {
				_, _, err = captureAllAndDiff(cfg.Targets, cfg, b)
			} else {
				_, err = collectLoadedDrift(cfg, b, nil, nil, "")
			}
			if err == nil || !strings.Contains(filepath.ToSlash(err.Error()), "read native-agents/agent-0000.md") {
				t.Errorf("native comparison failure=%v", err)
			}
			mustWriteFile(t, stateFilePath("."), `{"output_sums":{"probe":"last"}}`)
			if priorStateFile().OutputSums["probe"] != "last" {
				t.Error("failed native comparison retained ledger")
			}
		})
	}
}
