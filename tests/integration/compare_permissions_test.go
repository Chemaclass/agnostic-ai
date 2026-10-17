package integration

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestComparePermissions_ReportsPolicyWithoutWriting(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "agnostic-ai")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	dir := t.TempDir()
	files := map[string]string{
		"agnostic-ai.yaml":                  "targets: [claude, codex]\noutputs:\n  codex:\n    exec-policies-from-permissions: true\n",
		".agnostic-ai/settings/policy.yaml": "name: policy\npermissions:\n  allow: [shell(go test:*), read(src/**)]\n  ask: [shell(git push:*)]\n  deny: [shell(rm:*), delete]\n  default-mode: plan\n",
		".claude/settings.json":             "{\"userSetting\":true}\n",
	}
	for name, body := range files {
		path := filepath.Join(dir, name)
		must(t, os.MkdirAll(filepath.Dir(path), 0o755))
		must(t, os.WriteFile(path, []byte(body), 0o644))
	}
	snapshot := func() map[string]string {
		t.Helper()
		result := map[string]string{}
		must(t, filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			result[path] = string(body)
			return nil
		}))
		return result
	}
	before := snapshot()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "AGNOSTIC_AI_HOME="+t.TempDir(), "AGNOSTIC_AI_NO_UPDATE_CHECK=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	text := run("compare", "claude", "codex")
	var document struct {
		Differences int `json:"differences"`
		Specs       []struct {
			Kind   string `json:"kind"`
			Fields []struct {
				Field   string `json:"field"`
				Differs bool   `json:"differs"`
				Results []struct {
					Target string `json:"target"`
					Status string `json:"status"`
					Reason string `json:"reason"`
				} `json:"results"`
			} `json:"fields"`
		} `json:"specs"`
	}
	must(t, json.Unmarshal([]byte(run("compare", "claude", "codex", "--json")), &document))
	count := 0
	for _, entry := range document.Specs {
		if entry.Kind != "settings" {
			continue
		}
		for _, field := range entry.Fields {
			count++
			if !strings.Contains(text, field.Field) {
				t.Errorf("text omits JSON field %s: %s", field.Field, text)
			}
			for _, result := range field.Results {
				if !strings.Contains(text, result.Reason) || !strings.Contains(text, result.Status) || !strings.Contains(text, result.Target) {
					t.Errorf("text and JSON disagree on %+v: %s", result, text)
				}
			}
		}
	}
	if count != 6 {
		t.Errorf("permission rows = %d, want six", count)
	}
	if !strings.Contains(text, "read(src/**): the adapter does not write") || !strings.Contains(text, "global sync") || !strings.Contains(text, ".codex/rules/") {
		t.Errorf("supported output and unsupported restrictions must be visible: %s", text)
	}
	if !reflect.DeepEqual(before, snapshot()) {
		t.Error("compare changed project or existing native files")
	}
	policy := filepath.Join(dir, ".agnostic-ai/settings/policy.yaml")
	must(t, os.WriteFile(policy, []byte("name: policy\npermissions:\n  allow: [web]\n"), 0o644))
	beforePartial := snapshot()
	partialText := run("compare", "claude", "windsurf")
	must(t, json.Unmarshal([]byte(run("compare", "claude", "windsurf", "--json")), &document))
	if len(document.Specs) != 1 || len(document.Specs[0].Fields) != 1 {
		t.Fatalf("expected one web permission row: %+v", document)
	}
	field := document.Specs[0].Fields[0]
	if !field.Differs || document.Differences != 1 || !strings.Contains(partialText, "permissions.allow[0] (differs)") || !strings.Contains(partialText, "1 of 1 fields differ") {
		t.Errorf("text and JSON must count partial permission loss: %+v\n%s", document, partialText)
	}
	for _, result := range field.Results {
		if result.Status != "translated" || !strings.Contains(partialText, result.Reason) {
			t.Errorf("independent translation results changed or formats disagree: %+v\n%s", result, partialText)
		}
		if result.Target == "windsurf" && !strings.Contains(result.Reason, "part of this permission is unsupported") {
			t.Errorf("Windsurf partial loss must remain visible: %+v", result)
		}
	}
	if !reflect.DeepEqual(beforePartial, snapshot()) {
		t.Error("partial-loss comparison changed project or existing native files")
	}
}
