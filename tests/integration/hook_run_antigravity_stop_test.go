package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestHookRun_AntigravityEmptyStopReplyAllowsCompletion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Antigravity hook diagnostics require a POSIX shell")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is unavailable")
	}
	binary := builtBinary(t)
	project := t.TempDir()
	for name, body := range map[string]string{
		"agnostic-ai.yaml":             "version: 1\ntargets: [antigravity]\n",
		".agnostic-ai/hooks/done.yaml": "name: done\nevent: Stop\ncommand: .agnostic-ai/scripts/stop.sh\n",
		".agnostic-ai/scripts/stop.sh": "#!/bin/sh\ncat >/dev/null\nprintf '{}'\n",
		"payload.json":                 `{"executionNum":1,"fullyIdle":true}`,
	} {
		path := filepath.Join(project, name)
		must(t, os.MkdirAll(filepath.Dir(path), 0o755))
		must(t, os.WriteFile(path, []byte(body), 0o644))
	}
	must(t, os.Chmod(filepath.Join(project, ".agnostic-ai/scripts/stop.sh"), 0o755))
	sync := exec.Command(binary, "sync", "--gitignore=off")
	sync.Dir = project
	if out, err := sync.CombinedOutput(); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	cmd := exec.Command(binary, "hook", "run", "done", "--target", "antigravity", "--payload", "payload.json", "--format", "json", "--expect", "allow", "--include-assumed")
	cmd.Dir = project
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("hook run: %v\n%s", err, out)
	}
	var report struct {
		Targets []struct {
			Target, Decision string
			Counted          bool
		}
	}
	must(t, json.Unmarshal(out, &report))
	if len(report.Targets) != 1 {
		t.Fatalf("want one target, got %s", out)
	}
	if got := report.Targets[0]; got.Target != "antigravity" || got.Decision != "allow" || !got.Counted {
		t.Errorf("empty Stop reply = %+v, want counted allow", got)
	}
}
