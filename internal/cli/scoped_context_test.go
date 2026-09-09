package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestNewRule_ScaffoldsDirectoryScope(t *testing.T) {
	dir := setupEmptyProject(t)
	testutil.Chdir(t, dir)
	silence(t)
	cmd := NewRootCmd("test")
	cmd.SetArgs([]string{"new", "rule", "payments", "--scope", "services/payments"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(".agnostic-ai/rules/payments.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "scope: services/payments") || strings.Contains(string(body), "alwaysApply:") || strings.Contains(string(body), "globs:") {
		t.Errorf("unexpected scaffold: %s", body)
	}
}

func TestSync_ScopedContextDoesNotEnterRootInstructions(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	if err := os.WriteFile("agnostic-ai.yaml", []byte("version: 1\ntargets: [claude, codex, gemini, cursor, copilot]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(".agnostic-ai/rules", 0o755); err != nil {
		t.Fatal(err)
	}
	for name, scope := range map[string]string{"payments": "services/payments", "refunds": "services/payments/refunds", "catalog": "services/catalog"} {
		body := "---\nname: " + name + "\nscope: " + scope + "\n---\n\nOnly " + name + " convention.\n"
		if err := os.WriteFile(filepath.Join(".agnostic-ai/rules", name+".md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := NewRootCmd("test")
	cmd.SetArgs([]string{"sync", "--all"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"AGENTS.md", "GEMINI.md", "CLAUDE.md"} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "Only payments convention.") {
			t.Errorf("scoped rule leaked to %s", path)
		}
	}
	for _, path := range []string{"services/payments/AGENTS.md", "services/payments/GEMINI.md"} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "Only payments convention.") || strings.Contains(string(b), "Only refunds convention.") || strings.Contains(string(b), "Only catalog convention.") {
			t.Errorf("wrong scope in %s: %s", path, b)
		}
	}
	cmd = NewRootCmd("test")
	cmd.SetArgs([]string{"sync", "--check"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestSync_ScopedContextLifecycle(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	if err := os.WriteFile("agnostic-ai.yaml", []byte("version: 1\ntargets: [codex, amp]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(".agnostic-ai/rules", 0755); err != nil {
		t.Fatal(err)
	}
	source := ".agnostic-ai/rules/payments.md"
	write := func(scope, body string) {
		t.Helper()
		if err := os.WriteFile(source, []byte("---\nname: payments\nscope: "+scope+"\n---\n"+body+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) {
		t.Helper()
		cmd := NewRootCmd("test")
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	}
	write("payments", "first version")
	run("sync")
	write("payments", "second version")
	run("sync", "--backup")
	run("revert", "--force")
	content, err := os.ReadFile("payments/AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "first version") {
		t.Fatalf("shared backup was not restored once: %s", content)
	}
	run("sync")
	write("billing", "moved convention")
	run("sync")
	if _, err := os.Stat("payments/AGENTS.md"); !os.IsNotExist(err) {
		t.Fatalf("old scope still present: %v", err)
	}
	content, err = os.ReadFile("billing/AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "moved convention") {
		t.Fatal("new scope missing")
	}
	run("sync", "--only", "codex")
	run("sync", "--check")
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	run("sync")
	if _, err := os.Stat("billing/AGENTS.md"); !os.IsNotExist(err) {
		t.Fatalf("deleted rule remains: %v", err)
	}
}

func TestScopedContext_InspectionMatchesSync(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	if err := os.WriteFile("agnostic-ai.yaml", []byte("version: 1\ntargets: [codex, cursor]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(".agnostic-ai/rules", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".agnostic-ai/rules/payments.md", []byte("---\nname: payments\nscope: payments\n---\nPayment conventions.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	edges, err := computeGraphEdges(b, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 2 {
		t.Fatalf("want one edge per reader, got %+v", edges)
	}
	for _, e := range edges {
		if e.Path != "payments/AGENTS.md" {
			t.Errorf("unexpected graph path: %+v", e)
		}
	}
	report, err := traceFile("payments/AGENTS.md", cfg, b, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Sources) != 1 || report.Sources[0].Name != "payments" {
		t.Fatalf("wrong scoped sources: %+v", report)
	}
	root, err := traceFile("AGENTS.md", cfg, b, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(root.Sources) != 0 {
		t.Fatalf("scoped source leaked into root trace: %+v", root)
	}
}
