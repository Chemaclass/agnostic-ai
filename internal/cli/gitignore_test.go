package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestReplaceManagedBlock_AppendsToExisting(t *testing.T) {
	got := replaceManagedBlock("node_modules/\nbuild/\n", []string{"CLAUDE.md", ".claude/"})
	if !strings.Contains(got, "node_modules/") {
		t.Errorf("preserved lines lost: %q", got)
	}
	if !strings.Contains(got, gitignoreBlockStart) || !strings.Contains(got, gitignoreBlockEnd) {
		t.Errorf("missing markers: %q", got)
	}
	if !strings.Contains(got, "CLAUDE.md\n") {
		t.Errorf("missing entry: %q", got)
	}
}

func TestReplaceManagedBlock_ReplacesExistingBlock(t *testing.T) {
	first := replaceManagedBlock("", []string{"CLAUDE.md"})
	second := replaceManagedBlock(first, []string{"AGENTS.md", "GEMINI.md"})
	if strings.Contains(second, "CLAUDE.md") {
		t.Errorf("old entry leaked: %q", second)
	}
	for _, want := range []string{"AGENTS.md", "GEMINI.md"} {
		if !strings.Contains(second, want+"\n") {
			t.Errorf("missing %q: %q", want, second)
		}
	}
	if strings.Count(second, gitignoreBlockStart) != 1 {
		t.Errorf("expected exactly one block, got %q", second)
	}
}

func TestReplaceManagedBlock_EmptyEntriesRemovesBlock(t *testing.T) {
	withBlock := replaceManagedBlock("node_modules/\n", []string{"CLAUDE.md"})
	got := replaceManagedBlock(withBlock, nil)
	if strings.Contains(got, gitignoreBlockStart) {
		t.Errorf("expected block removed: %q", got)
	}
	if !strings.Contains(got, "node_modules/") {
		t.Errorf("preserved lines lost: %q", got)
	}
}

func TestReplaceManagedBlock_EmptyFileEmptyEntriesNoop(t *testing.T) {
	if got := replaceManagedBlock("", nil); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestReplaceManagedBlock_TruncatedBlockReplaced(t *testing.T) {
	// Start marker present, end marker absent: simulate a corrupted
	// .gitignore. The replacement should rewrite a fresh block; trailing
	// junk after the start marker is treated as part of the truncated
	// block and consumed.
	corrupt := "node_modules/\n" + gitignoreBlockStart + "\nold-entry.md\n"
	got := replaceManagedBlock(corrupt, []string{"CLAUDE.md"})
	if !strings.Contains(got, "node_modules/") {
		t.Errorf("preserved lines lost: %q", got)
	}
	if strings.Contains(got, "old-entry.md") {
		t.Errorf("truncated content leaked: %q", got)
	}
	if !strings.Contains(got, gitignoreBlockEnd) {
		t.Errorf("expected end marker rewritten: %q", got)
	}
	if strings.Count(got, gitignoreBlockStart) != 1 {
		t.Errorf("expected exactly one block, got %q", got)
	}
}

func TestUpdateGitignore_CreatesFileWhenMissing(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	cfg := &config.Config{Gitignore: config.Gitignore{Enabled: true}}
	if err := updateGitignore(cfg, []string{"CLAUDE.md"}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "CLAUDE.md") {
		t.Errorf("missing entry: %s", got)
	}
}

func TestUpdateGitignore_RespectsCustomPath(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	cfg := &config.Config{Gitignore: config.Gitignore{Enabled: true, Path: ".gitignore.local"}}
	if err := updateGitignore(cfg, []string{"X"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore.local")); err != nil {
		t.Errorf("expected custom path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); !os.IsNotExist(err) {
		t.Errorf("expected default path untouched, err=%v", err)
	}
}

func TestUpdateGitignore_NoopWhenContentMatches(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	cfg := &config.Config{Gitignore: config.Gitignore{Enabled: true}}
	if err := updateGitignore(cfg, []string{"CLAUDE.md"}); err != nil {
		t.Fatal(err)
	}
	first, err := os.Stat(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	// Run again with the same entries; mtime should not change.
	if err := updateGitignore(cfg, []string{"CLAUDE.md"}); err != nil {
		t.Fatal(err)
	}
	second, err := os.Stat(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !first.ModTime().Equal(second.ModTime()) {
		t.Errorf("expected no rewrite when content unchanged")
	}
}

func TestNormalizeGitignorePath(t *testing.T) {
	cases := map[string]string{
		"./CLAUDE.md":         "CLAUDE.md",
		".claude/agents/x.md": ".claude/agents/x.md",
	}
	for in, want := range cases {
		if got := normalizeGitignorePath(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeAndSort_DedupesAndSorts(t *testing.T) {
	got := normalizeAndSort([]string{
		"./b.md",
		"a.md",
		"b.md",
		"./b.md",
	})
	want := []string{"a.md", "b.md"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("index %d: got %q, want %q", i, got[i], w)
		}
	}
}

func TestResolveGitignore_FlagOverridesConfig(t *testing.T) {
	cfg := &config.Config{Gitignore: config.Gitignore{Enabled: true}}
	if resolveGitignore(cfg, "off") {
		t.Error("flag off should win over config on")
	}
	if !resolveGitignore(cfg, "on") {
		t.Error("flag on should be honored")
	}
	if !resolveGitignore(cfg, "") {
		t.Error("no flag should defer to config (true)")
	}
	cfg.Gitignore.Enabled = false
	if resolveGitignore(cfg, "") {
		t.Error("no flag should defer to config (false)")
	}
}

func TestValidateGitignoreFlag(t *testing.T) {
	for _, ok := range []string{"", "on", "off"} {
		if err := validateGitignoreFlag(ok); err != nil {
			t.Errorf("expected %q to validate, got %v", ok, err)
		}
	}
	for _, bad := range []string{"true", "false", "yes", "no", "1", "0", "maybe"} {
		if err := validateGitignoreFlag(bad); err == nil {
			t.Errorf("expected %q to error", bad)
		}
	}
}
