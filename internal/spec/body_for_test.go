package spec

import (
	"strings"
	"testing"
)

// Bodies without `::target` fences pass through unchanged. The common
// case carries no fence — keep it cheap.
func TestEntry_BodyFor_NoFences_Unchanged(t *testing.T) {
	body := "# Test\n\n## Workflow\n\nStep 1.\n"
	e := Entry{Body: body}
	if got := e.BodyFor("claude"); got != body {
		t.Errorf("BodyFor mutated unfenced body:\ngot:  %q\nwant: %q", got, body)
	}
	if got := e.BodyFor(""); got != body {
		t.Errorf("empty target mutated body:\ngot %q", got)
	}
}

// A fenced section pinned to the matching target emits without the
// fence markers but with surrounding whitespace preserved.
func TestEntry_BodyFor_MatchingFence_RendersInline(t *testing.T) {
	body := "Intro line.\n\n::target codex\nCodex-only paragraph.\n::end\n\nOutro.\n"
	e := Entry{Body: body}
	got := e.BodyFor("codex")
	want := "Intro line.\n\nCodex-only paragraph.\n\nOutro.\n"
	if got != want {
		t.Errorf("BodyFor(codex):\ngot:  %q\nwant: %q", got, want)
	}
}

// A fenced section pinned to a non-matching target disappears entirely
// (including the marker lines) so the rendered body reads cleanly for
// the active target.
func TestEntry_BodyFor_NonMatchingFence_Dropped(t *testing.T) {
	body := "Intro.\n\n::target codex\nCodex stuff.\n::end\n\nOutro.\n"
	e := Entry{Body: body}
	got := e.BodyFor("claude")
	want := "Intro.\n\nOutro.\n"
	if got != want {
		t.Errorf("BodyFor(claude):\ngot:  %q\nwant: %q", got, want)
	}
}

// Multiple consecutive fences for different targets emit only the
// matching one.
func TestEntry_BodyFor_MultipleFences_MatchOneOnly(t *testing.T) {
	body := "Shared header.\n\n::target claude\nClaude paragraph.\n::end\n\n::target codex\nCodex paragraph.\n::end\n\nShared footer.\n"
	e := Entry{Body: body}
	if got := e.BodyFor("claude"); !strings.Contains(got, "Claude paragraph.") || strings.Contains(got, "Codex paragraph.") {
		t.Errorf("BodyFor(claude) leaked the wrong fence:\n%s", got)
	}
	if got := e.BodyFor("codex"); !strings.Contains(got, "Codex paragraph.") || strings.Contains(got, "Claude paragraph.") {
		t.Errorf("BodyFor(codex) leaked the wrong fence:\n%s", got)
	}
	if got := e.BodyFor("gemini"); strings.Contains(got, "Claude paragraph.") || strings.Contains(got, "Codex paragraph.") {
		t.Errorf("BodyFor(gemini) leaked a fence:\n%s", got)
	}
}

// `::targets <a> <b>` allow-lists multiple targets in one fence.
func TestEntry_BodyFor_TargetsList_MatchesAny(t *testing.T) {
	body := "::targets claude codex\nShared section.\n::end\n"
	e := Entry{Body: body}
	if got := e.BodyFor("claude"); !strings.Contains(got, "Shared section.") {
		t.Errorf("BodyFor(claude) dropped a multi-target fence:\n%s", got)
	}
	if got := e.BodyFor("codex"); !strings.Contains(got, "Shared section.") {
		t.Errorf("BodyFor(codex) dropped a multi-target fence:\n%s", got)
	}
	if got := e.BodyFor("gemini"); strings.Contains(got, "Shared section.") {
		t.Errorf("BodyFor(gemini) kept a multi-target fence excluding it:\n%s", got)
	}
}

// An unterminated fence runs to end-of-body. Avoids dropping the rest
// of the file on a typo while still rendering deterministically.
func TestEntry_BodyFor_UnterminatedFence_RunsToEnd(t *testing.T) {
	body := "Intro.\n\n::target codex\nNo end marker.\n"
	e := Entry{Body: body}
	if got := e.BodyFor("codex"); !strings.Contains(got, "No end marker.") {
		t.Errorf("BodyFor(codex) dropped unterminated fence content:\n%s", got)
	}
	if got := e.BodyFor("claude"); strings.Contains(got, "No end marker.") {
		t.Errorf("BodyFor(claude) leaked unterminated fence content:\n%s", got)
	}
}

// Empty target string returns the raw body (preserves fences). Useful
// for the source view / round-trip emit where the fence syntax is the
// canonical form on disk.
func TestEntry_BodyFor_EmptyTarget_PreservesFences(t *testing.T) {
	body := "::target codex\nx\n::end\n"
	e := Entry{Body: body}
	if got := e.BodyFor(""); got != body {
		t.Errorf("empty target should return raw body, got %q", got)
	}
}
