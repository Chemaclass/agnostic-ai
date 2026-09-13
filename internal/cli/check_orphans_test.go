package cli

import (
	"bytes"
	"os"
	"strings"
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

	if err := printSyncCheckJSON(cmd, reports); err == nil {
		t.Error("orphan drift must fail the check")
	}
	if !strings.Contains(out.String(), `"action": "orphan"`) || !strings.Contains(out.String(), keptReference) {
		t.Errorf("JSON does not list the orphan:\n%s", out.String())
	}
}
