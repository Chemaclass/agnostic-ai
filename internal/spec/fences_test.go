package spec

import (
	"reflect"
	"testing"
)

// A body with no `::target` markers passes through unchanged for any
// reader set, matching BodyFor's own fast path.
func TestFilterFences_NoFences_Unchanged(t *testing.T) {
	t.Parallel()
	body := "# Test\n\n## Workflow\n\nStep 1.\n"
	if got := FilterFences(body, []string{"codex", "cline"}); got != body {
		t.Errorf("FilterFences mutated an unfenced body:\ngot:  %q\nwant: %q", got, body)
	}
}

// A single-reader call keeps the fence pinned to that reader and drops
// a fence pinned to a different one, same as Entry.BodyFor.
func TestFilterFences_SingleReader_KeepsMatchingDropsOther(t *testing.T) {
	t.Parallel()
	body := "Intro.\n\n::target codex\nCodex stuff.\n::end\n\n::target claude\nClaude stuff.\n::end\n\nOutro.\n"
	got := FilterFences(body, []string{"codex"})
	want := "Intro.\n\nCodex stuff.\n\nOutro.\n"
	if got != want {
		t.Errorf("FilterFences([codex]):\ngot:  %q\nwant: %q", got, want)
	}
}

// `::targets <a> <b>` matches when any reader is in the allow-list.
func TestFilterFences_TargetsOpener_MatchesAnyListed(t *testing.T) {
	t.Parallel()
	body := "::targets codex amp\nShared paragraph.\n::end\n"
	got := FilterFences(body, []string{"amp"})
	want := "Shared paragraph.\n"
	if got != want {
		t.Errorf("FilterFences([amp]):\ngot:  %q\nwant: %q", got, want)
	}
}

// A path shared by several targets (e.g. AGENTS.md read by codex and
// cline) keeps a fenced block when any one of its readers is listed.
func TestFilterFences_SharedPath_AnyReaderKeepsBlock(t *testing.T) {
	t.Parallel()
	readers := []string{"codex", "cline"}

	kept := FilterFences("::target cline\nCline-relevant.\n::end\n", readers)
	if kept != "Cline-relevant.\n" {
		t.Errorf("fence naming a listed reader must stay:\ngot: %q", kept)
	}

	dropped := FilterFences("::target gemini\nNot for this file.\n::end\n", readers)
	if dropped != "" {
		t.Errorf("fence naming no reader must drop:\ngot: %q", dropped)
	}
}

// An empty readers list is the source view: fences stay intact.
func TestFilterFences_NoReaders_PreservesFences(t *testing.T) {
	t.Parallel()
	body := "::target codex\nx\n::end\n"
	if got := FilterFences(body, nil); got != body {
		t.Errorf("empty readers should return raw body, got %q", got)
	}
}

// An unterminated fence runs to end-of-body for a multi-reader call.
func TestFilterFences_UnterminatedFence_RunsToEnd(t *testing.T) {
	t.Parallel()
	body := "Intro.\n\n::target codex\nNo end marker.\n"
	got := FilterFences(body, []string{"codex", "cline"})
	want := "Intro.\n\nNo end marker.\n"
	if got != want {
		t.Errorf("FilterFences:\ngot:  %q\nwant: %q", got, want)
	}
}

// A second opener before ::end replaces the allow-list rather than
// extending it.
func TestFilterFences_OpenerInsideFence_ReplacesAllowList(t *testing.T) {
	t.Parallel()
	body := "::target codex\n::target cline\nOnly cline sees this.\n::end\n"
	if got := FilterFences(body, []string{"codex"}); got != "" {
		t.Errorf("replaced allow-list must drop codex-only readers:\ngot: %q", got)
	}
	if got := FilterFences(body, []string{"cline"}); got != "Only cline sees this.\n" {
		t.Errorf("replaced allow-list must keep cline:\ngot: %q", got)
	}
}

// A dropped fence at the end of the body must not leave the result with
// more trailing newlines than the source had. Before the fix, a kept
// blank connector line ahead of the dropped fence survived alongside
// the trailing newline strings.Split manufactures for the source's own
// final '\n', stacking two newlines where the source had one.
func TestFilterFences_DroppedTrailingFence_NoExtraTrailingNewline(t *testing.T) {
	t.Parallel()
	got := FilterFences("A\n\n::target x\nB\n::end\n", []string{"y"})
	if want := "A\n"; got != want {
		t.Errorf("FilterFences:\ngot:  %q\nwant: %q", got, want)
	}
}

// A dropped fence that opens the body must not leave the blank line that
// followed it as a leading blank line in the result (#790).
func TestFilterFences_DroppedLeadingFence_NoLeadingBlankLine(t *testing.T) {
	t.Parallel()
	got := FilterFences("::target gemini\nG.\n::end\n\nShared.\n", []string{"claude"})
	if want := "Shared.\n"; got != want {
		t.Errorf("FilterFences:\ngot:  %q\nwant: %q", got, want)
	}
}

// Leading blank lines the source itself carries survive: only the ones a
// dropped fence exposed are trimmed.
func TestFilterFences_KeptLeadingFence_KeepsSourceLeadingBlankLines(t *testing.T) {
	t.Parallel()
	got := FilterFences("\n::target claude\nC.\n::end\n\nShared.\n", []string{"claude"})
	if want := "\nC.\n\nShared.\n"; got != want {
		t.Errorf("FilterFences:\ngot:  %q\nwant: %q", got, want)
	}
}

// FenceTargets collects every name used by a fence opener, deduplicated
// in first-seen order, ignoring markers that are not at column 0.
func TestFenceTargets_CollectsNamesInOrder(t *testing.T) {
	t.Parallel()
	body := "Intro.\n\n::target codex\nx\n::end\n\n::targets amp codex gemini\ny\n::end\n\n  ::target indented\nnot a marker\n"
	got := FenceTargets(body)
	want := []string{"codex", "amp", "gemini"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FenceTargets:\ngot:  %v\nwant: %v", got, want)
	}
}
