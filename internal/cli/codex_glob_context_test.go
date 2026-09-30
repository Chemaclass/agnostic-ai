package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSync_CodexGlobContextUsesExactSubtrees(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("agnostic-ai.yaml", "version: 1\ntargets: [codex, amp, cursor, copilot]\n")
	if err := os.MkdirAll(".agnostic-ai/rules", 0755); err != nil {
		t.Fatal(err)
	}
	write(".agnostic-ai/rules/api.md", "---\nglobs: [src/app/api/**, prisma/**/*]\n---\nAPI subtree conventions.\n")
	run := func(args ...string) {
		t.Helper()
		cmd := NewRootCmd("test")
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	}
	run("sync")
	root, err := os.ReadFile("AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(root), "API subtree conventions.") {
		t.Error("directory rule still always loads at root")
	}
	for _, path := range []string{"src/app/api/AGENTS.md", "prisma/AGENTS.md"} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "API subtree conventions.") {
			t.Errorf("rule missing from %s", path)
		}
	}
	run("sync", "--check")
	cfg, bundle, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	edges, err := computeGraphEdges(bundle, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, edge := range edges {
		if edge.Target == "codex" && edge.Path == "AGENTS.md" {
			t.Errorf("graph still places subtree rule at root: %+v", edge)
		}
	}
	report, err := traceFile("src/app/api/AGENTS.md", cfg, bundle, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Sources) != 1 || report.Sources[0].Path != ".agnostic-ai/rules/api.md" {
		t.Errorf("nested document cannot be traced to its rule: %+v", report)
	}
	run("sync", "--only", "amp")
	write(".agnostic-ai/rules/api.md", "---\nglobs: billing/**\n---\nAPI subtree conventions.\n")
	run("sync")
	for _, path := range []string{"src/app/api/AGENTS.md", "prisma/AGENTS.md"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("old generated document remains at %s: %v", path, err)
		}
	}
	write("agnostic-ai.yaml", "version: 1\ntargets: [codex, amp, cursor, copilot]\noutputs:\n  codex:\n    nested-glob-rules: false\n")
	run("sync")
	root, err = os.ReadFile("AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(root), "API subtree conventions.") {
		t.Error("opt-out did not restore root inlining")
	}
	if _, err := os.Stat("billing/AGENTS.md"); !os.IsNotExist(err) {
		t.Errorf("opt-out left nested context: %v", err)
	}
}

func TestSync_CodexGlobContextProtectsUserOwnedDocument(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	for _, dir := range []string{".agnostic-ai/rules", "src/api"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for path, body := range map[string]string{
		"agnostic-ai.yaml":          "version: 1\ntargets: [codex]\n",
		".agnostic-ai/rules/api.md": "---\nglobs: src/api/**\n---\nAPI subtree conventions.\n",
		"src/api/AGENTS.md":         "Keep my handwritten instruction.\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := NewRootCmd("test")
	cmd.SetArgs([]string{"sync"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "src/api/AGENTS.md") {
		t.Errorf("expected ownership refusal, got %v", err)
	}
	body, err := os.ReadFile("src/api/AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "Keep my handwritten instruction.\n" {
		t.Error("user-owned document changed")
	}
}
