package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/overview"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

type servedEvidence struct {
	overview  overview.View
	authority struct {
		Snapshot struct {
			Evidence struct {
				Installed string `json:"installed"`
			} `json:"evidence"`
		} `json:"snapshot"`
	}
}

func serveAndFetchEvidence(t *testing.T, args ...string) servedEvidence {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var got servedEvidence
	var fetchErr error
	done := make(chan struct{})
	cmd := newServeCmdWithBrowser(func(url string) error {
		go func() {
			defer close(done)
			defer cancel()
			get := func(path string, into any) {
				resp, err := http.Get(url + path)
				if err != nil {
					fetchErr = err
					return
				}
				defer func() { _ = resp.Body.Close() }()
				if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
					fetchErr = err
				}
			}
			get("api/views/overview", &got.overview)
			get("api/views/authority", &got.authority)
		}()
		return nil
	})
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append(args, "--addr", "127.0.0.1:0", "--open"))
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	<-done
	if fetchErr != nil {
		t.Fatal(fetchErr)
	}
	return got
}

func TestServe_NoInstalledSkipsTheProjectOverviewComparison(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeBareReadProject(t, dir)

	skipped := serveAndFetchEvidence(t, "--no-installed")
	if got := skipped.overview.Snapshot.Evidence.Installed; got != "not-checked" {
		t.Errorf("overview installed evidence = %q, want not-checked", got)
	}
	if got := skipped.authority.Snapshot.Evidence.Installed; got != "not-checked" {
		t.Errorf("authority installed evidence = %q, want not-checked", got)
	}
	for _, target := range skipped.overview.Targets {
		if target.Sync.State != overview.SyncNotChecked || len(target.Sync.Files) != 0 {
			t.Errorf("target %s sync = %+v, want not-checked with no files", target.Target, target.Sync)
		}
	}

	compared := serveAndFetchEvidence(t)
	if got := compared.overview.Snapshot.Evidence.Installed; got != "checked" {
		t.Errorf("overview installed evidence = %q, want checked", got)
	}
	if len(compared.overview.Targets) != 1 || compared.overview.Targets[0].Sync.State != overview.SyncDrifted {
		t.Errorf("targets = %+v, want claude drifted", compared.overview.Targets)
	}
}

func TestRunOverviewChecks_SkipsRenderingAndReadingGeneratedFiles(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeBareReadProject(t, dir)
	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "CLAUDE.md"), "stale\n")

	skipped := runOverviewChecksWith(".", cfg, b, false)
	if len(skipped.compared) != 0 || len(skipped.skipped) != 0 || len(driftByPath(t, skipped.findings)) != 0 || len(skipped.fieldNotes) != 0 {
		t.Errorf("checks = %+v, want no drift work", skipped)
	}
	if len(skipped.findings) == 0 {
		t.Error("source checks did not run")
	}
	if full := runOverviewChecksWith(".", cfg, b, true); len(full.compared) != 1 || driftByPath(t, full.findings)["CLAUDE.md"] != driftStale {
		t.Errorf("checks = %+v, want claude compared with CLAUDE.md stale", full)
	}
}

func TestServe_NoInstalledSkipsTheGlobalOverviewComparison(t *testing.T) {
	globalOverviewHome(t)
	testutil.Chdir(t, t.TempDir())

	skipped := serveAndFetchEvidence(t, "--global", "--no-installed")
	if got := skipped.overview.Snapshot.Evidence.Installed; got != "not-checked" {
		t.Errorf("overview installed evidence = %q, want not-checked", got)
	}
	if got := skipped.authority.Snapshot.Evidence.Installed; got != "not-checked" {
		t.Errorf("authority installed evidence = %q, want not-checked", got)
	}
	for _, target := range skipped.overview.Targets {
		if target.Sync.State != overview.SyncNotChecked || len(target.Sync.Files) != 0 {
			t.Errorf("target %s sync = %+v, want not-checked with no files", target.Target, target.Sync)
		}
	}
	if got := serveAndFetchEvidence(t, "--global").overview.Snapshot.Evidence.Installed; got != "checked" {
		t.Errorf("overview installed evidence = %q, want checked without the flag", got)
	}
}

func TestServe_NoInstalledKeepsTheGlobalNoteForSettingsSyncDoesNotWrite(t *testing.T) {
	home := globalOverviewHome(t)
	source := filepath.Join(home, "source")
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets:\n  - claude\n  - codex\n  - cursor\n  - opencode\n")
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "base.yaml"), "model: gpt-5\neffort: high\n")
	testutil.Chdir(t, t.TempDir())

	const reason = "sync --global does not write this target's user settings yet"
	for _, args := range [][]string{{"--global"}, {"--global", "--no-installed"}} {
		served := serveAndFetchEvidence(t, args...)
		rows := resolvedByTarget(t, &served.overview)
		for _, field := range []string{"model", "effort"} {
			for _, target := range []string{"cursor", "opencode"} {
				if got := rows[field][target]; !strings.Contains(got.Dropped, reason) {
					t.Errorf("%v: %s %s = %+v, want dropped with %q", args, target, field, got, reason)
				}
			}
		}
	}
}
