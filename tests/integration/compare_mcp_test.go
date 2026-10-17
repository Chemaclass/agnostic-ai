package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestCompareMCP_ReportsReferencesWithoutWritingOrLaunching(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "agnostic-ai")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = filepath.Clean(filepath.Join(packageDir, "..", ".."))
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	dir := t.TempDir()
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	t.Setenv("AGNOSTIC_AI_NO_UPDATE_CHECK", "1")
	command, args := "sh", []string{"-c", "printf launched > mcp-launched", "${TEST_MCP_ROOT}"}
	if runtime.GOOS == "windows" {
		command, args = "powershell", []string{"-NoProfile", "-Command", "Set-Content mcp-launched launched", "${TEST_MCP_ROOT}"}
	}
	quoted, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"agnostic-ai.yaml":                "targets: [claude, cursor]\n",
		".agnostic-ai/mcps/local.yaml":    "name: local\ncommand: " + command + "\nargs: " + string(quoted) + "\nenv:\n  ROOT: !literal /tmp\n",
		".agnostic-ai/mcps/remote.yaml":   "name: remote\ntype: http\nurl: https://mcp.example.com/mcp\nheaders:\n  Authorization: Bearer ${TEST_MCP_TOKEN:-DUMMY_FALLBACK}\n  X-Team: !literal DUMMY_LITERAL\n",
		".agnostic-ai/mcps/excluded.yaml": "name: excluded\ncommand: not-launched\ntarget-exclude: cursor\n",
	}
	for path, body := range files {
		path = filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	before := snapFiles(t, dir)
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		for _, secret := range []string{"DUMMY_FALLBACK", "DUMMY_LITERAL", "printf launched", "Set-Content", "https://mcp.example.com"} {
			if strings.Contains(string(out), secret) {
				t.Errorf("report exposed a connection value: %s", secret)
			}
		}
		return string(out)
	}
	text := run("compare", "claude", "cursor")
	raw := run("compare", "claude", "cursor", "--json")
	var report struct {
		Specs []struct {
			Kind, Name string
			Fields     []struct {
				Field   string
				Results []struct{ Target, Status string }
			}
		}
	}
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, entry := range report.Specs {
		if entry.Kind != "mcp" {
			continue
		}
		found[entry.Name] = true
		for _, field := range entry.Fields {
			if !strings.Contains(text, field.Field) {
				t.Errorf("text omitted JSON field %s", field.Field)
			}
			for _, result := range field.Results {
				if !strings.Contains(text, result.Status) {
					t.Errorf("text omitted JSON result %s", result.Status)
				}
			}
		}
	}
	for _, name := range []string{"local", "remote", "excluded"} {
		if !found[name] {
			t.Errorf("report omitted server %s", name)
		}
	}
	if after := snapFiles(t, dir); !reflect.DeepEqual(before, after) {
		t.Error("comparison wrote files or launched a server")
	}
	if _, err := os.Stat(filepath.Join(dir, "mcp-launched")); !os.IsNotExist(err) {
		t.Errorf("server launch marker exists: %v", err)
	}
}
