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

func TestReplaceManagedBlock_HeaderCarriesRegenHint(t *testing.T) {
	got := replaceManagedBlock("", []string{"CLAUDE.md"})
	if !strings.Contains(got, gitignoreBlockHint) {
		t.Errorf("managed block header missing regen hint:\n%s", got)
	}
	// The hint sits on its own comment line between the note and the
	// first ignored path, so a reader staring at .gitignore learns the
	// outputs need `sync` to reappear.
	want := gitignoreBlockNote + "\n" + gitignoreBlockHint + "\n"
	if !strings.Contains(got, want) {
		t.Errorf("hint not on its own line after the note:\n%s", got)
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
	if err := updateGitignore(".", cfg, []string{"CLAUDE.md"}); err != nil {
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

func TestBuildManagedBlock_IncludesFixedEntries(t *testing.T) {
	block := buildManagedBlock(&config.Config{}, nil)
	for _, want := range []string{
		"/agnostic-ai.local.yaml",
		"/.agnostic-ai/.sync-state",
		"/.agnostic-ai/packs/",
	} {
		found := false
		for _, e := range block {
			if e == want {
				found = true
			}
		}
		if !found {
			t.Errorf("managed block missing fixed entry %q, got %v", want, block)
		}
	}
}

func TestUpdateGitignore_StripsLooseFixedDuplicatesAndConsolidates(t *testing.T) {
	dir := t.TempDir()
	legacy := "node_modules/\n" +
		"agnostic-ai.local.yaml\n" +
		".agnostic-ai/.sync-state\n" +
		".agnostic-ai/packs/\n\n" +
		gitignoreBlockStart + "\n" + gitignoreBlockNote + "\n" +
		"/.agnostic-ai/.sync-state\n/.claude/\n/CLAUDE.md\n" +
		gitignoreBlockEnd + "\n"
	path := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Gitignore: config.Gitignore{Enabled: true}}
	block := buildManagedBlock(cfg, []string{".claude/settings.json", "CLAUDE.md"})
	if err := updateGitignore(dir, cfg, block); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := string(got)

	if !strings.Contains(out, "node_modules/") {
		t.Errorf("dropped unmanaged line:\n%s", out)
	}
	for _, bare := range []string{"\nagnostic-ai.local.yaml\n", "\n.agnostic-ai/.sync-state\n", "\n.agnostic-ai/packs/\n"} {
		if strings.Contains(out, bare) {
			t.Errorf("loose duplicate %q survived:\n%s", strings.TrimSpace(bare), out)
		}
	}
	if n := strings.Count(out, ".agnostic-ai/.sync-state"); n != 1 {
		t.Errorf("expected one .sync-state entry, got %d:\n%s", n, out)
	}
	for _, want := range []string{"/agnostic-ai.local.yaml", "/.agnostic-ai/.sync-state", "/.agnostic-ai/packs/"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing anchored fixed entry %q:\n%s", want, out)
		}
	}
}

func TestEnsureManagedGitignore_CreatesWhenAbsentNoopWhenPresent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")

	if err := ensureManagedGitignore(dir); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first), "/.agnostic-ai/packs/") {
		t.Errorf("expected packs entry after create:\n%s", first)
	}

	enriched := strings.Replace(string(first), gitignoreBlockEnd, "/.claude/\n"+gitignoreBlockEnd, 1)
	if err := os.WriteFile(path, []byte(enriched), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureManagedGitignore(dir); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != enriched {
		t.Errorf("ensureManagedGitignore clobbered an existing block:\nwant:\n%s\ngot:\n%s", enriched, after)
	}
}

func TestUpdateGitignore_RespectsCustomPath(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	cfg := &config.Config{Gitignore: config.Gitignore{Enabled: true, Path: ".gitignore.local"}}
	if err := updateGitignore(".", cfg, []string{"X"}); err != nil {
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
	if err := updateGitignore(".", cfg, []string{"CLAUDE.md"}); err != nil {
		t.Fatal(err)
	}
	first, err := os.Stat(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	// Run again with the same entries; mtime should not change.
	if err := updateGitignore(".", cfg, []string{"CLAUDE.md"}); err != nil {
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

func TestCollapseManagedEntries_FoldsOutputSubdirsKeepsRootAndSources(t *testing.T) {
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
		"/.claude/CLAUDE.md",        // file under tool dir: kept precise
		"/.claude/README.md",        // file under tool dir: kept precise
		"/.claude/skills/",          // output subdir: collapsed
		"/.codex/agents/",           // output subdir: collapsed
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

// Regression for #414: collapsing stops at the generated subdirectory so a
// hand-authored file living directly under the same tool dir is never
// swallowed by a `/.claude/` ignore.
func TestCollapseManagedEntries_DoesNotSwallowHandAuthoredSiblings(t *testing.T) {
	// Only the generated rules subdir is emitted; settings.json is
	// hand-authored and never appears in the entry list.
	got := collapseManagedEntries([]string{"/.claude/rules/auth.md"}, nil)
	want := []string{"/.claude/rules/"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("got %v, want %v", got, want)
	}
	for _, e := range got {
		if e == "/.claude/" {
			t.Fatalf("whole tool dir ignored, would hide hand-authored siblings: %v", got)
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

func TestNormalizeAllowEntries_PrefixesDedupesSorts(t *testing.T) {
	got := normalizeAllowEntries([]string{
		"internal/adapters/**/testdata/**",
		"!**/AGENTS.md",        // user-supplied bang is tolerated, not doubled
		"./fixtures/CLAUDE.md", // leading ./ stripped
		"  ",                   // blank dropped
		"**/AGENTS.md",         // duplicate of the bang form above
	})
	want := []string{
		"!**/AGENTS.md",
		"!fixtures/CLAUDE.md",
		"!internal/adapters/**/testdata/**",
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

// The managed block lists collapsed ignores first, then re-allow lines so a
// fixture tree stays tracked even when enabled (#388).
func TestBuildManagedBlock_AppendsAllowExceptionsLast(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gitignore.Allow = []string{"internal/adapters/**/testdata/**"}
	got := buildManagedBlock(cfg, []string{".claude/CLAUDE.md", "AGENTS.md"})
	if len(got) == 0 || got[len(got)-1] != "!internal/adapters/**/testdata/**" {
		t.Fatalf("re-allow line not appended last: %v", got)
	}
	for _, e := range got[:len(got)-1] {
		if strings.HasPrefix(e, "!") {
			t.Errorf("re-allow line %q appeared before ignores: %v", e, got)
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
