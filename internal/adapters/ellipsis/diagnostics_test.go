package ellipsis

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Each case is a settings block Ellipsis could not run, with the words the
// diagnostic must use so the author knows which key to fix.
var brokenSettings = []struct {
	name string
	x    map[string]any
	want string
}{
	{"no x-ellipsis block", nil, "x-ellipsis.reviewer"},
	{"no reviewer", map[string]any{"budget": map[string]any{"session": 3}}, "x-ellipsis.reviewer"},
	{"reviewer is not a mapping", map[string]any{"reviewer": "bugs"}, "x-ellipsis.reviewer must be a mapping"},
	{"no name", map[string]any{"reviewer": map[string]any{"claude_code": map[string]any{"model": "claude-opus-5-5"}}}, "x-ellipsis.reviewer.name"},
	{"no engine", map[string]any{"reviewer": map[string]any{"name": "r"}}, "claude_code, codex, or gemini"},
	{"two engines", map[string]any{"reviewer": map[string]any{"name": "r",
		"claude_code": map[string]any{"model": "claude-opus-5-5"}, "codex": map[string]any{"model": "gpt-6-sol"}}}, "only one"},
	{"two engines with gemini", map[string]any{"reviewer": map[string]any{"name": "r",
		"codex": map[string]any{"model": "gpt-6-sol"}, "gemini": map[string]any{"model": "gemini-4-argon"}}}, "only one"},
	{"no gemini model", map[string]any{"reviewer": map[string]any{"name": "r", "gemini": map[string]any{}}}, "x-ellipsis.reviewer.gemini.model"},
	{"no model", map[string]any{"reviewer": map[string]any{"name": "r", "claude_code": map[string]any{}}}, "x-ellipsis.reviewer.claude_code.model"},
	{"prompt in settings", map[string]any{"reviewer": map[string]any{"name": "r",
		"claude_code": map[string]any{"model": "claude-opus-5-5", "prompt": "Find bugs."}}}, "prompt comes from the review specs"},
	{"unknown top-level key", map[string]any{"reviewer": reviewer(nil), "triggers": map[string]any{}}, "x-ellipsis.triggers"},
	{"wrong kind", map[string]any{"reviewer": reviewer(nil), "ellipsis": map[string]any{"kind": "agent"}}, "code_review"},
	{"description is not a string", map[string]any{"reviewer": reviewer(nil), "ellipsis": map[string]any{"description": 3}}, "x-ellipsis.ellipsis.description must be a string"},
	{"enabled is not a boolean", map[string]any{"reviewer": reviewer(nil), "ellipsis": map[string]any{"enabled": "no"}}, "x-ellipsis.ellipsis.enabled must be a boolean"},
	{"metadata is not a mapping", map[string]any{"reviewer": reviewer(nil), "ellipsis": map[string]any{"metadata": "x"}}, "x-ellipsis.ellipsis.metadata must be a mapping"},
	{"unknown ellipsis key", map[string]any{"reviewer": reviewer(nil), "ellipsis": map[string]any{"owner": "x"}}, "x-ellipsis.ellipsis.owner"},
	{"output is not a mapping", map[string]any{"reviewer": reviewer(nil), "output": "approve"}, "x-ellipsis.output must be a mapping"},
}

func TestEmit_BrokenReviewerConfigNotesAndWritesNoFile(t *testing.T) {
	for _, tc := range brokenSettings {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			buf := captureNotes(t)
			entries := []spec.Entry{review("r", "", "Check.")}
			if tc.x != nil {
				entries = append(entries, settings(tc.x))
			}
			if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
				t.Fatalf("warn mode must not fail: %v", err)
			}
			emit.FlushCoverageNotes()
			if paths := testutil.WalkRel(t, dir); len(paths) != 0 {
				t.Errorf("no runnable file may be written, got %v", paths)
			}
			out := buf.String()
			if !strings.Contains(out, "ellipsis") || !strings.Contains(out, tc.want) {
				t.Errorf("note should name the target and %q, got:\n%s", tc.want, out)
			}
		})
	}
}

func TestEmit_BrokenReviewerConfigFailsUnderOnUnsupportedError(t *testing.T) {
	for _, tc := range brokenSettings {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			captureNotes(t)
			entries := []spec.Entry{review("r", "", "Check.")}
			if tc.x != nil {
				entries = append(entries, settings(tc.x))
			}
			err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{OnUnsupported: "error"}, false)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error naming %q, got %v", tc.want, err)
			}
			if paths := testutil.WalkRel(t, dir); len(paths) != 0 {
				t.Errorf("no file may be written on error, got %v", paths)
			}
		})
	}
}

func TestEmit_BrokenReviewerConfigIsQuietUnderOnUnsupportedSilent(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := captureNotes(t)
	b := spec.NewBundle([]spec.Entry{review("r", "", "Check.")})
	if err := New().Emit(emit.NewSession(), b, &config.Config{OnUnsupported: "silent"}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if paths := testutil.WalkRel(t, dir); len(paths) != 0 || buf.Len() != 0 {
		t.Errorf("silent mode: files %v, output %q", paths, buf)
	}
}

func TestEmit_ReportsEveryProblemAtOnce(t *testing.T) {
	testutil.TempCwd(t)
	buf := captureNotes(t)
	x := map[string]any{"reviewer": map[string]any{"claude_code": map[string]any{}}, "triggers": 1}
	b := spec.NewBundle([]spec.Entry{review("r", "", "Check."), settings(x)})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	for _, want := range []string{"x-ellipsis.reviewer.name", "claude_code.model", "x-ellipsis.triggers"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %q in:\n%s", want, buf)
		}
	}
}

func TestEmit_UnsupportedKindsFollowOnUnsupported(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)
	rule := spec.Entry{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule"}
	b := spec.NewBundle([]spec.Entry{rule, review("r", "", "Check."), settings(map[string]any{"reviewer": reviewer(nil)})})
	if err := New().Emit(emit.NewSession(), b, &config.Config{OnUnsupported: "warn"}, false); err != nil {
		t.Fatal(err)
	}
	if got := emit.PendingCapabilityWarningsCount(); got != 1 {
		t.Errorf("want one capability warning (rule), got %d", got)
	}
	err := New().Emit(emit.NewSession(), b, &config.Config{OnUnsupported: "error"}, false)
	if err == nil || !strings.Contains(err.Error(), "ellipsis") {
		t.Errorf("on-unsupported: error must fail for a rule, got %v", err)
	}
}

func TestEmit_PortableSettingsFieldsAreNotedAsIgnored(t *testing.T) {
	testutil.TempCwd(t)
	buf := captureNotes(t)
	portable := spec.Entry{Kind: spec.KindSettings, Name: "defaults", Meta: map[string]any{
		"model":       "claude-opus-5-5",
		"permissions": map[string]any{"allow": []any{"read(src/**)"}},
	}}
	b := spec.NewBundle([]spec.Entry{review("r", "", "Check."), settings(map[string]any{"reviewer": reviewer(nil)}), portable})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	for _, want := range []string{"model", "permissions"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("a portable %s must be noted as having no effect:\n%s", want, buf)
		}
	}
	if got := readReview(t, reviewFile); strings.Contains(got, "read(src") {
		t.Errorf("portable permissions leaked into the review file:\n%s", got)
	}
}

func TestEmit_ProblemsRewriteTheGeneratedFileUnchanged(t *testing.T) {
	testutil.TempCwd(t)
	captureNotes(t)
	good := []spec.Entry{review("r", "", "Check."), settings(map[string]any{"reviewer": reviewer(nil)})}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(good), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	before := readReview(t, reviewFile)
	bad := []spec.Entry{review("r", "", "Changed."), settings(map[string]any{"reviewer": map[string]any{"name": "project-review", "claude_code": map[string]any{}}})}
	sess := emit.NewSession()
	sess.StartCapture()
	if err := New().Emit(sess, spec.NewBundle(bad), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	files := sess.StopCapture()
	if len(files) != 1 || filepath.ToSlash(files[0].Path) != reviewFile || files[0].Content != before {
		t.Errorf("a reviewer with problems must keep the deployed file as it is, got %+v", files)
	}
}

func TestEmit_NoReviewerKeyLeavesTheFileToTheSweep(t *testing.T) {
	testutil.TempCwd(t)
	captureNotes(t)
	good := []spec.Entry{review("r", "", "Check."), settings(map[string]any{"reviewer": reviewer(nil)})}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(good), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	sess := emit.NewSession()
	sess.StartCapture()
	if err := New().Emit(sess, spec.NewBundle([]spec.Entry{review("r", "", "Check.")}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if files := sess.StopCapture(); len(files) != 0 {
		t.Errorf("without a reviewer key nothing may be kept, got %+v", files)
	}
}
