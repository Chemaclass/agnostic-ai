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
		"./CLAUDE.md":         "/CLAUDE.md",
		".claude/agents/x.md": "/.claude/agents/x.md",
		"/AGENTS.md":          "/AGENTS.md",
		".":                   "",
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
	want := []string{"/a.md", "/b.md"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("index %d: got %q, want %q", i, got[i], w)
		}
	}
}

// Regression for #362: managed entries must be root-anchored so an
// unanchored basename (CONVENTIONS.md, .rules, AGENTS.md) cannot match a
// same-named golden fixture nested under internal/adapters/*/testdata/.
func TestNormalizeAndSort_AnchorsEveryEntry(t *testing.T) {
	got := normalizeAndSort([]string{"CONVENTIONS.md", ".rules", ".claude/rules/x.md", "AGENTS.md"})
	for _, e := range got {
		if !strings.HasPrefix(e, "/") {
			t.Errorf("entry %q not root-anchored", e)
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

func TestCollapseManagedEntries_FoldsOutputDirsKeepsRootAndSources(t *testing.T) {
	in := []string{
		"/.agnostic-ai/.sync-state",
		"/.claude/CLAUDE.md",
		"/.claude/README.md",
		"/.claude/skills/test/SKILL.md",
		"/.codex/agents/x.md",
		"/AGENTS.md",
	}
	got := collapseManagedEntries(in, []string{".agnostic-ai"})
	want := []string{
		"/.agnostic-ai/.sync-state", // protected source dir: kept precise
		"/.claude/",                 // output dir: collapsed
		"/.codex/",                  // output dir: collapsed
		"/AGENTS.md",                // root file: kept
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %q want %q", i, got[i], want[i])
		}
	}
}

func TestCollapseManagedEntries_NeverIgnoresSourceTree(t *testing.T) {
	// A spec living under .agnostic-ai must never be collapsed into
	// `/.agnostic-ai/`, which would ignore committed sources.
	in := []string{"/.agnostic-ai/.sync-state", "/.agnostic-ai/agents/a.md"}
	got := collapseManagedEntries(in, []string{".agnostic-ai"})
	for _, e := range got {
		if e == "/.agnostic-ai/" {
			t.Fatalf("source dir was collapsed: %v", got)
		}
	}
}

func TestProtectedSourceTopDirs_IncludesLayerAndSources(t *testing.T) {
	cfg := &config.Config{}
	cfg.Sources.Agents = ".agnostic-ai/agents"
	cfg.Sources.Skills = "specs/skills"
	got := protectedSourceTopDirs(cfg)
	found := map[string]bool{}
	for _, d := range got {
		found[d] = true
	}
	for _, want := range []string{".agnostic-ai", "specs"} {
		if !found[want] {
			t.Errorf("missing protected dir %q in %v", want, got)
		}
	}
}
