package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func setupEmptyProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := `version: 1
sources:
  agents: .agnostic-ai/agents
  skills: .agnostic-ai/skills
  rules: .agnostic-ai/rules
  hooks: .agnostic-ai/hooks
  mcps: .agnostic-ai/mcps
targets:
  - claude
  - cursor
`
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestNew_WritesRule(t *testing.T) {
	dir := setupEmptyProject(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"new", "rule", "no-console-log"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, ".agnostic-ai", "rules", "no-console-log.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected rule file at %s: %v", path, err)
	}
	if !strings.Contains(string(body), "name: no-console-log") {
		t.Errorf("rule body missing frontmatter name field:\n%s", body)
	}
	if !strings.Contains(string(body), "alwaysApply: true") {
		t.Errorf("rule body missing alwaysApply: %s", body)
	}
}

func TestNew_MCPHintLinksRecipes(t *testing.T) {
	dir := setupEmptyProject(t)
	testutil.Chdir(t, dir)
	out := captureSummary(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"new", "mcp", "filesystem"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "https://agnostic-ai.org/docs/spec-format/mcp-recipes/") {
		t.Errorf("mcp hint missing recipes URL:\n%s", out.String())
	}
}

func TestNew_DryRunPrintsScaffoldWithoutWriting(t *testing.T) {
	dir := setupEmptyProject(t)
	testutil.Chdir(t, dir)
	out := captureSummary(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"new", "rule", "no-console-log", "--dry-run"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	got := out.String()
	// The preview names the destination path...
	if !strings.Contains(got, filepath.Join(".agnostic-ai", "rules", "no-console-log.md")) {
		t.Errorf("dry-run output missing target path:\n%s", got)
	}
	// ...and prints the rendered frontmatter and body.
	if !strings.Contains(got, "name: no-console-log") {
		t.Errorf("dry-run output missing rendered frontmatter:\n%s", got)
	}
	if !strings.Contains(got, "alwaysApply: true") {
		t.Errorf("dry-run output missing rendered body:\n%s", got)
	}
	// Nothing may land on disk.
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "rules", "no-console-log.md")); !os.IsNotExist(err) {
		t.Errorf("dry-run wrote a file, want none: %v", err)
	}
}

func TestNew_HookEmitsYAML(t *testing.T) {
	dir := setupEmptyProject(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"new", "hook", "fmt-on-save"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".agnostic-ai", "hooks", "fmt-on-save.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected hook YAML at %s", path)
	}
}

func TestNew_ErrorsIfFileExists(t *testing.T) {
	dir := setupEmptyProject(t)
	testutil.Chdir(t, dir)
	silence(t)

	first := NewRootCmd("test")
	first.SetArgs([]string{"new", "rule", "dup"})
	if err := first.Execute(); err != nil {
		t.Fatal(err)
	}
	second := NewRootCmd("test")
	second.SetArgs([]string{"new", "rule", "dup"})
	err := second.Execute()
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected already-exists error, got %v", err)
	}
}

func TestNew_RejectsUnknownKind(t *testing.T) {
	dir := setupEmptyProject(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"new", "wat", "x"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown kind") {
		t.Fatalf("expected unknown-kind error, got %v", err)
	}
}

func TestNew_RejectsBadName(t *testing.T) {
	dir := setupEmptyProject(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"new", "rule", "Bad Name!"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid name") {
		t.Fatalf("expected invalid-name error, got %v", err)
	}
}

func TestNew_HonorsConfiguredSources(t *testing.T) {
	dir := t.TempDir()
	cfg := `version: 1
sources:
  agents: specs/agents
  skills: specs/skills
  rules: specs/rules
  hooks: specs/hooks
  mcps: specs/mcps
targets: [claude]
`
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"new", "rule", "x"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "specs", "rules", "x.md")); err != nil {
		t.Errorf("expected file under custom sources: %v", err)
	}
}

func TestNew_AgentScaffoldLoadsOnEveryTarget(t *testing.T) {
	dir := setupEmptyProject(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\nsources:\n  agents: .agnostic-ai/agents\n  skills: .agnostic-ai/skills\n  rules: .agnostic-ai/rules\n  hooks: .agnostic-ai/hooks\n  mcps: .agnostic-ai/mcps\ntargets: [claude, codex, cursor]\n")
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"new", "agent", "foo"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	report, _, _ := runLintJSON(t)
	for _, f := range report.Findings {
		// LINT031 is the placeholder description, which new writes on purpose.
		if strings.HasSuffix(f.Path, "foo.md") && f.Code != "LINT031" {
			t.Errorf("fresh agent scaffold has a finding: %s %s", f.Code, f.Message)
		}
	}
}

func TestNew_MistypedKindSuggestsTheClosest(t *testing.T) {
	testutil.Chdir(t, setupEmptyProject(t))
	silence(t)
	root := NewRootCmd("test")
	root.SetArgs([]string{"new", "skil", "deploy"})

	err := root.Execute()

	if err == nil || !strings.Contains(err.Error(), "did you mean skill?") {
		t.Errorf("got %v", err)
	}
}

func TestNew_AddedKindsLoad(t *testing.T) {
	for _, kind := range []string{"command", "settings", "review", "environment", "ignore"} {
		t.Run(kind, func(t *testing.T) {
			testutil.Chdir(t, setupEmptyProject(t))
			silence(t)
			root := NewRootCmd("test")
			root.SetArgs([]string{"new", kind, "example"})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			_, bundle, err := loadProject(".")
			if err != nil {
				t.Fatalf("load scaffold: %v", err)
			}
			found := false
			for _, entry := range bundle.All() {
				if string(entry.Kind) == kind && entry.Name == "example" {
					found = true
				}
			}
			if !found {
				t.Errorf("created %s scaffold did not load as its kind", kind)
			}
		})
	}
}

func TestNew_AddedKindsPreviewCustomPathsAndRefuseOverwrite(t *testing.T) {
	cases := []struct{ kind, source, ext string }{
		{"command", "commands", ".md"}, {"settings", "settings", ".yaml"}, {"review", "reviews", ".md"}, {"environment", "environments", ".yaml"}, {"ignore", "ignore", ".md"},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			dir := setupEmptyProject(t)
			testutil.Chdir(t, dir)
			source := filepath.Join("custom specs", c.source)
			writeFile(t, "agnostic-ai.yaml", fmt.Sprintf("version: 1\nsources:\n  %s: %s\ntargets: [claude, cursor]\n", c.source, filepath.ToSlash(source)))
			out := captureSummary(t)
			run := func(args ...string) error { root := NewRootCmd("test"); root.SetArgs(args); return root.Execute() }
			before := snapshotTree(t, dir)
			if err := run("new", c.kind, "example", "--dry-run"); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(source, "example"+c.ext)
			prefix := "would create " + path + "\n\n"
			preview := strings.TrimPrefix(out.String(), prefix)
			if preview == out.String() {
				t.Fatalf("preview omitted configured path %s: %s", path, out.String())
			}
			if after := snapshotTree(t, dir); !reflect.DeepEqual(before, after) {
				t.Error("dry run changed project files")
			}
			if _, err := os.Stat(source); !os.IsNotExist(err) {
				t.Errorf("dry run created source directory: %v", err)
			}
			if err := run("new", c.kind, "example"); err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != preview {
				t.Errorf("preview and created bytes differ:\npreview:%s\ncreated:%s", preview, body)
			}
			_, bundle, err := loadProject(".")
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, entry := range bundle.All() {
				if string(entry.Kind) == c.kind && entry.Path == path {
					found = true
				}
			}
			if !found {
				t.Errorf("custom path did not load: %s", path)
			}
			if err := run("new", c.kind, "example"); err == nil || !strings.Contains(err.Error(), "already exists") {
				t.Errorf("overwrite refusal: %v", err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(body, after) {
				t.Error("overwrite attempt changed existing bytes")
			}
		})
	}
}

func TestNew_AddedKindsKeepSafeDefaultsAndPlaceholderLint(t *testing.T) {
	testutil.Chdir(t, setupEmptyProject(t))
	silence(t)
	for _, kind := range []string{"command", "settings", "review", "environment", "ignore"} {
		root := NewRootCmd("test")
		root.SetArgs([]string{"new", kind, kind})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
	}
	_, bundle, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range bundle.All() {
		if entry.Kind == spec.KindSettings {
			for _, key := range []string{"permissions", "model", "effort", "protected"} {
				if _, ok := entry.Meta[key]; ok {
					t.Errorf("settings default actively sets %s", key)
				}
			}
		}
		if entry.Kind == spec.KindEnvironment {
			for _, key := range []string{"setup", "setup-windows", "install", "cleanup", "terminals", "dev-commands"} {
				if _, ok := entry.Meta[key]; ok {
					t.Errorf("environment default actively sets %s", key)
				}
			}
		}
	}
	report, _, _ := runLintJSON(t)
	found := map[string]bool{}
	for _, finding := range report.Findings {
		if finding.Code == "LINT031" {
			found[finding.Path] = true
		} else {
			t.Errorf("unexpected scaffold finding: %s %s", finding.Code, finding.Message)
		}
	}
	for _, entry := range bundle.All() {
		if !found[entry.Path] {
			t.Errorf("placeholder description not reported for %s", entry.Path)
		}
	}
	var rendered bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&rendered)
	root.SetArgs([]string{"render", ".agnostic-ai/ignore/ignore.md", "--target", "cursor"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(rendered.String(), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			t.Errorf("ignore scaffold excludes a path by default: %q", line)
		}
	}
}

func TestNew_HelpAndCompletionListAllKinds(t *testing.T) {
	cmd := newNewCmd()
	kinds, directive := cmd.ValidArgsFunction(cmd, nil, "")
	var expected []string
	for _, kind := range spec.AllKinds {
		expected = append(expected, string(kind))
	}
	if !reflect.DeepEqual(kinds, expected) {
		t.Errorf("completion kinds = %v, want %v", kinds, expected)
	}
	if directive == 0 {
		t.Error("completion must refuse file suggestions")
	}
	var help bytes.Buffer
	cmd.SetOut(&help)
	if err := cmd.Help(); err != nil {
		t.Fatal(err)
	}
	for _, kind := range expected {
		if !strings.Contains(help.String(), kind) {
			t.Errorf("help omits %s", kind)
		}
	}
}

func TestNew_RenderHintQuotesConfiguredSettingsPath(t *testing.T) {
	cases := []struct {
		source, quoted string
	}{
		{"custom/settings", "custom/settings/example.yaml"},
		{"-settings", "./-settings/example.yaml"},
		{"custom specs/settings", "'custom specs/settings/example.yaml'"},
		{"developer's specs/settings", "'developer'\\''s specs/settings/example.yaml'"},
		{"$special` specs/settings", "'$special` specs/settings/example.yaml'"},
	}
	for _, c := range cases {
		t.Run(c.source, func(t *testing.T) {
			dir := setupEmptyProject(t)
			testutil.Chdir(t, dir)
			config := fmt.Sprintf("version: 1\nsources:\n  settings: %s\ntargets: [claude]\n", c.source)
			if err := os.WriteFile("agnostic-ai.yaml", []byte(config), 0o644); err != nil {
				t.Fatal(err)
			}
			out := captureSummary(t)
			root := NewRootCmd("test")
			root.SetArgs([]string{"new", "settings", "example"})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			quoted := c.quoted
			if runtime.GOOS == "windows" {
				path := filepath.Join(c.source, "example.yaml")
				switch c.source {
				case "-settings":
					quoted = "./" + path
				case "custom/settings":
					quoted = path
				default:
					quoted = "'" + strings.ReplaceAll(path, "'", "''") + "'"
				}
			}
			want := "`agnostic-ai render " + quoted + " --target <name>`"
			if !strings.Contains(out.String(), want) {
				t.Errorf("render hint must pass the configured path as one literal argument; want %s in %s", want, out.String())
			}
		})
	}
}
