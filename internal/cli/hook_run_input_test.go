package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestHookRun_RejectsAPayloadThatIsNotAJSONObject(t *testing.T) {
	dir := hookRunProject(t, "name: guard\nevent: PreToolUse\ncommand: 'true'\n")
	for name, body := range map[string]string{"broken": "{not json", "null": "null", "list": "[]", "empty": ""} {
		bad := filepath.Join(dir, name+".json")
		mustWrite(t, bad, body)

		out, err := runHookRun(t, "guard", "--payload", bad, "--expect", "allow")
		if err == nil || !strings.Contains(err.Error(), name+".json") {
			t.Errorf("%s: an unusable payload must fail naming the file, got %v\n%s", name, err, out)
		}
		if strings.Contains(out, "claude:") {
			t.Errorf("%s: no hook may run on an unusable payload:\n%s", name, out)
		}
	}
}

func TestHookRun_JSONKeepsItsShapeWhenATargetCannotBeSetUp(t *testing.T) {
	dir := hookRunProject(t, "name: guard\nevent: PreToolUse\nmatcher: 'Edit|('\ncommand: 'true'\n")
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [aider, claude]\n")

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
	if report.Hook != "guard" || len(report.Targets) != 1 || !strings.HasPrefix(report.Error, "claude:") {
		t.Errorf("the report must keep the target checked before the failure and name the failing one: %+v", report)
	}
	if len(report.Targets) == 1 && !strings.Contains(string(report.Targets[0]), `"target": "aider"`) {
		t.Errorf("the kept result must be aider's: %s", report.Targets[0])
	}
}
