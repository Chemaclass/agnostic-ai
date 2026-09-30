package cli

import (
	"bytes"
	"encoding/json"
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

func TestSync_CodexGlobContextOneOffReaderPreservesHandwrittenDocument(t *testing.T) {
	for _, jsonMode := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "json"}[jsonMode], func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			silence(t)
			captureLogOut(t)
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
			args := []string{"sync", "-t", "codex,crush"}
			if jsonMode {
				args = append(args, "--json")
			}
			var out bytes.Buffer
			cmd := NewRootCmd("test")
			cmd.SetOut(&out)
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile("src/api/AGENTS.md")
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != "Keep my handwritten instruction.\n" {
				t.Errorf("one-off reader bypassed ownership and overwrote nested document: %s", body)
			}
			root, err := os.ReadFile("AGENTS.md")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(root), "API subtree conventions.") {
				t.Error("fallback lost root delivery")
			}
			if jsonMode {
				var result jsonOutput
				if err := json.Unmarshal(out.Bytes(), &result); err != nil {
					t.Fatalf("invalid JSON: %v: %s", err, &out)
				}
				for _, write := range result.Writes {
					if write.Path == "src/api/AGENTS.md" {
						t.Errorf("JSON planned an unsafe nested write: %+v", write)
					}
				}
			}
			cmd = NewRootCmd("test")
			cmd.SetArgs([]string{"sync", "-t", "codex,crush", "--check"})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			var preview bytes.Buffer
			cmd = NewRootCmd("test")
			cmd.SetOut(&preview)
			cmd.SetArgs([]string{"render", ".agnostic-ai/rules/api.md", "-t", "codex,crush"})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(preview.String(), "src/api/AGENTS.md") || !strings.Contains(preview.String(), "API subtree conventions.") {
				t.Errorf("preview differs from actual root delivery: %s", &preview)
			}
			args = []string{"revert", "-t", "codex,crush", "--force"}
			if jsonMode {
				args = append(args, "--json")
			}
			cmd = NewRootCmd("test")
			cmd.SetOut(&out)
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			body, err = os.ReadFile("src/api/AGENTS.md")
			if err != nil || string(body) != "Keep my handwritten instruction.\n" {
				t.Errorf("revert touched a nested destination sync never wrote: %v, %s", err, body)
			}
		})
	}
}

func TestSync_CodexGlobContextUnmanagedDestinationKeepsWholeRuleInline(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	captureLogOut(t)
	notes := captureNotes(t)
	for _, dir := range []string{".agnostic-ai/rules", "src/api"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for path, body := range map[string]string{
		"agnostic-ai.yaml":          "version: 1\ntargets: [codex]\nsync:\n  unmanaged: [src/api/AGENTS.md]\n",
		".agnostic-ai/rules/api.md": "---\nglobs: [src/api/**, prisma/**]\n---\nAPI subtree conventions.\n",
		"src/api/AGENTS.md":         "Keep my handwritten instruction.\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := NewRootCmd("test")
	cmd.SetArgs([]string{"sync"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	root, err := os.ReadFile("AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(root), "API subtree conventions.") {
		t.Error("unmanaged destination silently dropped rule")
	}
	body, err := os.ReadFile("src/api/AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "Keep my handwritten instruction.\n" {
		t.Error("unmanaged document changed")
	}
	if _, err := os.Stat("prisma/AGENTS.md"); !os.IsNotExist(err) {
		t.Errorf("mixed managed destinations were partially relocated: %v", err)
	}
	if !strings.Contains(notes.String(), "src/api/AGENTS.md is unmanaged") || !strings.Contains(notes.String(), "always loaded from AGENTS.md") {
		t.Errorf("missing named unmanaged fallback note: %s", notes)
	}
	if err := os.WriteFile("agnostic-ai.yaml", []byte("version: 1\ntargets: [codex]\non-unsupported: error\nsync:\n  unmanaged: [src/api/AGENTS.md]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd = NewRootCmd("test")
	cmd.SetArgs([]string{"sync"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "unmanaged") || !strings.Contains(err.Error(), "always loaded") {
		t.Errorf("expected named unmanaged fallback error, got %v", err)
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
