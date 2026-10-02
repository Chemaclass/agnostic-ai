package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// fakePATH points PATH at a temp dir holding one empty executable per
// name, so the PATH tier sees exactly those CLIs.
func fakePATH(t *testing.T, bins ...string) {
	t.Helper()
	dir := t.TempDir()
	for _, bin := range bins {
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		if err := os.WriteFile(filepath.Join(dir, bin), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
}

func TestInitDefaultTargets_ExistingTargetsWinOverPATH(t *testing.T) {
	dir := t.TempDir()
	fakePATH(t, "gemini")
	if err := os.MkdirAll(filepath.Join(dir, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, kind := initDefaultTargets(dir)
	if want := []string{"cursor"}; !slices.Equal(got, want) || kind != "detected" {
		t.Errorf("got %v (%s), want %v (detected)", got, kind, want)
	}
}

func TestInitDefaultTargets_CLIsOnPATHWhenNothingDetected(t *testing.T) {
	fakePATH(t, "gemini", "claude", "not-an-ai-cli")
	got, kind := initDefaultTargets(t.TempDir())
	if want := []string{"claude", "gemini"}; !slices.Equal(got, want) || kind != "installed" {
		t.Errorf("got %v (%s), want %v (installed)", got, kind, want)
	}
}

func TestInitDefaultTargets_ClaudeAndCodexWhenNothingFound(t *testing.T) {
	fakePATH(t)
	got, kind := initDefaultTargets(t.TempDir())
	if want := []string{"claude", "codex"}; !slices.Equal(got, want) || kind != "default" {
		t.Errorf("got %v (%s), want %v (default)", got, kind, want)
	}
}

func TestInitCmd_NoTTYNoPipe_EnablesCLIsOnPATH(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	fakePATH(t, "opencode", "aider")

	stderr := &bytes.Buffer{}
	root := NewRootCmd("test")
	root.SetIn(devNullStdin(t))
	root.SetErr(stderr)
	root.SetArgs([]string{"init"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	want, _ := initDefaultTargets(dir)
	if got := configuredTargets(t, dir); !slices.Equal(got, want) || !slices.Equal(got, []string{"aider", "opencode"}) {
		t.Errorf("non-terminal init must enable the picker default\ngot  %v\nwant %v", got, want)
	}
	if msg := "enabled 2 installed targets: aider, opencode"; !strings.Contains(stderr.String(), msg) {
		t.Errorf("stderr missing %q:\n%s", msg, stderr.String())
	}
}

func TestNewTargetPicker_ShowsKeyHintAndPreticksDefaults(t *testing.T) {
	picked := []string{"claude", "codex"}
	picker := newTargetPicker(&picked)
	if view := picker.View(); !strings.Contains(view, "space to toggle, enter to confirm") {
		t.Errorf("picker must name its keys:\n%s", view)
	}
	if got, ok := picker.GetValue().([]string); !ok || !slices.Equal(got, []string{"claude", "codex"}) {
		t.Errorf("picker must start with the defaults ticked, got %v", picker.GetValue())
	}
}

func TestOfferExistingImport_TerminalImportsAllOnYes(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	var asked []string
	got, err := offerExistingImport(dir, true, func(sources []string) (bool, error) {
		asked = sources
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "all" {
		t.Errorf("yes must import as --from all, got %q", got)
	}
	if !slices.Equal(asked, []string{"claude"}) {
		t.Errorf("the offer must name the detected config, got %v", asked)
	}
}

func TestOfferExistingImport_TerminalSkipsOnNo(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := offerExistingImport(dir, true, func([]string) (bool, error) { return false, nil })
	if err != nil || got != "" {
		t.Errorf("no must skip the import, got %q, %v", got, err)
	}
}

func TestOfferExistingImport_OffersARootAgentsFile(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "AGENTS.md"), "# Guidelines\n")
	var asked []string
	if _, err := offerExistingImport(dir, true, func(sources []string) (bool, error) {
		asked = sources
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(asked, []string{"AGENTS.md"}) {
		t.Errorf("a root AGENTS.md is config --from all folds in, got %v", asked)
	}
}

func TestOfferExistingImport_NeverAsksWithoutConfigOrTerminal(t *testing.T) {
	withConfig := t.TempDir()
	if err := os.MkdirAll(filepath.Join(withConfig, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		dir      string
		terminal bool
	}{
		"no config":   {t.TempDir(), true},
		"no terminal": {withConfig, false},
	} {
		got, err := offerExistingImport(tc.dir, tc.terminal, func([]string) (bool, error) {
			return false, errors.New("must not ask")
		})
		if err != nil || got != "" {
			t.Errorf("%s: got %q, %v", name, got, err)
		}
	}
}

func TestInitCmd_NoTTY_DoesNotImportExistingConfig(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	mustWrite(t, filepath.Join(dir, ".claude", "agents", "helper.md"), "---\nname: helper\ndescription: Helps.\n---\n\nHelp.\n")

	root := NewRootCmd("test")
	root.SetIn(devNullStdin(t))
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"init"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "agents", "helper.md")); !os.IsNotExist(err) {
		t.Errorf("non-terminal init must not import existing config: %v", err)
	}
}
