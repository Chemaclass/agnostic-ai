package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestGraph_JSONFlagMatchesFormatJSON(t *testing.T) {
	testutil.Chdir(t, setupGraphFixture(t))
	silence(t)

	want := runGraph(t, "--format", "json")
	if got := runGraph(t, "--json"); got != want {
		t.Errorf("--json output differs from --format json:\n%s\nwant:\n%s", got, want)
	}
	if got := runGraph(t, "--json", "--format", "json"); got != want {
		t.Errorf("--json with --format json differs:\n%s", got)
	}
}

func TestGraph_JSONFlagRejectsAnotherFormat(t *testing.T) {
	testutil.Chdir(t, setupGraphFixture(t))
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"graph", "--json", "--format", "dot"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "--json") {
		t.Fatalf("--json --format dot must fail naming --json, got %v", err)
	}
	if got := errs.CodeOf(err); got != errs.CodeFlagConflict {
		t.Errorf("code=%q, want %q", got, errs.CodeFlagConflict)
	}
}

func TestWhy_JSONFlagRejectsAnotherFormat(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"why", ".claude/rules/r1.md", "--json", "--format", "text"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "--json") {
		t.Fatalf("--json --format text must fail naming --json, got %v", err)
	}
	if got := errs.CodeOf(err); got != errs.CodeFlagConflict {
		t.Errorf("code=%q, want %q", got, errs.CodeFlagConflict)
	}
}

func TestHookRun_JSONFlagMatchesFormatJSON(t *testing.T) {
	skipWithoutPOSIXShell(t)
	hookRunProject(t, "name: guard\nevent: PreToolUse\nmatcher: Edit|Write\n"+
		`command: 'if grep -q "\.github/workflows/"; then echo "protected file" >&2; exit 2; fi'`+"\n")

	want, wantErr := runHookRun(t, "guard", "--edit", ".github/workflows/tests.yml", "--expect", "allow", "--format", "json")
	got, gotErr := runHookRun(t, "guard", "--edit", ".github/workflows/tests.yml", "--expect", "allow", "--json")
	elapsed := regexp.MustCompile(`"elapsed_ms": \d+`)
	if elapsed.ReplaceAllString(got, "") != elapsed.ReplaceAllString(want, "") {
		t.Errorf("--json output differs from --format json:\n%s\nwant:\n%s", got, want)
	}
	var stdout bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"hook", "run", "guard", "--edit", ".github/workflows/tests.yml", "--json"})
	_ = root.Execute()
	if !json.Valid(stdout.Bytes()) {
		t.Errorf("--json stdout must be one JSON value:\n%s", stdout.String())
	}
	if (gotErr == nil) != (wantErr == nil) || gotErr == nil {
		t.Errorf("--json must fail like --format json on the wrong decision, got %v, want %v", gotErr, wantErr)
	}
}

func TestHookRun_JSONFlagRejectsAnotherFormat(t *testing.T) {
	hookRunProject(t, "name: guard\nevent: PreToolUse\ncommand: 'true'\n")

	_, err := runHookRun(t, "guard", "--json", "--format", "text")
	if err == nil || !strings.Contains(err.Error(), "--json") {
		t.Fatalf("--json --format text must fail naming --json, got %v", err)
	}
	if got := errs.CodeOf(err); got != errs.CodeFlagConflict {
		t.Errorf("code=%q, want %q", got, errs.CodeFlagConflict)
	}
}
