package augment

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

func TestName(t *testing.T) {
	if got := New().Name(); got != "augment" {
		t.Errorf("Name() = %q, want %q", got, "augment")
	}
}

// The project-root AGENTS.md (with rule bodies inlined) is written
// centrally by sync; this adapter never writes it. The legacy
// .augment-guidelines document also stays absent without the
// rules-file opt-in. Unlike both of those, the native
// .augment/rules/ directory below is unconditional (see the next
// test): this test only pins the two surfaces that stay opt-in/central.
func TestEmit_NoRootAGENTSMdOrLegacyGuidelinesByDefault(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("adapter should not write AGENTS.md, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".augment-guidelines")); !os.IsNotExist(err) {
		t.Errorf("adapter should not write .augment-guidelines without the rules-file opt-in, err=%v", err)
	}
}

// The native .augment/rules/<name>.md surface (target-audit
// 2026-08-01: "Rules are files that live in the .augment/rules
// directory") is unconditional, unlike the legacy .augment-guidelines
// document. A plain rule (no alwaysApply override, no description)
// renders with no `type` key at all: always_apply is the vendor
// default, so it stays implicit rather than written out on every rule.
func TestEmit_Rule_WritesNativeRulesDirByDefault(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment/rules/r1.md"))
	if !strings.Contains(got, "rule body") {
		t.Errorf("missing rule body in %s", got)
	}
	if strings.Contains(got, "type:") {
		t.Errorf("always_apply is the vendor default and must stay implicit, got:\n%s", got)
	}
	if strings.Contains(got, "name:") {
		t.Errorf("Augment rules have no name key; identity is filename-based, got:\n%s", got)
	}
}

// A description is optional at the always_apply default (the vendor
// only requires one for agent_requested), but still passes through
// when the author sets it.
func TestEmit_Rule_AlwaysApplyKeepsOptionalDescription(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Meta: map[string]any{"description": "Always-on rule."}, Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment/rules/r1.md"))
	if !strings.Contains(got, "description: Always-on rule.") {
		t.Errorf("expected description to pass through, got:\n%s", got)
	}
	if strings.Contains(got, "type:") {
		t.Errorf("always_apply must stay implicit even with a description set, got:\n%s", got)
	}
}

// alwaysApply: false (the same generic field Cursor rules already
// read) switches the rule to Augment's agent_requested type, which the
// vendor requires a description for (WAVE 2 SCHEMAS, target-audit
// 2026-08-01); the fallback is the rule name, matching every other
// adapter's description-fallback convention.
func TestEmit_Rule_AgentRequestedRequiresDescription(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Meta: map[string]any{"alwaysApply": false}, Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment/rules/r1.md"))
	for _, want := range []string{"type: agent_requested", "description: r1"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// An explicit description on an agent_requested rule wins over the
// name fallback.
func TestEmit_Rule_AgentRequestedUsesExplicitDescription(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindRule, Name: "r1",
			Meta: map[string]any{"alwaysApply": false, "description": "Only when relevant."},
			Body: "rule body",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment/rules/r1.md"))
	for _, want := range []string{"type: agent_requested", "description: Only when relevant."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "description: r1\n") {
		t.Errorf("explicit description must win over the name fallback, got:\n%s", got)
	}
}

// x-augment cannot reintroduce name (Augment rules have no such key)
// or Cursor's own globs/alwaysApply spelling, which has no Augment
// meaning.
func TestEmit_Rule_XAugmentCannotReintroduceNameOrCursorFields(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindRule, Name: "r1",
			Meta: map[string]any{
				"x-augment": map[string]any{"name": "override", "globs": "**/*.go", "alwaysApply": true},
			},
			Body: "rule body",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment/rules/r1.md"))
	for _, unwanted := range []string{"name:", "globs:", "alwaysApply:"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("Augment rules must never emit %q, got:\n%s", unwanted, got)
		}
	}
}

// x-augment.type can still deliberately request the IDE-only `manual`
// value the package doc calls out: the escape hatch is not limited to
// the two values this adapter computes by default.
func TestEmit_Rule_XAugmentTypeOverridesComputedValue(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindRule, Name: "r1",
			Meta: map[string]any{"x-augment": map[string]any{"type": "manual"}},
			Body: "rule body",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment/rules/r1.md"))
	if !strings.Contains(got, "type: manual") {
		t.Errorf("expected x-augment.type to override the computed default, got:\n%s", got)
	}
}

func TestEmit_RulesDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{Outputs: map[string]config.Output{"augment": {RulesDir: "custom/rules"}}}
	entries := []spec.Entry{{Kind: spec.KindRule, Name: "r1", Body: "rule body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/rules/r1.md")); err != nil {
		t.Errorf("expected override dir to hold the rule file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".augment/rules/r1.md")); !os.IsNotExist(err) {
		t.Errorf("expected no output at the default rules dir, err=%v", err)
	}
}

// A rule's source-directory scope nests under .augment/rules/, the
// same convention every other RulesDirectory-backed adapter (cursor,
// cline, windsurf, ...) already honors.
func TestEmit_Rule_NestedScopeRoutesUnderSubdir(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "auth", Scope: "backend/api", Body: "rule"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".augment/rules/backend/api/auth.md")); err != nil {
		t.Errorf("expected nested rule under .augment/rules/backend/api: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "backend/api/.augment/rules/auth.md")); !os.IsNotExist(err) {
		t.Errorf("expected no stray scope dir at repo root, err=%v", err)
	}
}

// Agents write to .augment/agents/<name>.md by default: name is
// always present (required), description falls back to the spec name,
// and color/model pass through only when set.
func TestEmit_Agent_WritesAgentFile(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent, Name: "reviewer",
			Meta: map[string]any{"description": "Reviews diffs.", "color": "blue", "model": "sonnet"},
			Body: "Review the diff for correctness.",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment/agents/reviewer.md"))
	if !strings.HasPrefix(got, "---\n") {
		t.Fatalf("frontmatter must be first, got:\n%s", got)
	}
	for _, want := range []string{
		"name: reviewer",
		"description: Reviews diffs.",
		"color: blue",
		"model: sonnet",
		"Review the diff for correctness.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestEmit_Agent_DescriptionFallsBackToName(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "no-desc", Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment/agents/no-desc.md"))
	if !strings.Contains(got, "description: no-desc") {
		t.Errorf("expected description fallback to agent name, got:\n%s", got)
	}
	if strings.Contains(got, "color:") || strings.Contains(got, "model:") {
		t.Errorf("expected no color/model keys when absent from meta, got:\n%s", got)
	}
}

// The generic cross-tool `tools` field uses Claude-style names (Read,
// Grep, Bash); Augment's own vocabulary is different (view,
// codebase-retrieval, ...), and there is no confirmed mapping between
// them (target-audit 2026-08-01, same trap as kilo's B2), so it must
// never reach Augment's frontmatter under either tools or
// disabled_tools.
func TestEmit_Agent_GenericToolsNeverEmitted(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"tools": []any{"Read", "Grep"}}, Body: "body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment/agents/reviewer.md"))
	for _, unwanted := range []string{"tools:", "Read", "Grep"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("generic Claude-style tools must never reach Augment frontmatter, got:\n%s", unwanted+" in "+got)
		}
	}
}

// An agent that declares the generic tools field with no x-augment
// rescue surfaces exactly one coverage note per sync, never a silent
// drop.
func TestEmit_Agent_ToolsSurfacesCoverageNote(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "a1", Meta: map[string]any{"tools": []any{"Read"}}, Body: "body"},
		{Kind: spec.KindAgent, Name: "a2", Body: "body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if n := emit.PendingCoverageNotesCount(); n != 1 {
		t.Errorf("expected one coverage note (only a1 declares tools), got %d", n)
	}
}

// A bundle where no agent sets the generic tools field must not
// surface a coverage note.
func TestEmit_Agent_NoToolsNoCoverageGap(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "a1", Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if n := emit.PendingCoverageNotesCount(); n != 0 {
		t.Errorf("expected no coverage note when no agent sets tools, got %d", n)
	}
}

// x-augment.tools / x-augment.disabled_tools are the one channel this
// adapter trusts to already carry Augment's own tool vocabulary, so
// they pass through verbatim (whether a YAML list or a comma-separated
// string, both vendor-documented) and rescue the agent from the
// coverage note that a bare generic `tools` field would otherwise
// trigger.
func TestEmit_Agent_XAugmentToolsPassesThroughAndSuppressesCoverageNote(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent, Name: "reviewer",
			Meta: map[string]any{
				"tools":     []any{"Read", "Grep"},
				"x-augment": map[string]any{"tools": "view, codebase-retrieval"},
			},
			Body: "body",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment/agents/reviewer.md"))
	if !strings.Contains(got, "tools: view, codebase-retrieval") {
		t.Errorf("expected x-augment.tools to pass through verbatim, got:\n%s", got)
	}
	if strings.Contains(got, "Read") || strings.Contains(got, "Grep") {
		t.Errorf("generic Claude-style tools must not leak through, got:\n%s", got)
	}
	if n := emit.PendingCoverageNotesCount(); n != 0 {
		t.Errorf("an explicit x-augment.tools override rescues the agent; expected no coverage note, got %d", n)
	}
}

func TestEmit_Agent_XAugmentDisabledToolsPassesThrough(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent, Name: "reviewer",
			Meta: map[string]any{"x-augment": map[string]any{"disabled_tools": []any{"remove-files", "launch-process"}}},
			Body: "body",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment/agents/reviewer.md"))
	for _, want := range []string{"disabled_tools:", "remove-files", "launch-process"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected x-augment.disabled_tools to pass through, missing %q in:\n%s", want, got)
		}
	}
}

// x-augment cannot reintroduce name: the frontmatter identity always
// comes from the spec name, keeping it in lockstep with the filename.
func TestEmit_Agent_XAugmentCannotReintroduceName(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent, Name: "reviewer",
			Meta: map[string]any{"x-augment": map[string]any{"name": "override"}},
			Body: "body",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment/agents/reviewer.md"))
	if strings.Contains(got, "name: override") {
		t.Errorf("x-augment must not override the agent's frontmatter name, got:\n%s", got)
	}
	if !strings.Contains(got, "name: reviewer") {
		t.Errorf("expected name to stay the spec name, got:\n%s", got)
	}
}

// Arbitrary x-augment keys beyond the documented schema pass through,
// same convention as every other adapter's escape hatch.
func TestEmit_Agent_XAugmentPassthrough(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent, Name: "reviewer",
			Meta: map[string]any{"x-augment": map[string]any{"auto_mode": true}},
			Body: "body",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment/agents/reviewer.md"))
	if !strings.Contains(got, "auto_mode: true") {
		t.Errorf("expected x-augment key to pass through, got:\n%s", got)
	}
}

func TestEmit_AgentsDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{Outputs: map[string]config.Output{"augment": {AgentsDir: "custom/agents"}}}
	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "a1", Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/agents/a1.md")); err != nil {
		t.Errorf("expected override dir to hold the agent file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".augment/agents/a1.md")); !os.IsNotExist(err) {
		t.Errorf("expected no output at the default agents dir, err=%v", err)
	}
}

// Skills route into the shared .agents/skills/ tree (target-audit
// 2026-08-01), the same tree codex, amp, zed, crush, openhands, and
// windsurf already write byte-identically.
func TestEmit_Skill_WritesSharedSkillsTree(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindSkill, Name: "yaml-validator", Meta: map[string]any{"description": "Validate YAML."}, Body: "Steps."},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".agents/skills/yaml-validator/SKILL.md"))
	for _, want := range []string{"name: yaml-validator", "description: Validate YAML.", "Steps."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestEmit_SkillsDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{Outputs: map[string]config.Output{"augment": {SkillsDir: "custom/skills"}}}
	entries := []spec.Entry{{Kind: spec.KindSkill, Name: "sk1", Body: "skill body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/skills/sk1/SKILL.md")); err != nil {
		t.Errorf("expected custom/skills/sk1/SKILL.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents/skills/sk1/SKILL.md")); !os.IsNotExist(err) {
		t.Errorf("expected no output at the default skills dir, err=%v", err)
	}
}

// TestSkillMarkdown_MatchesSharedTreeTargets asserts that the shared
// SKILL.md renderer (emit.SkillMarkdown), which every adapter writing
// into the .agents/skills/ tree calls, produces byte-identical output
// for augment, codex, amp, zed, crush, openhands, and windsurf given
// the same entry. A plain entry (no x-<target> override) must render
// the same regardless of which of those targets asks, or the
// shared-tree dedupe in sync.shared-skills would silently stop
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

	want := emit.SkillMarkdown(entry, "augment")
	for _, other := range []string{"codex", "amp", "zed", "crush", "openhands", "windsurf"} {
		if got := emit.SkillMarkdown(entry, other); got != want {
			t.Errorf("SkillMarkdown(entry, %q) diverged from augment:\n--- augment ---\n%s\n--- %s ---\n%s",
				other, want, other, got)
		}
	}
}

// TestKitSink_SkillOutputMatchesAmpGolden confirms the augment
// SKILL.md render is byte-identical to the checked-in amp golden
// fixture for the same kit-sink skill entries, so the on-disk
// shared-tree dedupe (sync.shared-skills) actually finds the two trees
// identical rather than relying solely on the in-memory renderer check
// above. Reads amp's testdata directly off disk (a fixture read, not a
// Go import) so this stays within no-cross-adapter-imports.
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
			t.Fatalf("read augment output %s: %v", gotPath, err)
		}
		wantPath := filepath.Join(ampSkillsDir, name, "SKILL.md")
		want, err := os.ReadFile(wantPath)
		if err != nil {
			t.Fatalf("read amp golden %s: %v", wantPath, err)
		}
		if string(got) != string(want) {
			t.Errorf("augment SKILL.md for %q diverged from amp golden:\n--- amp (%s) ---\n%s\n--- augment ---\n%s",
				name, wantPath, want, got)
		}
	}
}

// outputs.augment.rules-file opts into the legacy concatenated
// `.augment-guidelines`-style document.
func TestEmit_RulesFile_WritesConcatenatedRules(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"augment": {RulesFile: ".augment-guidelines"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "security", Path: "rules/security.md", Body: "Never expose secrets."},
		{Kind: spec.KindRule, Name: "commits", Path: "rules/commits.md", Body: "Use conventional commits."},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment-guidelines"))
	for _, want := range []string{"Never expose secrets.", "Use conventional commits."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// Agents and skills have their own native surfaces
// (.augment/agents/, .agents/skills/) and must not also leak into the
// opt-in legacy rules document: only rule bodies belong there.
func TestEmit_RulesFile_ExcludesAgentsAndSkills(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"augment": {RulesFile: ".augment-guidelines"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
		{Kind: spec.KindAgent, Name: "helper", Path: "agents/helper.md", Body: "agent body should not appear"},
		{Kind: spec.KindSkill, Name: "sk1", Path: "skills/sk1/SKILL.md", Body: "skill body should not appear"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment-guidelines"))
	if !strings.Contains(got, "rule body") {
		t.Errorf("missing rule body in %s", got)
	}
	for _, absent := range []string{"agent body should not appear", "skill body should not appear"} {
		if strings.Contains(got, absent) {
			t.Errorf("unexpected %q in %s", absent, got)
		}
	}
}

func TestEmit_EmptyBundle_WritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)

	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected an empty directory, got %v", entries)
	}
}

// The rules-file opt-in is itself a no-op when the bundle carries no
// rules, agents, or skills: MergedDocument short-circuits before
// writing.
func TestEmit_RulesFile_NoRulesWritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"augment": {RulesFile: ".augment-guidelines"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".augment-guidelines")); !os.IsNotExist(err) {
		t.Errorf("expected no .augment-guidelines for a bundle with no rules, err=%v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
