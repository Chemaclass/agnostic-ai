package windsurf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_WritesRulesAgentsAndSkills(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	a := New()
	if a.Name() != "windsurf" {
		t.Errorf("expected windsurf, got %s", a.Name())
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule"},
		{Kind: spec.KindAgent, Name: "ag1", Body: "agent"},
		{Kind: spec.KindSkill, Name: "sk1", Body: "skill"},
	}
	if err := a.Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{".devin/rules/r1.md", ".devin/agents/ag1.md", ".agents/skills/sk1/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("missing %s", p)
		}
	}
	// The pre-fix flat form must not be written; Devin Desktop only
	// discovers skills under .agents/skills/<name>/SKILL.md.
	if _, err := os.Stat(filepath.Join(dir, ".devin/rules/skill-sk1.md")); !os.IsNotExist(err) {
		t.Errorf("expected no flat .devin/rules/skill-sk1.md, err=%v", err)
	}
}

// TestEmit_SkillsDirOverride_WritesToCustomDir confirms
// outputs.windsurf.skills-dir redirects the folder-per-skill output,
// consistent with every other emit.OutputSkillsDir consumer.
func TestEmit_SkillsDirOverride_WritesToCustomDir(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"windsurf": {SkillsDir: "custom/skills"},
		},
	}
	entries := []spec.Entry{{Kind: spec.KindSkill, Name: "sk1", Body: "skill body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/skills/sk1/SKILL.md")); err != nil {
		t.Errorf("expected custom/skills/sk1/SKILL.md: %v", err)
	}
}

// TestSkillMarkdown_MatchesSharedTreeTargets asserts that the shared
// SKILL.md renderer (emit.SkillMarkdown), which every adapter writing
// into the `.agents/skills/` tree calls, produces byte-identical output
// for windsurf, codex, amp, zed, crush, and openhands given the same
// entry. The `target` argument only changes output when the entry
// carries a `x-<target>` override; a plain entry (no such keys) must
// render the same regardless of which of those targets asks, or the
// shared-tree dedupe in `sync.shared-skills` would silently stop
// working the moment two targets' bytes drift.
//
// This test deliberately calls emit.SkillMarkdown directly rather than
// importing another adapter package: no-cross-adapter-imports forbids
// one adapter package importing another, even in tests.
func TestSkillMarkdown_MatchesSharedTreeTargets(t *testing.T) {
	entry := spec.Entry{
		Kind: spec.KindSkill,
		Name: "uno",
		Meta: map[string]any{"description": "Uno skill description."},
		Body: "uno skill body",
	}

	want := emit.SkillMarkdown(entry, "windsurf")
	for _, other := range []string{"codex", "amp", "zed", "crush", "openhands"} {
		if got := emit.SkillMarkdown(entry, other); got != want {
			t.Errorf("SkillMarkdown(entry, %q) diverged from windsurf:\n--- windsurf ---\n%s\n--- %s ---\n%s",
				other, want, other, got)
		}
	}
}

// TestKitSink_SkillOutputMatchesAmpGolden confirms the windsurf SKILL.md
// render is byte-identical to the checked-in amp golden fixture for the
// same kit-sink skill entries, so the on-disk shared-tree dedupe
// (sync.shared-skills) actually finds the two trees identical rather
// than relying solely on the in-memory renderer check above. Reads
// amp's testdata directly off disk (a fixture read, not a Go import)
// so this stays within no-cross-adapter-imports.
func TestKitSink_SkillOutputMatchesAmpGolden(t *testing.T) {
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	ampSkillsDir := filepath.Join(origCwd, "..", "amp", "testdata", "kitsink", ".agents", "skills")
	if _, err := os.Stat(ampSkillsDir); err != nil {
		t.Skipf("amp golden fixtures not found at %s: %v", ampSkillsDir, err)
	}

	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	for _, name := range []string{"uno", "dos", "tres"} {
		gotPath := filepath.Join(dir, ".agents", "skills", name, "SKILL.md")
		got, err := os.ReadFile(gotPath)
		if err != nil {
			t.Fatalf("read windsurf output %s: %v", gotPath, err)
		}
		wantPath := filepath.Join(ampSkillsDir, name, "SKILL.md")
		want, err := os.ReadFile(wantPath)
		if err != nil {
			t.Fatalf("read amp golden %s: %v", wantPath, err)
		}
		if string(got) != string(want) {
			t.Errorf("windsurf SKILL.md for %q diverged from amp golden:\n--- amp (%s) ---\n%s\n--- windsurf ---\n%s",
				name, wantPath, want, got)
		}
	}
}

func TestEmit_WorkflowsDirEmitsAgentsAsWorkflows(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent,
			Name: "ship-it",
			Meta: map[string]any{"description": "open and merge a PR"},
			Body: "Run the release.",
		},
		{Kind: spec.KindRule, Name: "r1", Body: "rule body"},
	}
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"windsurf": {WorkflowsDir: ".windsurf/workflows"},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}

	wfPath := filepath.Join(dir, ".windsurf/workflows/ship-it.md")
	got, err := os.ReadFile(wfPath)
	if err != nil {
		t.Fatalf("missing workflow: %v", err)
	}
	body := string(got)
	if !strings.Contains(body, "description: open and merge a PR") {
		t.Errorf("workflow missing description frontmatter: %q", body)
	}
	if !strings.Contains(body, "Run the release.") {
		t.Errorf("workflow missing body: %q", body)
	}

	// The native subagent file emits either way.
	if _, err := os.Stat(filepath.Join(dir, ".devin/agents/ship-it.md")); err != nil {
		t.Errorf("native agent file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".devin/rules/r1.md")); err != nil {
		t.Errorf("rule missing: %v", err)
	}
}

func TestEmit_RulesCarryProvenanceHeader(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule body"},
		{Kind: spec.KindAgent, Name: "ag1", Body: "agent body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{".devin/rules/r1.md", ".devin/agents/ag1.md"} {
		got, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if !strings.Contains(string(got), "Generated by agnostic-ai") {
			t.Errorf("%s missing provenance header:\n%s", p, got)
		}
	}
}

func TestEmit_NoWorkflowsDirNoEmit(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "ag1", Body: "agent"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".windsurf/workflows")); !os.IsNotExist(err) {
		t.Errorf("expected no workflows dir; err=%v", err)
	}
}

// outputs.windsurf.rules-dir keeps the pre-rename layout; Devin Desktop
// still reads .windsurf/rules/ as a backward-compat fallback.
func TestEmit_RulesDirOverride_KeepsLegacyWindsurfTree(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"windsurf": {RulesDir: ".windsurf/rules"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".windsurf/rules/r1.md")); err != nil {
		t.Errorf("override should keep the legacy tree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".devin/rules/r1.md")); !os.IsNotExist(err) {
		t.Errorf("override must not also write the new default, err=%v", err)
	}
}

// .devinignore is the vendor-documented current ignore-file path
// (docs.devin.ai/desktop/context-awareness/windsurf-ignore); the legacy
// `.codeiumignore` and `.windsurfignore` filenames stay reader-side
// only, so this adapter never writes either.
func TestEmit_IgnoreFile_WritesDevinignore(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{{Kind: spec.KindIgnore, Name: "secrets", Body: "*.env\nsecrets/"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".devinignore"))
	if err != nil {
		t.Fatalf("missing .devinignore: %v", err)
	}
	body := string(got)
	if !strings.Contains(body, "*.env") || !strings.Contains(body, "secrets/") {
		t.Errorf("missing ignore patterns, got:\n%s", body)
	}
	if !strings.HasPrefix(body, "#") {
		t.Errorf("expected shell-style (#) provenance header, got:\n%s", body)
	}
}

func TestEmit_IgnoreFileOverride(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"windsurf": {IgnoreFile: ".codeiumignore"}},
	}
	entries := []spec.Entry{{Kind: spec.KindIgnore, Name: "secrets", Body: "*.env"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codeiumignore")); err != nil {
		t.Errorf("expected override path written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".devinignore")); !os.IsNotExist(err) {
		t.Errorf("override must not also write the default, err=%v", err)
	}
}

// Managed leftovers at the pre-rename .windsurf/rules path are swept on
// sync (they predate the ledger on old projects); hand-authored files
// there survive.
func TestEmit_SweepsLegacyWindsurfTree(t *testing.T) {
	dir := testutil.TempCwd(t)

	legacy := filepath.Join(dir, ".windsurf", "rules")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "old.md"),
		[]byte("<!-- Generated by agnostic-ai -->\n\nold managed rule\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "mine.md"),
		[]byte("hand-authored, no marker\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(legacy, "old.md")); !os.IsNotExist(err) {
		t.Errorf("managed legacy file should be swept, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(legacy, "mine.md")); err != nil {
		t.Errorf("hand-authored legacy file must survive: %v", err)
	}
}

// TestEmit_ScopedRulesLandInASubdirectoryRulesDir pins the routing
// fixed in #628. Devin discovers `.devin/rules` directories anywhere in
// the workspace tree, never a subdirectory inside one, so a scoped rule
// belongs at `<scope>/.devin/rules/<name>.md`.
func TestEmit_ScopedRulesLandInASubdirectoryRulesDir(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "auth", Path: "rules/backend/auth.md", Scope: "backend", Body: "auth body"},
		{Kind: spec.KindRule, Name: "limits", Path: "rules/backend/api/limits.md", Scope: "backend/api", Body: "limits body"},
		{Kind: spec.KindAgent, Name: "delta", Path: "agents/backend/delta.md", Scope: "backend", Body: "delta body"},
		{Kind: spec.KindRule, Name: "root", Path: "rules/root.md", Body: "root body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{
		"backend/.devin/rules/auth.md",
		"backend/api/.devin/rules/limits.md",
		// A scoped agent stays flat: Devin documents sub-directory
		// discovery for rules only.
		".devin/agents/delta.md",
		".devin/rules/root.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}
	for _, p := range []string{
		".devin/rules/backend/auth.md",
		".devin/rules/backend/api/limits.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, p)); !os.IsNotExist(err) {
			t.Errorf("unreachable nested path still written: %s (err=%v)", p, err)
		}
	}
}

// TestEmit_ScopedRulesFollowTheRulesDirOverride keeps the scope prefix
// attached to whatever rules dir the user configured, so the legacy
// `.windsurf/rules` layout scopes the same way.
func TestEmit_ScopedRulesFollowTheRulesDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"windsurf": {RulesDir: ".windsurf/rules"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "auth", Path: "rules/backend/auth.md", Scope: "backend", Body: "auth body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "backend/.windsurf/rules/auth.md")); err != nil {
		t.Errorf("missing backend/.windsurf/rules/auth.md: %v", err)
	}
}

// TestEmit_ScopeEscapingRootFallsBackToTheRulesDir covers a frontmatter
// `scope: ../x`. Prefixing it to the rules dir would write outside the
// project, and dropping the rule is the bug #628 fixes, so it lands
// unscoped instead.
func TestEmit_ScopeEscapingRootFallsBackToTheRulesDir(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "auth", Path: "rules/auth.md", Body: "auth body",
			Meta: map[string]any{"scope": "../outside"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".devin/rules/auth.md")); err != nil {
		t.Errorf("expected the escaping scope to fall back to .devin/rules/auth.md: %v", err)
	}
}

// TestEmit_RuleActivationBecomesTriggerFrontmatter pins the trigger
// mapping. An always-on rule stays bare: Devin treats a rule file with
// no frontmatter as always-on, so writing `trigger: always_on` would
// churn every existing file for no behavior change.
func TestEmit_RuleActivationBecomesTriggerFrontmatter(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "always", Path: "rules/always.md", Body: "always body"},
		{Kind: spec.KindRule, Name: "globbed", Path: "rules/globbed.md", Body: "globbed body",
			Meta: map[string]any{"alwaysApply": false, "globs": "**/*.test.ts", "description": "Test conventions"}},
		{Kind: spec.KindRule, Name: "decided", Path: "rules/decided.md", Body: "decided body",
			Meta: map[string]any{"alwaysApply": false, "description": "API design"}},
		{Kind: spec.KindRule, Name: "manual", Path: "rules/manual.md", Body: "manual body",
			Meta: map[string]any{"alwaysApply": false}},
		{Kind: spec.KindRule, Name: "override", Path: "rules/override.md", Body: "override body",
			Meta: map[string]any{"alwaysApply": true, "x-windsurf": map[string]any{"alwaysApply": false, "globs": "cmd/**"}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		file    string
		want    []string
		notWant []string
	}{
		{"always.md", nil, []string{"trigger:", "---"}},
		{"globbed.md", []string{"trigger: glob", "globs: '**/*.test.ts'"}, nil},
		{"decided.md", []string{"trigger: model_decision", "description: API design"}, []string{"globs:"}},
		{"manual.md", []string{"trigger: manual"}, []string{"description:", "globs:"}},
		{"override.md", []string{"trigger: glob", "globs: cmd/**"}, nil},
	}
	for _, c := range cases {
		data, err := os.ReadFile(filepath.Join(dir, ".devin/rules", c.file))
		if err != nil {
			t.Fatalf("read %s: %v", c.file, err)
		}
		got := string(data)
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s missing %q:\n%s", c.file, w, got)
			}
		}
		for _, w := range c.notWant {
			if strings.Contains(got, w) {
				t.Errorf("%s unexpectedly contains %q:\n%s", c.file, w, got)
			}
		}
	}
}
