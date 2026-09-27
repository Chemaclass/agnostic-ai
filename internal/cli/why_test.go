package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func setupWhyFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := `version: 1
sources:
  agents: agents
  skills: skills
  rules: rules
  hooks: hooks
  mcps: mcps
targets:
  - claude
  - cursor
`
	mustWrite := func(p, body string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(filepath.Join(dir, "agnostic-ai.yaml"), cfg)
	mustWrite(filepath.Join(dir, "rules", "no-console-log.md"), `---
name: no-console-log
description: No console.log in shipped code.
globs: "**/*"
alwaysApply: true
---

Do not commit console.log statements.
`)
	mustWrite(filepath.Join(dir, "rules", "other.md"), `---
name: other
description: Sibling rule.
---

Sibling body.
`)
	return dir
}

func TestWhy_HumanOutputReportsAdapterAndSource(t *testing.T) {
	dir := setupWhyFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"why", ".cursor/rules/no-console-log.mdc"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, ".cursor/rules/no-console-log.mdc") {
		t.Errorf("missing file header: %s", got)
	}
	if !strings.Contains(got, "adapter: cursor") {
		t.Errorf("missing adapter line: %s", got)
	}
	if !strings.Contains(got, "no-console-log") {
		t.Errorf("missing source name: %s", got)
	}
	if !strings.Contains(got, "(rules/no-console-log.md)") {
		t.Errorf("missing source path: %s", got)
	}
}

func TestWhy_UnknownFileEmitsActionableError(t *testing.T) {
	dir := setupWhyFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	// Pre-create the sync state so the error path is "not tracked", not
	// "no sync state".
	if err := os.MkdirAll(filepath.Join(dir, ".agnostic-ai"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agnostic-ai", ".sync-state"),
		[]byte(`{"synced_at":"2026-01-01T00:00:00Z","files_changed":1}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"why", "some/random/path.md"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for untracked file")
	}
	if !strings.Contains(err.Error(), "not synced or not tracked") {
		t.Errorf("expected not-tracked message, got %v", err)
	}
}

func TestWhy_MissingStateFileSuggestsSync(t *testing.T) {
	dir := setupWhyFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	// No .sync-state present; an unknown path should surface the
	// "no sync state" hint.
	root := NewRootCmd("test")
	root.SetArgs([]string{"why", "some/random/path.md"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when sync state missing")
	}
	if !strings.Contains(err.Error(), "no sync state found") {
		t.Errorf("expected no-sync-state message, got %v", err)
	}
	if !strings.Contains(err.Error(), "agnostic-ai sync") {
		t.Errorf("expected sync hint in error, got %v", err)
	}
}

func TestWhy_JSONOutputSchema(t *testing.T) {
	dir := setupWhyFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	if err := os.MkdirAll(filepath.Join(dir, ".agnostic-ai"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agnostic-ai", ".sync-state"),
		[]byte(`{"synced_at":"2026-01-01T00:00:00Z","files_changed":1}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got := runWhyJSON(t, ".cursor/rules/no-console-log.mdc")
	if got.Version != "1" {
		t.Errorf("version: want 1, got %q", got.Version)
	}
	if got.Command != "why" {
		t.Errorf("command: want why, got %q", got.Command)
	}
	if got.Target != "cursor" {
		t.Errorf("target: want cursor, got %q", got.Target)
	}
	if got.File == "" {
		t.Errorf("file: missing")
	}
	if len(got.Sources) == 0 {
		t.Errorf("expected at least one source")
	}
	if got.LastSync == nil {
		t.Errorf("expected last_sync present")
	}
	if got.OutputKeys == nil {
		t.Errorf("output_keys must be a JSON array, not null")
	}
}

func TestWhy_OutputKeysReportConfiguredOverrides(t *testing.T) {
	dir := setupWhyFixture(t)
	// Override the cursor rules-dir so the path uses a custom segment.
	cfg := `version: 1
sources:
  rules: rules
targets:
  - cursor
outputs:
  cursor:
    rules-dir: custom-rules
`
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
	silence(t)

	got := runWhyJSON(t, "custom-rules/no-console-log.mdc")
	found := false
	for _, k := range got.OutputKeys {
		if k == "outputs.cursor.rules-dir" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected outputs.cursor.rules-dir in keys, got %v", got.OutputKeys)
	}
}

// why on an entry-point file (AGENTS.md) credits the rule specs inlined
// into it, even though no adapter's Emit writes that file.
func TestWhy_EntryPointFileCreditsInlinedRules(t *testing.T) {
	dir := setupWhyFixture(t)
	cfg := `version: 1
sources:
  rules: rules
targets:
  - codex
`
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
	silence(t)

	got := runWhyJSON(t, "AGENTS.md")
	if got.Target != "codex" {
		t.Errorf("expected codex as the entry-point consumer, got %q", got.Target)
	}
	names := map[string]bool{}
	for _, s := range got.Sources {
		names[s.Name] = true
		if s.Mode != "section" {
			t.Errorf("inlined rule must be a section source, got %q for %s", s.Mode, s.Name)
		}
	}
	for _, want := range []string{"no-console-log", "other"} {
		if !names[want] {
			t.Errorf("expected inlined rule %q in sources, got %v", want, got.Sources)
		}
	}
}

// TestWhy_RootAGENTSMdNotConfusedWithJunieMirror guards against a
// regression the junie adapter's own `.junie/AGENTS.md` entry-point
// mirror could reintroduce: findEmittingAdapter runs every registered
// adapter's capture output (regardless of the project's `targets:`
// list), and `.junie/AGENTS.md` shares a basename with the root
// `AGENTS.md` this test queries. The basename-only fallback must not
// let that unrelated nested file outrank the dedicated entry-point
// resolution for an exact top-level query.
func TestWhy_RootAGENTSMdNotConfusedWithJunieMirror(t *testing.T) {
	dir := setupWhyFixture(t)
	cfg := `version: 1
sources:
  rules: rules
targets:
  - codex
`
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
	silence(t)

	got := runWhyJSON(t, "AGENTS.md")
	if got.Target != "codex" {
		t.Errorf("root AGENTS.md must resolve to codex (the inlined-rules entry-point consumer), not junie's unrelated .junie/AGENTS.md mirror; got %q", got.Target)
	}
}

// TestWhy_JunieAGENTSMdMirrorResolvesToJunie confirms the exact-path
// query for junie's own entry-point mirror still resolves correctly:
// the fix for the collision above must not make `.junie/AGENTS.md`
// itself untraceable.
func TestWhy_JunieAGENTSMdMirrorResolvesToJunie(t *testing.T) {
	dir := setupWhyFixture(t)
	cfg := `version: 1
sources:
  rules: rules
targets:
  - junie
`
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
	silence(t)

	got := runWhyJSON(t, ".junie/AGENTS.md")
	if got.Target != "junie" {
		t.Errorf("expected junie as the .junie/AGENTS.md emitter, got %q", got.Target)
	}
}

// runWhyJSON runs `why <file> --format json` in the current directory and
// decodes the report.
func runWhyJSON(t *testing.T, file string) whyOutput {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"why", file, "--format", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got whyOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	return got
}

// chdirThroughSymlink links a fresh path to dir and enters the project
// through that link, with PWD set so the working directory keeps the
// symlinked form, the way a shell reports it.
func chdirThroughSymlink(t *testing.T, dir string) {
	t.Helper()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	testutil.Chdir(t, link)
	t.Setenv("PWD", link)
}

func TestWhy_SymlinkedProjectMatchesExistingFileExactly(t *testing.T) {
	dir := setupWhyFixture(t)
	emitted := filepath.Join(dir, ".cursor", "rules", "no-console-log.mdc")
	if err := os.MkdirAll(filepath.Dir(emitted), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(emitted, []byte("emitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdirThroughSymlink(t, dir)
	silence(t)

	got := runWhyJSON(t, ".cursor/rules/no-console-log.mdc")
	if got.File != ".cursor/rules/no-console-log.mdc" {
		t.Errorf("file: want project-relative path, got %q", got.File)
	}
	if got.Target != "cursor" {
		t.Errorf("target: want cursor, got %q", got.Target)
	}
}

func TestWhy_SymlinkedProjectResolvesMissingFile(t *testing.T) {
	dir := setupWhyFixture(t)
	if err := os.MkdirAll(filepath.Join(dir, ".claude", "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	chdirThroughSymlink(t, dir)
	silence(t)

	got := runWhyJSON(t, ".claude/rules/no-console-log.md")
	if got.File != ".claude/rules/no-console-log.md" {
		t.Errorf("file: want project-relative path, got %q", got.File)
	}
	if got.Target != "claude" {
		t.Errorf("target: want claude, got %q", got.Target)
	}
}

func TestWhy_PrefersConfiguredTargetForSharedPath(t *testing.T) {
	dir := setupWhyFixture(t)
	cfg := `version: 1
sources:
  skills: skills
targets:
  - codex
`
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "skills", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "demo", "SKILL.md"),
		[]byte("---\nname: demo\ndescription: Demo skill.\n---\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
	silence(t)

	got := runWhyJSON(t, ".agents/skills/demo/SKILL.md")
	if got.Target != "codex" {
		t.Errorf("target: want configured codex, got %q", got.Target)
	}
	if !got.Configured {
		t.Errorf("configured: want true for codex")
	}
}

func TestWhy_LabelsUnconfiguredTarget(t *testing.T) {
	dir := setupWhyFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	got := runWhyJSON(t, ".devin/rules/no-console-log.md")
	if got.Target != "windsurf" {
		t.Fatalf("target: want windsurf, got %q", got.Target)
	}
	if got.Configured {
		t.Errorf("configured: want false for a target missing from targets")
	}

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"why", ".devin/rules/no-console-log.md"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "adapter: windsurf (not configured)") {
		t.Errorf("missing not-configured label: %s", out.String())
	}
}

// CLAUDE.md is the file users open first. Claude reads rules from its own
// directory, so nothing is inlined, but the file still comes from the
// shared instructions.
func TestWhy_EntryPointFileCreditsTheSharedInstructions(t *testing.T) {
	dir := setupWhyFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	got := runWhyJSON(t, "CLAUDE.md")

	if got.Target != "claude" {
		t.Errorf("expected claude as the entry-point consumer, got %q", got.Target)
	}
	if len(got.Sources) != 1 || got.Sources[0].Path != ".agnostic-ai/AGNOSTIC_AI.md" || got.Sources[0].Mode != "full" {
		t.Errorf("want AGNOSTIC_AI.md as the one full source, got %v", got.Sources)
	}
}

func TestWhy_EntryPointWithInlinedRulesListsInstructionsFirst(t *testing.T) {
	dir := setupWhyFixture(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\nsources:\n  rules: rules\ntargets:\n  - codex\n")
	testutil.Chdir(t, dir)
	silence(t)

	got := runWhyJSON(t, "AGENTS.md")

	if len(got.Sources) == 0 || got.Sources[0].Path != ".agnostic-ai/AGNOSTIC_AI.md" {
		t.Errorf("want AGNOSTIC_AI.md first, got %v", got.Sources)
	}
}

// A source spec is not an output; say which command answers the question.
func TestWhy_SourceFilePointsAtExplain(t *testing.T) {
	dir := setupWhyFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"why", "rules/no-console-log.md"})
	err := root.Execute()

	if err == nil || !strings.Contains(err.Error(), "agnostic-ai explain rules/no-console-log.md") {
		t.Errorf("got %v", err)
	}
}

func TestWhy_SharedInstructionsSayWhereTheyGo(t *testing.T) {
	dir := setupWhyFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"why", ".agnostic-ai/AGNOSTIC_AI.md"})
	err := root.Execute()

	if err == nil || !strings.Contains(err.Error(), "shared instructions body") {
		t.Errorf("got %v", err)
	}
}

func whyProject(t *testing.T, cfg string, files map[string]string) {
	t.Helper()
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), cfg)
	for p, body := range files {
		mustWriteFile(t, filepath.Join(dir, p), body)
	}
	silence(t)
}

func sourceNames(w whyOutput) []string {
	var out []string
	for _, s := range w.Sources {
		out = append(out, s.Name)
	}
	return out
}

// A shared AGENTS.md carries the rules of the reader that inlines them,
// not of the first reader by name.
func TestWhy_SharedEntryPointCreditsTheInliningReader(t *testing.T) {
	whyProject(t, "version: 1\ntargets: [codex, cline]\n", map[string]string{
		".agnostic-ai/rules/codex-only.md": "---\ntargets: [codex]\n---\nCodex only rule.\n",
		".agnostic-ai/rules/cline-only.md": "---\ntargets: [cline]\n---\nCline only rule.\n",
	})

	got := runWhyJSON(t, "AGENTS.md")

	if got.Target != "codex" || !slices.Contains(sourceNames(got), "codex-only") || slices.Contains(sourceNames(got), "cline-only") {
		t.Errorf("got target %q, sources %v", got.Target, sourceNames(got))
	}
}

// rules-mode: import appends @-lines for each rule to CLAUDE.md.
func TestWhy_ImportedRulesAreSectionsOfTheEntryPoint(t *testing.T) {
	whyProject(t, "version: 1\ntargets: [claude]\noutputs:\n  claude:\n    rules-mode: import\n", map[string]string{
		".agnostic-ai/rules/always.md": "---\nalwaysApply: true\n---\nAlways.\n",
	})

	got := runWhyJSON(t, "CLAUDE.md")

	if got.Sources[0].Mode != "section" || !slices.Contains(sourceNames(got), "always") {
		t.Errorf("got %v", got.Sources)
	}
}

func TestWhy_UnmanagedEntryPointIsNotCredited(t *testing.T) {
	whyProject(t, "version: 1\ntargets: [claude]\nsync:\n  unmanaged: [CLAUDE.md]\n", map[string]string{"CLAUDE.md": "hand written\n"})

	root := NewRootCmd("test")
	root.SetArgs([]string{"why", "CLAUDE.md", "--format", "json"})
	var out bytes.Buffer
	root.SetOut(&out)
	_ = root.Execute()

	if strings.Contains(out.String(), "AGNOSTIC_AI.md") {
		t.Errorf("a user-owned CLAUDE.md must not trace to AGNOSTIC_AI.md:\n%s", out.String())
	}
}

func TestWhy_JunieMirrorTracesToTheSharedInstructions(t *testing.T) {
	whyProject(t, "version: 1\ntargets: [junie]\n", nil)

	got := runWhyJSON(t, ".junie/AGENTS.md")

	if len(got.Sources) == 0 || got.Sources[0].Path != ".agnostic-ai/AGNOSTIC_AI.md" {
		t.Errorf("got %v", got.Sources)
	}
}

func TestWhy_BlankLocalExtensionIsNotASource(t *testing.T) {
	whyProject(t, "version: 1\ntargets: [gemini]\n", map[string]string{".agnostic-ai/local/AGNOSTIC_AI.md": "   \n"})

	got := runWhyJSON(t, "GEMINI.md")

	if len(got.Sources) != 1 || got.Sources[0].Mode != "full" {
		t.Errorf("got %v", got.Sources)
	}
}

func TestWhy_OutputThatIsAlsoASourceStillTraces(t *testing.T) {
	whyProject(t, "version: 1\nsources:\n  rules: .claude/rules\ntargets: [claude]\n", map[string]string{".claude/rules/r.md": "---\ndescription: R.\n---\nBody.\n"})

	got := runWhyJSON(t, ".claude/rules/r.md")

	if got.Target != "claude" {
		t.Errorf("got %+v", got)
	}
}

func TestWhy_DotSlashEntryPointOverrideMatches(t *testing.T) {
	whyProject(t, "version: 1\ntargets: [gemini]\noutputs:\n  gemini:\n    file: ./docs/GEMINI.md\n", nil)

	got := runWhyJSON(t, "docs/GEMINI.md")

	if got.Target != "gemini" {
		t.Errorf("got %+v", got)
	}
}
