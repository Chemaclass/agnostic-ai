package antigravity

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

func swapNoteWarner(t *testing.T) *strings.Builder {
	t.Helper()
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	return buf
}

// A rule with no activation fields writes `trigger: always_on` as the
// file's first bytes. antigravity.google/docs/rules: "Every .md file
// inside rules/ must start with YAML frontmatter declaring a valid
// trigger" -- a bare file used to be this adapter's always-on shape,
// but the vendor's own caution note now says that gets the rule
// silently discarded, frontmatter and all (#1113).
func TestEmit_Rule_Bare_TriggersAlwaysOn(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{{Kind: spec.KindRule, Name: "house", Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, ".agents", "rules", "house.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(raw), "---\ntrigger: always_on\n---\n\n") {
		t.Errorf("expected trigger: always_on frontmatter as the first bytes, got:\n%s", raw)
	}
	if emit.PendingCoverageNotesCount() != 0 {
		t.Errorf("a plain always-on rule must not buffer a coverage note, got: %s", buf.String())
	}
}

// `alwaysApply` decides first and wins outright, matching windsurf's
// `activationFrontmatter` (#628) and cursor's `mdc()` ("An
// alwaysApply:true rule ignores globs entirely", #443): `globs` set
// alongside `alwaysApply: true` still writes `trigger: always_on`, with
// no `globs:` key at all. This is the exact shape `new rule` seeds
// (`globs: "**/*"` and `alwaysApply: true` together,
// docs/site/content/docs/spec-format.md#rules); turning it into
// `trigger: glob` would activate the rule only when the agent touches a
// matching file, not on every turn (#1113).
func TestEmit_Rule_GlobsWithAlwaysApplyTrue_StaysAlwaysOn(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "seeded", Body: "body", Meta: map[string]any{
			"globs":       "**/*",
			"alwaysApply": true,
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, ".agents", "rules", "seeded.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if !strings.Contains(got, "trigger: always_on\n") {
		t.Errorf("expected trigger: always_on for a seeded rule, got:\n%s", got)
	}
	if strings.Contains(got, "globs:") {
		t.Errorf("an always_on rule must not carry a globs key, got:\n%s", got)
	}
	if emit.PendingCoverageNotesCount() != 0 {
		t.Errorf("must not buffer a coverage note, got: %s", buf.String())
	}
}

// `globs` alone, with no explicit `alwaysApply: false`, also stays
// `always_on`: an unset `alwaysApply` behaves like `true` (windsurf's
// `!ok || always` check and cursor's `always := true` default both
// treat a missing key that way), so this adapter does too.
func TestEmit_Rule_GlobsAlone_StaysAlwaysOn(t *testing.T) {
	dir := testutil.TempCwd(t)
	swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "py-style", Body: "body", Meta: map[string]any{
			"globs": "**/*.py",
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, ".agents", "rules", "py-style.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if !strings.Contains(got, "trigger: always_on\n") {
		t.Errorf("expected trigger: always_on, got:\n%s", got)
	}
	if strings.Contains(got, "globs:") {
		t.Errorf("an always_on rule must not carry a globs key, got:\n%s", got)
	}
}

// `alwaysApply: false` with `globs` writes `trigger: glob` and carries
// the globs value verbatim, quoted per the vendor's own worked example
// ("Wrap patterns starting with `*` in quotes so YAML does not treat
// `*` as an alias anchor"). No coverage note fires (#1113).
func TestEmit_Rule_AlwaysApplyFalseWithGlobs_TriggersGlob(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "py-style", Body: "body", Meta: map[string]any{
			"globs":       "**/*.py",
			"alwaysApply": false,
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, ".agents", "rules", "py-style.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if !strings.Contains(got, "trigger: glob\n") {
		t.Errorf("expected trigger: glob, got:\n%s", got)
	}
	if !strings.Contains(got, `globs: '**/*.py'`) {
		t.Errorf("expected the globs pattern quoted (YAML needs it: a leading `*` reads as an alias anchor otherwise), got:\n%s", got)
	}
	if emit.PendingCoverageNotesCount() != 0 {
		t.Errorf("a rule with globs must not buffer a coverage note, got: %s", buf.String())
	}
}

// A `globs` list joins into the comma-separated string the vendor's
// table documents: "Comma-separated file glob patterns (for example,
// `"*.py, *_test.py"`)".
func TestEmit_Rule_GlobsList_CommaJoins(t *testing.T) {
	dir := testutil.TempCwd(t)
	swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "ts-style", Body: "body", Meta: map[string]any{
			"globs":       []any{"*.ts", "*.tsx"},
			"alwaysApply": false,
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, ".agents", "rules", "ts-style.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if !strings.Contains(got, `globs: '*.ts, *.tsx'`) {
		t.Errorf("expected the globs list comma-joined and quoted, got:\n%s", got)
	}
}

// `alwaysApply: false` with a `description` and no `globs` writes
// `trigger: model_decision` plus the description, the mode the vendor
// says injects "only the rule's path and description upfront".
func TestEmit_Rule_AlwaysApplyFalseWithDescription_TriggersModelDecision(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "migrations", Body: "body", Meta: map[string]any{
			"alwaysApply": false,
			"description": "Apply when writing database migrations.",
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, ".agents", "rules", "migrations.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if !strings.Contains(got, "trigger: model_decision\n") {
		t.Errorf("expected trigger: model_decision, got:\n%s", got)
	}
	if !strings.Contains(got, "description: Apply when writing database migrations.\n") {
		t.Errorf("expected the description carried through, got:\n%s", got)
	}
	if emit.PendingCoverageNotesCount() != 0 {
		t.Errorf("a model_decision rule with a description must not buffer a coverage note, got: %s", buf.String())
	}
}

// `alwaysApply: false` with neither `globs` nor `description` writes
// `trigger: manual`, the vendor's "load only on an @-mention" mode,
// which needs no companion field. This is the same bug #628 fixed for
// windsurf: a rule that opted out of always-on must not get promoted
// back to it for lack of a description, so there is no fallback and no
// coverage note (#1113).
func TestEmit_Rule_AlwaysApplyFalseNoDescriptionNoGlobs_TriggersManual(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "no-desc", Body: "body", Meta: map[string]any{
			"alwaysApply": false,
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, ".agents", "rules", "no-desc.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "trigger: manual\n") {
		t.Errorf("expected trigger: manual, got:\n%s", raw)
	}
	if emit.PendingCoverageNotesCount() != 0 {
		t.Errorf("must not buffer a coverage note, got: %s", buf.String())
	}
}

// The regression this PR fixes (#1117 review): a rule imported back
// from a native `trigger: manual` file that also carries a
// `description` (the vendor recommends one on every rule) must render
// as `manual` again, not `model_decision`. Without honoring the
// `x-antigravity.trigger` override import now writes
// (normalizeAntigravityRuleMeta), the generic fields alone
// (`alwaysApply: false` + a description, no globs) would re-derive
// `model_decision`, silently turning an explicit @-mention-only rule
// into an automatically-loaded one.
func TestEmit_Rule_XAntigravityTriggerManual_OverridesModelDecisionDerivation(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "checklist", Body: "body", Meta: map[string]any{
			"alwaysApply":   false,
			"description":   "Release checklist, read on demand.",
			"x-antigravity": map[string]any{"trigger": "manual"},
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, ".agents", "rules", "checklist.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if !strings.Contains(got, "trigger: manual\n") {
		t.Errorf("expected the override to keep trigger: manual, got:\n%s", got)
	}
	if strings.Contains(got, "model_decision") {
		t.Errorf("must not silently broaden manual to model_decision, got:\n%s", got)
	}
	if !strings.Contains(got, "description: Release checklist, read on demand.\n") {
		t.Errorf("expected the description to still carry through, got:\n%s", got)
	}
	if emit.PendingCoverageNotesCount() != 0 {
		t.Errorf("a valid override must not buffer a coverage note, got: %s", buf.String())
	}
}

// The override wins even against a generic `alwaysApply: true`, which
// alone would resolve to `always_on`: an explicit `x-antigravity`
// trigger states the author's intent more precisely than the generic
// fields can, the same precedence `x-<target>` blocks get everywhere
// else in this adapter family.
func TestEmit_Rule_XAntigravityTrigger_OverridesGenericAlwaysApply(t *testing.T) {
	dir := testutil.TempCwd(t)
	swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "scoped", Body: "body", Meta: map[string]any{
			"alwaysApply":   true,
			"globs":         "**/*.py",
			"x-antigravity": map[string]any{"trigger": "glob"},
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, ".agents", "rules", "scoped.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if !strings.Contains(got, "trigger: glob\n") {
		t.Errorf("expected the override to win over alwaysApply: true, got:\n%s", got)
	}
	if !strings.Contains(got, `globs: '**/*.py'`) {
		t.Errorf("expected globs to still carry through under the glob override, got:\n%s", got)
	}
}

// An `x-antigravity.trigger` naming a value outside
// always_on/glob/model_decision/manual falls to `manual`, the narrowest
// mode, and buffers a coverage note -- never `always_on`, Antigravity's
// most active mode, which a naive re-derivation from the generic fields
// alone would produce (#1117 review).
func TestEmit_Rule_XAntigravityTriggerUnknown_FallsToManualWithNote(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "typo", Body: "body", Meta: map[string]any{
			"x-antigravity": map[string]any{"trigger": "alwaysOn"},
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, ".agents", "rules", "typo.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if !strings.Contains(got, "trigger: manual\n") {
		t.Errorf("expected the unknown override to fall to manual, got:\n%s", got)
	}
	if strings.Contains(got, "trigger: always_on") {
		t.Errorf("an unrecognized override must never fall to always_on, got:\n%s", got)
	}

	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "`trigger` on 1 rule has no effect on antigravity") {
		t.Errorf("expected a coverage note for the unrecognized override, got: %s", buf.String())
	}
}

// `x-antigravity.trigger: glob` with no `globs` in hand cannot honor
// the override (the vendor requires `globs` for that trigger), so it
// falls to `manual` with a coverage note, the same treatment an
// unrecognized value gets.
func TestEmit_Rule_XAntigravityTriggerGlob_WithoutGlobs_FallsToManualWithNote(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "no-globs", Body: "body", Meta: map[string]any{
			"x-antigravity": map[string]any{"trigger": "glob"},
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, ".agents", "rules", "no-globs.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "trigger: manual\n") {
		t.Errorf("expected the unmet-requirement override to fall to manual, got:\n%s", raw)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "`trigger` on 1 rule has no effect on antigravity") {
		t.Errorf("expected a coverage note, got: %s", buf.String())
	}
}

// An always-on rule (alwaysApply unset, or explicitly true) loses
// nothing and buffers no note.
func TestEmit_Rule_AlwaysOnNotesNothing(t *testing.T) {
	testutil.TempCwd(t)
	swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "house", Body: "body"},
		{Kind: spec.KindRule, Name: "always", Body: "body", Meta: map[string]any{"alwaysApply": true}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if n := emit.PendingCoverageNotesCount(); n != 0 {
		t.Errorf("expected no coverage note for always-on rules, got %d", n)
	}
}

// `x-antigravity` overrides the generic frontmatter, so a rule whose
// override clears its globs falls through to `manual` instead of
// `glob`, proving the override actually lands before ruleTrigger runs.
func TestEmit_Rule_TargetOverrideClearsGlobs(t *testing.T) {
	dir := testutil.TempCwd(t)
	swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "py-style", Body: "body", Meta: map[string]any{
			"globs":          "**/*.py",
			"alwaysApply":    false,
			"x-antigravity":  map[string]any{"globs": ""},
			"someOtherThing": 1,
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if n := emit.PendingCoverageNotesCount(); n != 0 {
		t.Errorf("expected no coverage note when the target override clears globs, got %d", n)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".agents", "rules", "py-style.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "trigger: manual\n") {
		t.Errorf("expected the cleared globs to fall through to trigger: manual, got:\n%s", raw)
	}
}

// "Rules files are limited to 12,000 characters each"
// (antigravity.google/docs/rules). The vendor does not say whether it
// truncates or rejects past that, so the rule still emits; what must
// not happen is emitting over the cap in silence (#896).
func TestEmit_Rule_OverCharacterCapNotesSurfaceGap(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "huge", Body: strings.Repeat("x", 12001)},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()

	note := buf.String()
	for _, want := range []string{"antigravity", "12,000 characters", "rules loader"} {
		if !strings.Contains(note, want) {
			t.Errorf("expected the cap note to mention %q, got: %s", want, note)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents", "rules", "huge.md")); err != nil {
		t.Errorf("an over-cap rule must still emit: %v", err)
	}
}

// The cap is measured on the file that lands -- frontmatter,
// provenance header, and heading included -- not on the spec body, so
// a body comfortably under 12,000 that this adapter's own preamble
// pushes over the line still reports.
func TestEmit_Rule_CapCountsTheProvenanceHeader(t *testing.T) {
	testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	body := strings.Repeat("x", 11980)
	entry := spec.Entry{Kind: spec.KindRule, Name: "edge", Body: body}
	if utf8Len := len([]rune(rule(entry))); utf8Len <= 12000 {
		t.Fatalf("fixture no longer straddles the cap (emitted file is %d runes); adjust the body length", utf8Len)
	}
	entries := []spec.Entry{entry}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "12,000 characters") {
		t.Errorf("a body under the cap whose emitted file is over must report: %s", buf.String())
	}
}

// A rule comfortably under the cap stays quiet.
func TestEmit_Rule_UnderCharacterCapIsSilent(t *testing.T) {
	testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "small", Body: strings.Repeat("x", 100)},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if strings.Contains(buf.String(), "12,000 characters") {
		t.Errorf("a rule under the cap must not report: %s", buf.String())
	}
}
