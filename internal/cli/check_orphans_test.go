package cli

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const keptReference = ".claude/skills/gone/references/a.md"

// syncedWithKeptOrphan leaves a project in the state a sync produces when
// it could not remove an orphan: entry points in sync, the orphan on disk
// and recorded in the state file.
func syncedWithKeptOrphan(t *testing.T) *config.Config {
	t.Helper()
	testutil.TempCwd(t)
	writeAgnosticFile(t, "# Pointer body\n")
	cfg := &config.Config{Targets: []string{"claude"}}
	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, spec.Bundle{}, cfg.Targets, false); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, keptReference, "edited\n")
	ledger := syncLedger{outputs: []string{keptReference}, orphans: []string{keptReference}}
	if err := writeStateFile(".", 0, "", "", ledger); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestCollectEntryPointDrift_ReportsKeptOrphanStillOnDisk(t *testing.T) {
	cfg := syncedWithKeptOrphan(t)

	rep, err := collectEntryPointDrift(cfg, spec.Bundle{}, cfg.Targets)
	if err != nil {
		t.Fatal(err)
	}

	if !rep.hasDrift() {
		t.Fatal("kept orphan must count as drift")
	}
	if len(rep.Orphaned) != 1 || rep.Orphaned[0] != keptReference {
		t.Errorf("Orphaned=%v, want [%s]", rep.Orphaned, keptReference)
	}
}

func TestCollectEntryPointDrift_IgnoresOrphanDeletedByHand(t *testing.T) {
	cfg := syncedWithKeptOrphan(t)
	if err := os.Remove(keptReference); err != nil {
		t.Fatal(err)
	}

	rep, err := collectEntryPointDrift(cfg, spec.Bundle{}, cfg.Targets)
	if err != nil {
		t.Fatal(err)
	}

	if rep.hasDrift() {
		t.Errorf("deleted orphan still reported: %+v", rep.Orphaned)
	}
}

func TestCollectEntryPointDrift_IgnoresOrphanListedUnmanaged(t *testing.T) {
	cfg := syncedWithKeptOrphan(t)
	cfg.Sync.Unmanaged = []string{keptReference}

	rep, err := collectEntryPointDrift(cfg, spec.Bundle{}, cfg.Targets)
	if err != nil {
		t.Fatal(err)
	}

	if len(rep.Orphaned) != 0 {
		t.Errorf("user-owned orphan reported: %v", rep.Orphaned)
	}
}

func TestReportCheckDrift_NamesOrphanInEveryFormat(t *testing.T) {
	reports := []driftReport{{Target: "agnostic-ai", Orphaned: []string{keptReference}}}
	for _, format := range []string{checkFormatHuman, checkFormatGitHub} {
		t.Run(format, func(t *testing.T) {
			var out bytes.Buffer
			cmd := &cobra.Command{}
			cmd.SetOut(&out)
			cmd.SetErr(&bytes.Buffer{})
			prev := logOut
			logOut = &out
			defer func() { logOut = prev }()

			if err := reportCheckDrift(cmd, reports, format, true); err == nil {
				t.Error("orphan drift must fail the check")
			}
			if !strings.Contains(out.String(), keptReference) {
				t.Errorf("output does not name the orphan:\n%s", out.String())
			}
		})
	}
}

func TestPrintSyncCheckJSON_ListsOrphanAsWrite(t *testing.T) {
	reports := []driftReport{{Target: "agnostic-ai", Orphaned: []string{keptReference}}}
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)

	if err := printSyncCheckJSON(cmd, reports, nil); err == nil {
		t.Error("orphan drift must fail the check")
	}
	if !strings.Contains(out.String(), `"action": "orphan"`) || !strings.Contains(out.String(), keptReference) {
		t.Errorf("JSON does not list the orphan:\n%s", out.String())
	}
}

func TestDoctorFix_OffersRecordedOrphanRemoval(t *testing.T) {
	cfg := syncedWithKeptOrphan(t)
	reports := []driftReport{{Target: "agnostic-ai", Orphaned: []string{keptReference}}}
	calls := 0
	removed, err := offerOrphanRemoval(cfg, reports, true, func(path string) (bool, error) {
		calls++
		if path != keptReference {
			t.Errorf("offered %s", path)
		}
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || removed != 1 || orphanedCount(reports) != 0 {
		t.Errorf("calls=%d removed=%d remaining=%d", calls, removed, orphanedCount(reports))
	}
	if fileExists(keptReference) {
		t.Error("confirmed orphan remains")
	}
	backup, err := os.ReadFile(keptReference + ".bak")
	if err != nil || string(backup) != "edited\n" {
		t.Errorf("backup=%q err=%v", backup, err)
	}
}

func TestDoctorFix_DeclinedOrphanStays(t *testing.T) {
	cfg := syncedWithKeptOrphan(t)
	reports := []driftReport{{Target: "agnostic-ai", Orphaned: []string{keptReference}}}
	removed, err := offerOrphanRemoval(cfg, reports, false, func(string) (bool, error) { return false, nil })
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 || orphanedCount(reports) != 1 || !fileExists(keptReference) {
		t.Errorf("removed=%d remaining=%d exists=%v", removed, orphanedCount(reports), fileExists(keptReference))
	}
}

func TestDoctorFix_OffersOnlyRecordedManagedFiles(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*config.Config, []driftReport)
	}{
		{"unmanaged", func(cfg *config.Config, _ []driftReport) { cfg.Sync.Unmanaged = []string{keptReference} }},
		{"unledgered", func(_ *config.Config, reports []driftReport) { reports[0].Unledgered = true }},
		{"directory", func(_ *config.Config, _ []driftReport) {
			if err := os.Remove(keptReference); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(keptReference, 0o755); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := syncedWithKeptOrphan(t)
			reports := []driftReport{{Target: "agnostic-ai", Orphaned: []string{keptReference}}}
			tc.change(cfg, reports)
			removed, err := offerOrphanRemoval(cfg, reports, false, func(string) (bool, error) { t.Error("unsafe removal offered"); return true, nil })
			if err != nil {
				t.Fatal(err)
			}
			if removed != 0 || !fileExists(keptReference) {
				t.Errorf("removed=%d exists=%v", removed, fileExists(keptReference))
			}
		})
	}
}

func TestDoctorFix_NoninteractiveKeepsRecordedOrphan(t *testing.T) {
	syncedWithKeptOrphan(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader("yes\n"))
	root.SetArgs([]string{"doctor", "--fix"})
	err := root.Execute()
	if err == nil {
		t.Error("doctor passed while an orphan remains")
	}
	if !fileExists(keptReference) {
		t.Error("noninteractive doctor deleted orphan")
	}
	if !strings.Contains(out.String(), "run `agnostic-ai doctor --fix` in a terminal") {
		t.Errorf("missing actionable hint:\n%s", out.String())
	}
}

func TestDoctorFix_ConfirmationDefaultsToKeepingOrphan(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  bool
	}{
		{"\n", false}, {"no\n", false}, {"yes\n", true}, {"Y\n", true}, {"", false},
	} {
		t.Run(tc.input, func(t *testing.T) {
			var out bytes.Buffer
			cmd := &cobra.Command{}
			cmd.SetOut(&out)
			got, err := confirmOrphanRemoval(cmd, bufio.NewReader(strings.NewReader(tc.input)), keptReference)
			if err != nil || got != tc.want {
				t.Errorf("confirm=%v err=%v want=%v", got, err, tc.want)
			}
			if !strings.Contains(out.String(), keptReference) || !strings.Contains(out.String(), "[y/N]") {
				t.Errorf("missing path or safe default: %s", out.String())
			}
		})
	}
}

func TestDoctorFix_EditDuringConfirmationStays(t *testing.T) {
	cfg := syncedWithKeptOrphan(t)
	reports := []driftReport{{Target: "agnostic-ai", Orphaned: []string{keptReference}}}
	removed, err := offerOrphanRemoval(cfg, reports, false, func(string) (bool, error) {
		mustWriteFile(t, keptReference, "new edit\n")
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(keptReference)
	if err != nil || string(data) != "new edit\n" || removed != 0 || orphanedCount(reports) != 1 {
		t.Errorf("data=%q err=%v removed=%d remaining=%d", data, err, removed, orphanedCount(reports))
	}
}

func TestDoctorFix_RechecksPathAfterConfirmation(t *testing.T) {
	for _, replacement := range []string{"parent symlink", "file symlink", "directory"} {
		t.Run(replacement, func(t *testing.T) {
			cfg := syncedWithKeptOrphan(t)
			outside := t.TempDir()
			external := filepath.Join(outside, "a.md")
			mustWriteFile(t, external, "edited\n")
			probe := filepath.Join(outside, "probe")
			if err := os.Symlink(external, probe); err != nil {
				if errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.ENOSYS) {
					t.Skipf("symlinks unavailable: %v", err)
				}
				t.Fatal(err)
			}
			if err := os.Remove(probe); err != nil {
				t.Fatal(err)
			}
			reports := []driftReport{{Target: "agnostic-ai", Orphaned: []string{keptReference}}}
			removed, err := offerOrphanRemoval(cfg, reports, true, func(string) (bool, error) {
				replaced := keptReference
				if replacement == "parent symlink" {
					replaced = filepath.Dir(keptReference)
				}
				if err := os.Rename(replaced, replaced+".original"); err != nil {
					t.Fatal(err)
				}
				switch replacement {
				case "parent symlink":
					if err := os.Symlink(outside, replaced); err != nil {
						t.Fatal(err)
					}
				case "file symlink":
					if err := os.Symlink(external, replaced); err != nil {
						t.Fatal(err)
					}
				case "directory":
					if err := os.Mkdir(replaced, 0o755); err != nil {
						t.Fatal(err)
					}
				}
				return true, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if removed != 0 || orphanedCount(reports) != 1 {
				t.Errorf("removed=%d remaining=%d", removed, orphanedCount(reports))
			}
			data, err := os.ReadFile(external)
			if err != nil || string(data) != "edited\n" {
				t.Errorf("external file=%q err=%v", data, err)
			}
			if _, err := os.Lstat(keptReference); err != nil {
				t.Errorf("replacement was removed: %v", err)
			}
			if _, err := os.Lstat(external + ".bak"); !os.IsNotExist(err) {
				t.Errorf("external backup written: %v", err)
			}
			if _, err := os.Lstat(keptReference + ".bak"); !os.IsNotExist(err) {
				t.Errorf("replacement backup written: %v", err)
			}
		})
	}
}

func TestDoctorFix_RefusesSymlinkBackupDestination(t *testing.T) {
	cfg := syncedWithKeptOrphan(t)
	external := filepath.Join(t.TempDir(), "backup.md")
	mustWriteFile(t, external, "outside data\n")
	if err := os.Symlink(external, keptReference+".bak"); err != nil {
		if errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.ENOSYS) {
			t.Skipf("symlinks unavailable: %v", err)
		}
		t.Fatal(err)
	}
	reports := []driftReport{{Target: "agnostic-ai", Orphaned: []string{keptReference}}}
	removed, err := offerOrphanRemoval(cfg, reports, true, func(string) (bool, error) { return true, nil })
	if err == nil || !strings.Contains(err.Error(), keptReference+".bak") {
		t.Errorf("error=%v, want unsafe backup path", err)
	}
	if removed != 0 || !fileExists(keptReference) {
		t.Errorf("removed=%d exists=%v", removed, fileExists(keptReference))
	}
	data, readErr := os.ReadFile(external)
	if readErr != nil || string(data) != "outside data\n" {
		t.Errorf("external backup=%q err=%v", data, readErr)
	}
}

func TestDoctorFix_RestoredSpecIsNotOfferedAsOrphan(t *testing.T) {
	for _, state := range []string{"current", "stale", "missing"} {
		t.Run(state, func(t *testing.T) {
			testutil.TempCwd(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
			mustWriteFile(t, ".agnostic-ai/skills/gone/SKILL.md", "---\nname: gone\ndescription: Restored skill.\n---\nRead references/a.md.\n")
			mustWriteFile(t, ".agnostic-ai/skills/gone/references/a.md", "restored\n")
			silence(t)
			syncProject(t)
			const genuine = ".claude/skills/removed/reference.md"
			mustWriteFile(t, genuine, "legacy\n")
			prev := readStateFile(".")
			if err := writeStateFile(".", 0, "", "", syncLedger{outputs: append(prev.Outputs, genuine), orphans: []string{keptReference, genuine}}); err != nil {
				t.Fatal(err)
			}
			switch state {
			case "stale":
				mustWriteFile(t, keptReference, "old\n")
			case "missing":
				if err := os.Remove(keptReference); err != nil {
					t.Fatal(err)
				}
			}
			reports, err := collectDrift(nil)
			if err != nil {
				t.Fatal(err)
			}
			foundGenuine := false
			for _, rep := range reports {
				for _, path := range rep.Orphaned {
					if path == keptReference {
						t.Errorf("restored output still classified as orphan: %s", path)
					}
					if path == genuine {
						foundGenuine = true
					}
				}
			}
			if !foundGenuine {
				t.Error("genuine orphan was dropped")
			}
			// Feed an outdated classification to the removal helper to verify its guard.
			reports = append(reports, driftReport{Target: "agnostic-ai", Orphaned: []string{keptReference}})
			if _, err := fixDrift(reports, false); err != nil {
				t.Fatal(err)
			}
			cfg, _, err := loadProject(".")
			if err != nil {
				t.Fatal(err)
			}
			removed, err := offerOrphanRemoval(cfg, reports, false, func(path string) (bool, error) {
				if path == keptReference {
					t.Errorf("offered restored output for deletion: %s", path)
				}
				return true, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(keptReference)
			if err != nil || string(data) != "restored\n" {
				t.Errorf("restored output=%q err=%v", data, err)
			}
			if removed != 1 || fileExists(genuine) {
				t.Errorf("genuine orphan removal: count=%d exists=%v", removed, fileExists(genuine))
			}
		})
	}
}

func TestDoctorFix_PartialTargetKeepsOtherTargetRestoredOutput(t *testing.T) {
	testutil.TempCwd(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n")
	mustWriteFile(t, ".agnostic-ai/skills/gone/SKILL.md", "---\nname: gone\ndescription: Restored skill.\ntarget: claude\n---\nRead references/a.md.\n")
	mustWriteFile(t, ".agnostic-ai/skills/gone/references/a.md", "restored\n")
	silence(t)
	syncProject(t)
	const genuine = ".claude/skills/removed/reference.md"
	const other = ".claude/skills/gone/SKILL.md"
	mustWriteFile(t, other, "edited other target\n")
	mustWriteFile(t, genuine, "legacy\n")
	prev := readStateFile(".")
	if err := writeStateFile(".", 0, "", "", syncLedger{outputs: append(prev.Outputs, genuine), orphans: []string{keptReference, genuine}}); err != nil {
		t.Fatal(err)
	}
	reports, err := collectDrift([]string{"codex"})
	if err != nil {
		t.Fatal(err)
	}
	foundGenuine := false
	for _, rep := range reports {
		if rep.Target == "claude" {
			t.Error("partial check added Claude drift to fix")
		}
		for _, path := range rep.Orphaned {
			if path == keptReference {
				t.Errorf("other target's restored output classified as orphan: %s", path)
			}
			if path == genuine {
				foundGenuine = true
			}
		}
	}
	if !foundGenuine {
		t.Error("genuine orphan disappeared")
	}
	if _, err := fixDrift(reports, false); err != nil {
		t.Fatal(err)
	}
	// An outdated orphan report must not bypass the removal guard.
	reports = append(reports, driftReport{Target: "agnostic-ai", Orphaned: []string{keptReference}})
	cfg, _, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	removed, err := offerOrphanRemoval(cfg, reports, false, func(path string) (bool, error) {
		if path == keptReference {
			t.Errorf("other target's restored output offered for deletion: %s", path)
		}
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{keptReference: "restored\n", other: "edited other target\n"} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Errorf("other target output %s=%q err=%v want=%q", path, data, err, want)
		}
	}
	if removed != 1 || fileExists(genuine) {
		t.Errorf("genuine orphan count=%d exists=%v", removed, fileExists(genuine))
	}
}

func TestDoctorFix_RestoredOutputPathAliasesAreNotOfferedAsOrphans(t *testing.T) {
	for _, alias := range []string{"./" + keptReference, ".claude//skills/gone/references/a.md", ".claude/skills/gone/references/./a.md"} {
		t.Run(alias, func(t *testing.T) {
			testutil.TempCwd(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
			mustWriteFile(t, ".agnostic-ai/skills/gone/SKILL.md", "---\nname: gone\ndescription: Restored skill.\n---\nRead references/a.md.\n")
			mustWriteFile(t, ".agnostic-ai/skills/gone/references/a.md", "restored\n")
			silence(t)
			syncProject(t)
			previous := readStateFile(".")
			if err := writeStateFile(".", 0, "", "", syncLedger{outputs: previous.Outputs, orphans: []string{alias}}); err != nil {
				t.Fatal(err)
			}
			reports, err := collectDrift(nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, report := range reports {
				for _, path := range report.Orphaned {
					if path == alias {
						t.Errorf("restored alias classified as orphan: %s", path)
					}
				}
			}
			reports = append(reports, driftReport{Target: "agnostic-ai", Orphaned: []string{alias}})
			cfg, _, err := loadProject(".")
			if err != nil {
				t.Fatal(err)
			}
			removed, err := offerOrphanRemoval(cfg, reports, false, func(path string) (bool, error) {
				t.Errorf("restored alias offered for deletion: %s", path)
				return true, nil
			})
			if err != nil || removed != 0 {
				t.Errorf("removal = %d, error %v", removed, err)
			}
			data, err := os.ReadFile(keptReference)
			if err != nil || string(data) != "restored\n" {
				t.Errorf("restored output %q, error %v", data, err)
			}
		})
	}
}

// failingAdapterOnPath puts an external adapter for target on PATH that
// exits non-zero, so the target resolves but capturing its files fails.
func failingAdapterOnPath(t *testing.T, target string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture requires a POSIX shell")
	}
	bin := t.TempDir()
	script := filepath.Join(bin, "agnostic-ai-adapter-"+target)
	mustWriteFile(t, script, "#!/bin/sh\necho adapter crashed >&2\nexit 1\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestOrphanCheck_SkipsTargetsItCannotLoad(t *testing.T) {
	for _, tc := range []struct {
		name    string
		target  string
		adapter func(*testing.T, string)
		args    []string
	}{
		{"doctor with a binary not on PATH", "acme", nil, []string{"doctor"}},
		{"sync --check with a binary not on PATH", "acme", nil, []string{"sync", "--check"}},
		{"partial doctor with a failing adapter", "flaky", failingAdapterOnPath, []string{"doctor", "-t", "claude"}},
		{"partial sync --check with a failing adapter", "flaky", failingAdapterOnPath, []string{"sync", "-t", "claude", "--check"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			syncedWithKeptOrphan(t)
			if tc.adapter != nil {
				tc.adapter(t, tc.target)
			}
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, "+tc.target+"]\n")
			var logged bytes.Buffer
			prev := logOut
			logOut = &logged
			defer func() { logOut = prev }()

			var out string
			var err error
			stderr := captureStderr(t, func() { out, err = runCLI(t, tc.args...) })

			if err == nil || !strings.Contains(err.Error(), "drift detected") {
				t.Errorf("error=%v, want the drift error", err)
			}
			if err != nil && strings.Contains(err.Error(), "verify orphan producers") {
				t.Errorf("an unloadable target aborted the check: %v", err)
			}
			if !strings.Contains(logged.String(), keptReference) {
				t.Errorf("recorded orphan not reported:\n%s%s", logged.String(), out)
			}
			if !strings.Contains(stderr, tc.target) || strings.Count(stderr, "could not load") > 1 {
				t.Errorf("want one warning naming %s, got stderr:\n%s", tc.target, stderr)
			}
		})
	}
}

func TestCollectDrift_UnloadedTargetDoesNotHideOtherProducers(t *testing.T) {
	testutil.TempCwd(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [acme, claude, codex]\n")
	mustWriteFile(t, ".agnostic-ai/skills/gone/SKILL.md", "---\nname: gone\ndescription: Restored skill.\ntarget: claude\n---\nRead references/a.md.\n")
	mustWriteFile(t, ".agnostic-ai/skills/gone/references/a.md", "restored\n")
	silence(t)
	syncProject(t)
	const genuine = ".claude/skills/removed/reference.md"
	mustWriteFile(t, genuine, "legacy\n")
	prev := readStateFile(".")
	if err := writeStateFile(".", 0, "", "", syncLedger{outputs: append(prev.Outputs, genuine), orphans: []string{keptReference, genuine}}); err != nil {
		t.Fatal(err)
	}

	reports, err := collectDrift([]string{"codex"})
	if err != nil {
		t.Fatal(err)
	}

	foundGenuine := false
	for _, rep := range reports {
		for _, path := range rep.Orphaned {
			if path == keptReference {
				t.Errorf("output another target generates classified as orphan: %s", path)
			}
			if path == genuine {
				foundGenuine = true
			}
		}
	}
	if !foundGenuine {
		t.Error("genuine orphan disappeared")
	}
}

func TestDoctorFix_KeepsOrphansWhileAProducerIsUnloaded(t *testing.T) {
	const second = ".claude/skills/gone/references/b.md"
	for _, tc := range []struct {
		name             string
		missing, failing []string
	}{
		{"binary not on PATH", []string{"acme"}, nil},
		{"adapter fails", nil, []string{"flaky"}},
		{"both", []string{"acme"}, []string{"flaky"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := syncedWithKeptOrphan(t)
			unloaded := slices.Concat(tc.missing, tc.failing)
			for _, target := range tc.failing {
				failingAdapterOnPath(t, target)
			}
			cfg.Targets = append(cfg.Targets, unloaded...)
			mustWriteFile(t, second, "edited\n")
			ledger := syncLedger{outputs: []string{keptReference, second}, orphans: []string{keptReference, second}}
			if err := writeStateFile(".", 0, "", "", ledger); err != nil {
				t.Fatal(err)
			}
			reports := []driftReport{{Target: "agnostic-ai", Orphaned: []string{keptReference, second}}}
			var logged bytes.Buffer
			prev := logOut
			logOut = &logged
			defer func() { logOut = prev }()

			removed, err := offerOrphanRemoval(cfg, reports, false, func(path string) (bool, error) {
				t.Errorf("offered %s while %v cannot be loaded", path, unloaded)
				return true, nil
			})

			if err != nil {
				t.Fatal(err)
			}
			if removed != 0 || orphanedCount(reports) != 2 || !fileExists(keptReference) || !fileExists(second) {
				t.Errorf("removed=%d remaining=%d", removed, orphanedCount(reports))
			}
			got := logged.String()
			if strings.Count(got, "\n") != 1 {
				t.Errorf("want one line, got:\n%s", got)
			}
			for _, target := range unloaded {
				if !strings.Contains(got, target) {
					t.Errorf("line does not name %s:\n%s", target, got)
				}
			}
		})
	}
}

// danglingImportProject leaves a kept orphan beside an entry point only an
// unrun target cannot render. Claude keeps the @-line for itself, while
// codex's AGENTS.md needs the file it names under resolve-imports: inline.
func danglingImportProject(t *testing.T) *config.Config {
	t.Helper()
	cfg := syncedWithKeptOrphan(t)
	writeAgnosticFile(t, "# Pointer body\n\n@docs/missing.md\n")
	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, spec.Bundle{}, cfg.Targets, false); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\nsync:\n  resolve-imports: inline\n")
	cfg.Targets = []string{"claude", "codex"}
	cfg.Sync.ResolveImports = "inline"
	return cfg
}

func TestOrphanCheck_KeepsCheckingWhenAnUnrunEntryPointFailsToRender(t *testing.T) {
	for _, tc := range []struct {
		args    []string
		wantErr string
	}{
		{[]string{"sync", "-t", "claude", "--check"}, "drift detected"},
		{[]string{"doctor", "-t", "claude"}, "drift detected"},
		{[]string{"doctor", "-t", "claude", "--fix"}, "need manual removal"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			danglingImportProject(t)
			var logged bytes.Buffer
			prev := logOut
			logOut = &logged
			defer func() { logOut = prev }()

			var out string
			var err error
			stderr := captureStderr(t, func() { out, err = runCLI(t, tc.args...) })

			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error=%v, want %q", err, tc.wantErr)
			}
			if !strings.Contains(logged.String(), keptReference) {
				t.Errorf("recorded orphan not reported:\n%s%s", logged.String(), out)
			}
			if !fileExists(keptReference) {
				t.Error("orphan removed while codex's entry point cannot render")
			}
			if strings.Contains(stderr, "could not load") || strings.Contains(logged.String(), "could not be loaded") {
				t.Errorf("codex reported as unloaded:\n%s%s", stderr, logged.String())
			}
			if strings.Count(stderr, "could not render entry points") != 1 || !strings.Contains(stderr, "docs/missing.md") {
				t.Errorf("want one warning naming the render error, got stderr:\n%s", stderr)
			}
		})
	}
}

func TestOrphanCheck_WarnsOncePerUnloadedTarget(t *testing.T) {
	danglingImportProject(t)
	failingAdapterOnPath(t, "flaky")
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex, flaky]\nsync:\n  resolve-imports: inline\n")
	var logged bytes.Buffer
	prev := logOut
	logOut = &logged
	defer func() { logOut = prev }()

	var err error
	stderr := captureStderr(t, func() { _, err = runCLI(t, "sync", "-t", "claude", "--check") })

	if err == nil || !strings.Contains(err.Error(), "drift detected") {
		t.Errorf("error=%v, want the drift error", err)
	}
	for warning, want := range map[string]int{"could not load flaky": 1, "could not load codex": 0, "could not render entry points": 1} {
		if got := strings.Count(stderr, warning); got != want {
			t.Errorf("%d warnings %q, want %d; stderr:\n%s", got, warning, want, stderr)
		}
	}
}

func TestDoctorFix_KeepsOrphansWhileAnUnrunEntryPointCannotRender(t *testing.T) {
	cfg := danglingImportProject(t)
	reports := []driftReport{{Target: "claude"}, {Target: "agnostic-ai", Orphaned: []string{keptReference}}}
	var logged bytes.Buffer
	prev := logOut
	logOut = &logged
	defer func() { logOut = prev }()

	removed, err := offerOrphanRemoval(cfg, reports, false, func(path string) (bool, error) {
		t.Errorf("offered %s while codex's entry point cannot render", path)
		return true, nil
	})

	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 || orphanedCount(reports) != 1 || !fileExists(keptReference) {
		t.Errorf("removed=%d remaining=%d", removed, orphanedCount(reports))
	}
	got := logged.String()
	if strings.Count(got, "\n") != 1 || !strings.Contains(got, "entry points could not be rendered") || strings.Contains(got, "could not be loaded") {
		t.Errorf("want one line naming the render failure, got:\n%s", got)
	}
}

func TestDoctorFix_NamesEachReasonItKeepsOrphans(t *testing.T) {
	cfg := danglingImportProject(t)
	failingAdapterOnPath(t, "flaky")
	cfg.Targets = append(cfg.Targets, "flaky")
	reports := []driftReport{{Target: "claude"}, {Target: "agnostic-ai", Orphaned: []string{keptReference}}}
	var logged bytes.Buffer
	prev := logOut
	logOut = &logged
	defer func() { logOut = prev }()

	removed, err := offerOrphanRemoval(cfg, reports, false, func(path string) (bool, error) {
		t.Errorf("offered %s while flaky and codex's entry point are unknown", path)
		return true, nil
	})

	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 || !fileExists(keptReference) {
		t.Errorf("removed=%d exists=%v", removed, fileExists(keptReference))
	}
	got := logged.String()
	if strings.Count(got, "\n") != 1 || !strings.Contains(got, "target(s) flaky could not be loaded") || !strings.Contains(got, "entry points could not be rendered") {
		t.Errorf("want one line naming both reasons, got:\n%s", got)
	}
}
