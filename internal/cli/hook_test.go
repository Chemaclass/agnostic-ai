package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func runHookPaths(t *testing.T, payload string, args ...string) (string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetIn(strings.NewReader(payload))
	root.SetArgs(append([]string{"hook", "paths"}, args...))
	err := root.Execute()
	return out.String(), err
}

func TestHookPaths_PrintsAClaudeEditRelativeToTheProjectRoot(t *testing.T) {
	dir := testutil.TempCwd(t)
	t.Setenv("AGNOSTIC_AI_TARGET", "claude")
	file := filepath.Join(dir, "src", "Foo.php")
	payload := `{"cwd":` + jsonString(dir) + `,"tool_name":"Write","tool_input":{"file_path":` + jsonString(file) + `,"content":"x"}}`

	out, err := runHookPaths(t, payload)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("src", "Foo.php") + "\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

const codexPatchPayload = `{"cwd":".","tool_name":"apply_patch","tool_input":{"command":"*** Begin Patch\n*** Add File: tests/FooTest.php\n+x\n*** Update File: src/Foo.php\n@@\n-a\n+b\n*** Update File: src/Old.php\n*** Move to: src/New.php\n*** Delete File: src/Gone.php\n*** End Patch\n"}}`

func TestHookPaths_PrintsTheFilesACodexPatchLeavesOnDisk(t *testing.T) {
	testutil.TempCwd(t)
	t.Setenv("AGNOSTIC_AI_TARGET", "")

	out, err := runHookPaths(t, codexPatchPayload, "--target", "codex")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		filepath.Join("tests", "FooTest.php"),
		filepath.Join("src", "Foo.php"),
		filepath.Join("src", "New.php"),
	}, "\n") + "\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestHookPaths_ActionListsEveryChangeIncludingDeletes(t *testing.T) {
	testutil.TempCwd(t)
	t.Setenv("AGNOSTIC_AI_TARGET", "codex")

	out, err := runHookPaths(t, codexPatchPayload, "--action")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"add\t" + filepath.Join("tests", "FooTest.php"),
		"update\t" + filepath.Join("src", "Foo.php"),
		"delete\t" + filepath.Join("src", "Old.php"),
		"move\t" + filepath.Join("src", "New.php"),
		"delete\t" + filepath.Join("src", "Gone.php"),
	}, "\n") + "\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestHookPaths_JSONGivesAMoveItsSource(t *testing.T) {
	testutil.TempCwd(t)
	t.Setenv("AGNOSTIC_AI_TARGET", "codex")

	out, err := runHookPaths(t, codexPatchPayload, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got []map[string]string
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse %q: %v", out, err)
	}
	if len(got) != 5 {
		t.Fatalf("got %d changes, want 5: %v", len(got), got)
	}
	move := got[3]
	if move["action"] != "move" || move["path"] != filepath.Join("src", "New.php") || move["from"] != filepath.Join("src", "Old.php") {
		t.Errorf("move = %v", move)
	}
}

func TestHookPaths_FlagWinsOverTheEnvironment(t *testing.T) {
	testutil.TempCwd(t)
	t.Setenv("AGNOSTIC_AI_TARGET", "claude")

	out, err := runHookPaths(t, codexPatchPayload, "--target", "codex")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, filepath.Join("src", "New.php")) {
		t.Errorf("output = %q, want the Codex patch paths", out)
	}
}

func TestHookPaths_PrintsNothingForAToolCallThatIsNoEdit(t *testing.T) {
	testutil.TempCwd(t)
	t.Setenv("AGNOSTIC_AI_TARGET", "claude")

	out, err := runHookPaths(t, `{"tool_name":"Bash","tool_input":{"command":"ls"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("output = %q, want none", out)
	}
}

func TestHookPaths_FailsWithoutATarget(t *testing.T) {
	testutil.TempCwd(t)
	t.Setenv("AGNOSTIC_AI_TARGET", "")

	_, err := runHookPaths(t, `{}`)
	if err == nil || !strings.Contains(err.Error(), "AGNOSTIC_AI_TARGET") {
		t.Errorf("error = %v, want a hint naming AGNOSTIC_AI_TARGET", err)
	}
}

func TestHookPaths_ReadsAClaudePayloadWithoutATarget(t *testing.T) {
	dir := testutil.TempCwd(t)
	t.Setenv("AGNOSTIC_AI_TARGET", "")
	payload := `{"tool_name":"Edit","tool_input":{"file_path":` + jsonString(filepath.Join(dir, "a.go")) + `}}`

	out, err := runHookPaths(t, payload)
	if err != nil {
		t.Fatal(err)
	}
	if out != "a.go\n" {
		t.Errorf("output = %q, want a.go", out)
	}
}

func TestHookPaths_FailsOnInvalidJSON(t *testing.T) {
	testutil.TempCwd(t)

	if _, err := runHookPaths(t, `{"tool_name":`, "--target", "claude"); err == nil {
		t.Error("invalid JSON returned no error")
	}
}

func TestHookPaths_FailsForATargetItCannotRead(t *testing.T) {
	testutil.TempCwd(t)

	_, err := runHookPaths(t, `{}`, "--target", "aider")
	if err == nil || !strings.Contains(err.Error(), "aider") {
		t.Errorf("error = %v, want one naming aider", err)
	}
}

func TestHookPaths_RejectsActionWithJSON(t *testing.T) {
	testutil.TempCwd(t)

	if _, err := runHookPaths(t, `{}`, "--target", "claude", "--action", "--json"); err == nil {
		t.Error("--action with --json returned no error")
	}
}
