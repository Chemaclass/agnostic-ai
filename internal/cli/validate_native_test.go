package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestValidate_FlagsUnknownHookEvent(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"),
		"version: 1\ntargets:\n  - claude\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "fmt.yaml"),
		"name: fmt\nevent: BogusEvent\nmatcher: \"Edit\"\ncommand: \"echo hi\"\n")
	testutil.Chdir(t, dir)

	root := NewRootCmd("test")
	root.SetArgs([]string{"validate"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(&bytes.Buffer{})
	if err := root.Execute(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "unknown hook event") {
		t.Errorf("expected unknown-event message, got: %s", got)
	}
	if !strings.Contains(got, "BogusEvent") {
		t.Errorf("expected the offending event echoed, got: %s", got)
	}
	if !strings.Contains(got, "supported events:") {
		t.Errorf("expected the supported list, got: %s", got)
	}
}

func TestValidate_AcceptsKnownHookEvent(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"),
		"version: 1\ntargets:\n  - claude\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "fmt.yaml"),
		"name: fmt\nevent: PostToolUse\nmatcher: \"Edit\"\ncommand: \"echo hi\"\n")
	testutil.Chdir(t, dir)

	root := NewRootCmd("test")
	root.SetArgs([]string{"validate"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(&bytes.Buffer{})
	if err := root.Execute(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := out.String()
	if strings.Contains(got, "issue(s) found") {
		t.Errorf("expected no issues for valid hook event, got: %s", got)
	}
}

func TestValidate_HookEventGeminiSubsetUnknownToClaude(t *testing.T) {
	// Gemini supports BeforeTool; Claude does not. With both targets
	// enabled, BeforeTool should validate as ok because the union
	// covers it.
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"),
		"version: 1\ntargets:\n  - claude\n  - gemini\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "before.yaml"),
		"name: before\nevent: BeforeTool\nmatcher: \"\"\ncommand: \"echo hi\"\n")
	testutil.Chdir(t, dir)

	root := NewRootCmd("test")
	root.SetArgs([]string{"validate"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(&bytes.Buffer{})
	if err := root.Execute(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := out.String()
	if strings.Contains(got, "unknown hook event") {
		t.Errorf("union of targets should accept BeforeTool, got: %s", got)
	}
}

func TestValidate_OrphanHookKindWarning(t *testing.T) {
	// Cursor + cline configured; neither emits hooks. The hook spec
	// is dead weight and validate should say so.
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"),
		"version: 1\ntargets:\n  - cursor\n  - cline\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "fmt.yaml"),
		"name: fmt\nevent: PostToolUse\nmatcher: \"\"\ncommand: \"true\"\n")
	testutil.Chdir(t, dir)

	root := NewRootCmd("test")
	root.SetArgs([]string{"validate"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(&bytes.Buffer{})
	if err := root.Execute(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "no enabled target supports hooks") {
		t.Errorf("expected orphan-kind warning, got: %s", got)
	}
	if !strings.Contains(got, "claude") || !strings.Contains(got, "codex") {
		t.Errorf("expected supporters list to include claude/codex, got: %s", got)
	}
}

func TestValidate_MissingHookEventField(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"),
		"version: 1\ntargets:\n  - claude\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "broken.yaml"),
		"name: broken\nmatcher: \"\"\ncommand: \"true\"\n")
	testutil.Chdir(t, dir)

	root := NewRootCmd("test")
	root.SetArgs([]string{"validate"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(&bytes.Buffer{})
	if err := root.Execute(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "missing required 'event:' field") {
		t.Errorf("expected missing-event message, got: %s", got)
	}
}

func TestDoctor_ReportsMCPCommandResolution(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"),
		"version: 1\ntargets:\n  - claude\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "fs.yaml"),
		"name: fs\ncommand: this-binary-does-not-exist-please\nargs:\n  - hi\n")
	testutil.Chdir(t, dir)

	root := NewRootCmd("test")
	root.SetArgs([]string{"doctor"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(&bytes.Buffer{})
	// Doctor returns a non-nil error when drift is present (it always
	// is for a fresh project: no CLAUDE.md, etc.). The point of this
	// test is the MCP block in the printed output, not the exit code.
	_ = root.Execute()
	got := out.String()
	if !strings.Contains(got, "MCP command resolution:") {
		t.Errorf("expected MCP block, got: %s", got)
	}
	if !strings.Contains(got, "this-binary-does-not-exist-please") {
		t.Errorf("expected the missing command echoed, got: %s", got)
	}
}

func TestDoctor_SkipsMCPBlockWhenNoMCPSpecs(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"),
		"version: 1\ntargets:\n  - claude\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "x.md"),
		"---\nname: x\n---\nbody\n")
	testutil.Chdir(t, dir)

	root := NewRootCmd("test")
	root.SetArgs([]string{"doctor"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(&bytes.Buffer{})
	_ = root.Execute()
	got := out.String()
	if strings.Contains(got, "MCP command resolution:") {
		t.Errorf("expected no MCP block without MCP specs, got: %s", got)
	}
}

func TestDoctor_HTTPMCPSkippedFromCommandResolution(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"),
		"version: 1\ntargets:\n  - claude\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "remote.yaml"),
		"name: remote\nurl: https://example.com/mcp\n")
	testutil.Chdir(t, dir)

	root := NewRootCmd("test")
	root.SetArgs([]string{"doctor"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(&bytes.Buffer{})
	_ = root.Execute()
	got := out.String()
	if strings.Contains(got, "MCP command resolution:") {
		t.Errorf("HTTP MCP without command should not produce a resolution block, got: %s", got)
	}
}

func TestInstallHint_KnownCommands(t *testing.T) {
	cases := map[string]string{
		"npx":    "node",
		"uvx":    "uv",
		"python": "Python",
		"docker": "Docker",
		"madeup": "madeup",
	}
	for in, want := range cases {
		got := installHint(in)
		if !strings.Contains(strings.ToLower(got), strings.ToLower(want)) {
			t.Errorf("installHint(%q) → %q, expected to mention %q", in, got, want)
		}
	}
}
