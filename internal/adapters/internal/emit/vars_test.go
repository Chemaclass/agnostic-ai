package emit

import (
	"strings"
	"testing"
)

func TestExpandVars_ReplacesKnownNames(t *testing.T) {
	vals := map[string]string{"SKILLS_DIR": ".claude/skills", "AGENTS_DIR": ".claude/agents"}

	got, unresolved := ExpandVars("Put skills in {{$SKILLS_DIR}} and profiles in {{$AGENTS_DIR}}.", vals)

	want := "Put skills in .claude/skills and profiles in .claude/agents."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if len(unresolved) != 0 {
		t.Errorf("expected none unresolved, got %v", unresolved)
	}
}

// The $ sigil is what keeps this from eating real content. Warp
// workflows use {{placeholder}} for arguments, and specs quote
// Handlebars and Jinja in prose.
func TestExpandVars_LeavesSigillessBracesAlone(t *testing.T) {
	body := "Run with {{branch}} and {{ .Values.name }} and {%raw%}."

	got, unresolved := ExpandVars(body, map[string]string{"SKILLS_DIR": ".claude/skills"})

	if got != body {
		t.Errorf("non-variable braces must survive verbatim:\ngot  %q\nwant %q", got, body)
	}
	if len(unresolved) != 0 {
		t.Errorf("expected none unresolved, got %v", unresolved)
	}
}

// A target with no surface for a variable keeps the token verbatim
// instead of collapsing to an empty string, which would silently turn
// "see {{$COMMANDS_DIR}}" into "see ".
func TestExpandVars_UnknownNameStaysLiteralAndIsReported(t *testing.T) {
	got, unresolved := ExpandVars("see {{$COMMANDS_DIR}} now", map[string]string{"SKILLS_DIR": ".claude/skills"})

	if !strings.Contains(got, "{{$COMMANDS_DIR}}") {
		t.Errorf("unresolved variable must stay literal, got %q", got)
	}
	if len(unresolved) != 1 || unresolved[0] != "COMMANDS_DIR" {
		t.Errorf("expected COMMANDS_DIR reported, got %v", unresolved)
	}
}

// An empty value means the target declares the surface but has it
// switched off (an unset opt-in dir). Treat it like a missing one.
func TestExpandVars_EmptyValueIsUnresolved(t *testing.T) {
	got, unresolved := ExpandVars("see {{$COMMANDS_DIR}}", map[string]string{"COMMANDS_DIR": ""})

	if !strings.Contains(got, "{{$COMMANDS_DIR}}") {
		t.Errorf("empty value must not blank the token, got %q", got)
	}
	if len(unresolved) != 1 {
		t.Errorf("expected COMMANDS_DIR reported, got %v", unresolved)
	}
}

func TestExpandVars_ReportsEachUnresolvedNameOnce(t *testing.T) {
	_, unresolved := ExpandVars("{{$A}} {{$A}} {{$B}}", nil)

	if len(unresolved) != 2 {
		t.Fatalf("expected 2 distinct names, got %v", unresolved)
	}
	if unresolved[0] != "A" || unresolved[1] != "B" {
		t.Errorf("expected sorted distinct names, got %v", unresolved)
	}
}

func TestExpandVars_NoVariablesReturnsInputUnchanged(t *testing.T) {
	body := "plain body with no variables"

	got, unresolved := ExpandVars(body, map[string]string{"SKILLS_DIR": ".claude/skills"})

	if got != body || len(unresolved) != 0 {
		t.Errorf("got %q / %v, want unchanged", got, unresolved)
	}
}

// Lowercase is not the documented spelling, so it must not resolve.
// Otherwise two spellings would drift into meaning the same thing.
func TestExpandVars_IgnoresLowercaseNames(t *testing.T) {
	body := "see {{$skills_dir}}"

	got, unresolved := ExpandVars(body, map[string]string{"SKILLS_DIR": ".claude/skills"})

	if got != body {
		t.Errorf("lowercase must not resolve, got %q", got)
	}
	if len(unresolved) != 0 {
		t.Errorf("lowercase is not a variable at all, got %v", unresolved)
	}
}
