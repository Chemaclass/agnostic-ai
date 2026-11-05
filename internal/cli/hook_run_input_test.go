package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestHookRun_RejectsAPayloadThatIsNotJSON(t *testing.T) {
	dir := hookRunProject(t, "name: guard\nevent: PreToolUse\ncommand: 'true'\n")
	bad := filepath.Join(dir, "bad.json")
	mustWrite(t, bad, "{not json")

	out, err := runHookRun(t, "guard", "--payload", bad, "--expect", "allow")
	if err == nil || !strings.Contains(err.Error(), "bad.json") {
		t.Fatalf("an invalid payload must fail naming the file, got %v\n%s", err, out)
	}
	if strings.Contains(out, "claude:") {
		t.Errorf("no hook may run on an invalid payload:\n%s", out)
	}
}

func TestHookRun_JSONKeepsItsShapeWhenATargetCannotBeSetUp(t *testing.T) {
	hookRunProject(t, "name: guard\nevent: PreToolUse\nmatcher: 'Edit|('\ncommand: 'true'\n")

	var stdout bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"hook", "run", "guard", "--edit", "x.txt", "--format", "json"})
	err := root.Execute()
	if err == nil {
		t.Fatal("a target that cannot be set up must fail the run")
	}
	var report struct {
		Hook    string            `json:"hook"`
		Targets []json.RawMessage `json:"targets"`
		Error   string            `json:"error"`
	}
	if jerr := json.Unmarshal(stdout.Bytes(), &report); jerr != nil {
		t.Fatalf("stdout must be one JSON object: %v\n%s", jerr, stdout.String())
	}
	if report.Hook != "guard" || report.Targets == nil || !strings.Contains(report.Error, "claude") {
		t.Errorf("the report must name the hook, list targets, and name the failing target: %+v", report)
	}
}
