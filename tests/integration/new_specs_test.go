package integration

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestNewSpecs_LoadAndRenderAddedKinds(t *testing.T) {
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
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	t.Setenv("AGNOSTIC_AI_NO_UPDATE_CHECK", "1")
	cases := []struct {
		kind, source, ext, target, output, content string
	}{
		{"command", "commands", ".md", "claude", ".claude/commands/example.md", "Inspect the project and report findings."},
		{"settings", "settings", ".yaml", "claude", ".claude/settings.json", "Bash(go test:*)"},
		{"review", "reviews", ".md", "cursor", ".cursor/BUGBOT.md", "Inspect the project and report findings."},
		{"environment", "environments", ".yaml", "cursor", ".cursor/environment.json", "rendered"},
		{"ignore", "ignore", ".md", "cursor", ".cursorignore", "scaffold-cache/"},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join("custom specs", c.source)
			config := fmt.Sprintf("version: 1\nsources:\n  %s: %s\ntargets: [%s]\n", c.source, filepath.ToSlash(source), c.target)
			if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(config), 0o644); err != nil {
				t.Fatal(err)
			}
			run := func(args ...string) string {
				t.Helper()
				cmd := exec.Command(binary, args...)
				cmd.Dir = dir
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("%v: %v\n%s", args, err, out)
				}
				return string(out)
			}
			before := snapFiles(t, dir)
			preview := run("new", c.kind, "example", "--dry-run")
			if after := snapFiles(t, dir); !reflect.DeepEqual(before, after) {
				t.Error("preview changed project files")
			}
			path := filepath.Join(source, "example"+c.ext)
			prefix := "would create " + path + "\n\n"
			if !strings.HasPrefix(preview, prefix) {
				t.Fatalf("preview omitted actual path: %s", preview)
			}
			run("new", c.kind, "example")
			body, err := os.ReadFile(filepath.Join(dir, path))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(body, []byte(strings.TrimPrefix(preview, prefix))) {
				t.Error("preview and created content differ")
			}
			lint := run("lint", "--json")
			if !strings.Contains(lint, "LINT031") || !strings.Contains(lint, "example") {
				t.Errorf("created spec did not load with its marked placeholder: %s", lint)
			}
			var edited []string
			for _, line := range strings.Split(string(body), "\n") {
				switch {
				case strings.HasPrefix(line, "description: TODO"):
					line = "description: Project configuration example."
				case strings.HasPrefix(line, "TODO:"):
					line = "Inspect the project and report findings."
				case strings.HasPrefix(line, "# TODO"):
					if c.kind == "ignore" {
						line = "scaffold-cache/"
					} else {
						continue
					}
				}
				edited = append(edited, line)
			}
			text := strings.Join(edited, "\n")
			if c.kind == "settings" {
				text += "permissions:\n  ask:\n    - shell(go test:*)\n"
			}
			if c.kind == "environment" {
				text += "install: echo rendered > scaffold-was-executed\n"
			}
			if err := os.WriteFile(filepath.Join(dir, path), []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
			before = snapFiles(t, dir)
			rendered := run("render", path, "--target", c.target)
			if !strings.Contains(rendered, c.output) || !strings.Contains(rendered, c.content) {
				t.Errorf("render did not produce usable %s output: %s", c.kind, rendered)
			}
			if after := snapFiles(t, dir); !reflect.DeepEqual(before, after) {
				t.Error("render wrote files or executed the environment command")
			}
		})
	}
	t.Run("leading dash render hint", func(t *testing.T) {
		dir := t.TempDir()
		config := "version: 1\nsources:\n  settings: -settings\ntargets: [claude]\n"
		if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}
		create := exec.Command(binary, "new", "settings", "example")
		create.Dir = dir
		out, err := create.CombinedOutput()
		if err != nil {
			t.Fatalf("create leading dash source: %v\n%s", err, out)
		}
		text := string(out)
		start := strings.Index(text, "agnostic-ai render ")
		if start < 0 {
			t.Fatalf("create omitted render hint: %s", text)
		}
		end := strings.Index(text[start:], " --target <name>`")
		if end < 0 {
			t.Fatalf("create omitted render hint target: %s", text)
		}
		hint := strings.Replace(text[start:start+end+len(" --target <name>")], "<name>", "claude", 1)
		body := "name: example\ndescription: Ask before running tests.\npermissions:\n  ask:\n    - shell(go test:*)\n"
		if err := os.WriteFile(filepath.Join(dir, "-settings", "example.yaml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		before := snapFiles(t, dir)
		renderHint := exec.Command("sh", "-c", hint)
		if runtime.GOOS == "windows" {
			renderHint = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", hint)
		}
		renderHint.Dir = dir
		renderHint.Env = append(os.Environ(), "PATH="+filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
		rendered, err := renderHint.CombinedOutput()
		if err != nil {
			t.Fatalf("copied render hint %q: %v\n%s", hint, err, rendered)
		}
		if !strings.Contains(string(rendered), "Bash(go test:*)") {
			t.Errorf("copied hint did not render the chosen permission: %s", rendered)
		}
		if after := snapFiles(t, dir); !reflect.DeepEqual(before, after) {
			t.Error("copied render hint changed project files")
		}
	})

}
