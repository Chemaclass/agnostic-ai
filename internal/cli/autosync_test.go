package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// helpers

func boolPtr(b bool) *bool { return &b }

func writeSimpleConfig(t *testing.T, dir string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, "agnostic.config.yaml"),
		[]byte("version: 1\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
}

// PersistAutoSync tests

func TestPersistAutoSync_AppendsWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	writeSimpleConfig(t, dir)

	if err := config.PersistAutoSync(dir, true); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "agnostic.config.yaml"))
	if !strings.Contains(string(data), "autoSync: true") {
		t.Errorf("expected autoSync: true in config, got:\n%s", data)
	}
}

func TestPersistAutoSync_UpdatesExisting(t *testing.T) {
	dir := t.TempDir()
	content := "version: 1\nautoSync: false\n"
	if err := os.WriteFile(filepath.Join(dir, "agnostic.config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := config.PersistAutoSync(dir, true); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "agnostic.config.yaml"))
	if !strings.Contains(string(data), "autoSync: true") {
		t.Errorf("expected autoSync: true after update, got:\n%s", data)
	}
	if strings.Contains(string(data), "autoSync: false") {
		t.Error("old autoSync: false still present after update")
	}
}

func TestPersistAutoSync_IdempotentOnRepeat(t *testing.T) {
	dir := t.TempDir()
	writeSimpleConfig(t, dir)

	for range 3 {
		if err := config.PersistAutoSync(dir, true); err != nil {
			t.Fatal(err)
		}
	}

	data, _ := os.ReadFile(filepath.Join(dir, "agnostic.config.yaml"))
	count := strings.Count(string(data), "autoSync:")
	if count != 1 {
		t.Errorf("expected exactly 1 autoSync: line, got %d:\n%s", count, data)
	}
}

// resolveAutoSync tests

func TestResolveAutoSync_FlagYes(t *testing.T) {
	cfg := &config.Config{}
	result, err := resolveAutoSync(cfg, "yes", bytes.NewReader(nil), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !*result {
		t.Error("expected true for flag=yes")
	}
}

func TestResolveAutoSync_FlagNo(t *testing.T) {
	cfg := &config.Config{}
	result, err := resolveAutoSync(cfg, "no", bytes.NewReader(nil), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || *result {
		t.Error("expected false for flag=no")
	}
}

func TestResolveAutoSync_AlreadyAnsweredTrue(t *testing.T) {
	cfg := &config.Config{AutoSync: boolPtr(true)}
	result, err := resolveAutoSync(cfg, "", bytes.NewReader(nil), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Error("expected nil when already answered")
	}
}

func TestResolveAutoSync_AlreadyAnsweredFalse(t *testing.T) {
	cfg := &config.Config{AutoSync: boolPtr(false)}
	result, err := resolveAutoSync(cfg, "", bytes.NewReader(nil), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Error("expected nil when already answered false")
	}
}

func TestResolveAutoSync_NonTTYNoFlag(t *testing.T) {
	cfg := &config.Config{} // AutoSync nil = first run
	// bytes.Reader is not *os.File, so no TTY prompt fires
	result, err := resolveAutoSync(cfg, "", strings.NewReader("y\n"), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Error("expected nil for non-TTY without flag")
	}
}

// promptAutoSync tests

func TestPromptAutoSync_Yes(t *testing.T) {
	out := &bytes.Buffer{}
	answered, err := promptAutoSync(strings.NewReader("y\n"), out)
	if err != nil {
		t.Fatal(err)
	}
	if !answered {
		t.Error("expected true for input 'y'")
	}
}

func TestPromptAutoSync_YesUppercase(t *testing.T) {
	out := &bytes.Buffer{}
	answered, err := promptAutoSync(strings.NewReader("Y\n"), out)
	if err != nil {
		t.Fatal(err)
	}
	if !answered {
		t.Error("expected true for input 'Y'")
	}
}

func TestPromptAutoSync_No(t *testing.T) {
	out := &bytes.Buffer{}
	answered, err := promptAutoSync(strings.NewReader("n\n"), out)
	if err != nil {
		t.Fatal(err)
	}
	if answered {
		t.Error("expected false for input 'n'")
	}
}

func TestPromptAutoSync_Empty(t *testing.T) {
	out := &bytes.Buffer{}
	answered, err := promptAutoSync(strings.NewReader("\n"), out)
	if err != nil {
		t.Fatal(err)
	}
	if answered {
		t.Error("expected false (default N) for empty input")
	}
}

// writeAutoSyncSpec tests

func TestWriteAutoSyncSpec_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	if err := writeAutoSyncSpec(dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "auto-sync.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "agnostic-ai sync") {
		t.Error("spec file missing sync instruction")
	}
}

func TestWriteAutoSyncSpec_Idempotent(t *testing.T) {
	dir := t.TempDir()
	// Write custom content first
	custom := "custom content"
	if err := os.WriteFile(filepath.Join(dir, "auto-sync.md"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	// Should not overwrite existing file
	if err := writeAutoSyncSpec(dir); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "auto-sync.md"))
	if string(data) != custom {
		t.Error("writeAutoSyncSpec overwrote existing file")
	}
}

// handleAutoSync integration tests

func TestHandleAutoSync_FlagYesWritesSpec(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)

	out := &bytes.Buffer{}
	if err := handleAutoSync(".", "yes", bytes.NewReader(nil), out); err != nil {
		t.Fatal(err)
	}

	specPath := filepath.Join(dir, "rules", "auto-sync.md")
	if _, err := os.Stat(specPath); err != nil {
		t.Error("auto-sync spec not written to rules dir")
	}

	cfgData, _ := os.ReadFile(filepath.Join(dir, "agnostic.config.yaml"))
	if !strings.Contains(string(cfgData), "autoSync: true") {
		t.Error("config not updated with autoSync: true")
	}
}

func TestHandleAutoSync_FlagNoSkipsSpec(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)

	out := &bytes.Buffer{}
	if err := handleAutoSync(".", "no", bytes.NewReader(nil), out); err != nil {
		t.Fatal(err)
	}

	specPath := filepath.Join(dir, "rules", "auto-sync.md")
	if _, err := os.Stat(specPath); err == nil {
		t.Error("auto-sync spec should not be written for flag=no")
	}

	cfgData, _ := os.ReadFile(filepath.Join(dir, "agnostic.config.yaml"))
	if !strings.Contains(string(cfgData), "autoSync: false") {
		t.Error("config not updated with autoSync: false")
	}
}

func TestHandleAutoSync_BadFlag(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	if err := handleAutoSync(".", "maybe", bytes.NewReader(nil), &bytes.Buffer{}); err == nil {
		t.Error("expected error for invalid --auto-sync value")
	}
}

// sync command integration

func TestSync_AutoSyncFlagYes(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--auto-sync=yes"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "rules", "auto-sync.md")); err != nil {
		t.Error("auto-sync spec not created via sync --auto-sync=yes")
	}
}

func TestSync_AutoSyncFlagNo(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--auto-sync=no"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "rules", "auto-sync.md")); err == nil {
		t.Error("auto-sync spec should not exist for --auto-sync=no")
	}
}

func TestSync_AutoSyncFlagBad(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--auto-sync=maybe"})
	if err := root.Execute(); err == nil {
		t.Error("expected error for invalid --auto-sync value")
	}
}

func TestSync_DryRunSkipsAutoSync(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--dry-run", "--auto-sync=yes"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	// dry-run: spec must not be written
	if _, err := os.Stat(filepath.Join(dir, "rules", "auto-sync.md")); err == nil {
		t.Error("auto-sync spec should not be written in --dry-run mode")
	}
}
