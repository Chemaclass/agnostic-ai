package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportAuditContinueNestedActivation(t *testing.T) {
	for _, fields := range []string{"globs: ['src/**']", "globs: []", "globs: ['src/{a,b}/**', 'ui/**']"} {
		t.Run(fields, func(t *testing.T) {
			root := testutil.TempCwd(t)
			writeFile(t, filepath.Join(root, ".continue/rules/backend/frontend.md"), "---\n"+fields+"\n---\nGuide.\n")
			if err := importFromContinue(root, rootSources()); err != nil {
				t.Fatal(err)
			}
			cfg := &config.Config{Sources: rootSources()}
			bundle, err := spec.LoadBundle(root, cfg)
			if err != nil {
				t.Fatal(err)
			}
			adapter, _ := adapters.Get("continue")
			if err := adapter.Emit(adapters.NewSession(), bundle, cfg, false); err != nil {
				t.Fatal(err)
			}
			want, _ := splitMdcFrontmatter([]byte("---\n" + fields + "\n---\n"))
			got, _ := splitMdcFrontmatter([]byte(readFile(t, filepath.Join(root, ".continue/rules/backend/frontend.md"))))
			if !reflect.DeepEqual(got["globs"], want["globs"]) {
				t.Errorf("globs = %#v, want %#v", got["globs"], want["globs"])
			}
		})
	}
}

func TestImportAuditNestedActivationThroughSync(t *testing.T) {
	for _, tc := range []struct {
		target, dir, fields string
		unsupported         bool
	}{
		{"continue", ".continue/rules", "globs: ['backend/src/**', 'backend/tests/**']", false},
		{"continue", ".continue/rules", "globs: ['backend/src/**']", false},
		{"continue", ".continue/rules", "globs: ['src/**']", true},
		{"continue", ".continue/rules", "globs: []", true},
		{"cline", ".clinerules", "paths: []", false},
	} {
		t.Run(tc.target+tc.fields, func(t *testing.T) {
			root := testutil.TempCwd(t)
			native := filepath.Join(root, tc.dir, "backend/frontend.md")
			writeFile(t, native, "---\n"+tc.fields+"\n---\nGuide.\n")
			importFn := importFromContinue
			if tc.target == "cline" {
				importFn = importFromCline
			}
			if err := importFn(root, rootSources()); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(native); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: ["+tc.target+"]\non-unsupported: error\nsources:\n  rules: rules\n")
			cmd := NewRootCmd("test")
			cmd.SetArgs([]string{"sync", "--all"})
			err := cmd.Execute()
			if tc.unsupported {
				if err == nil || !strings.Contains(err.Error(), "scoped rule") {
					t.Fatalf("expected explicit unsupported scope error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, _ := splitMdcFrontmatter([]byte(readFile(t, native)))
			want, _ := splitMdcFrontmatter([]byte("---\n" + tc.fields + "\n---\n"))
			for key, value := range want {
				if !reflect.DeepEqual(got[key], value) {
					t.Errorf("%s = %#v, want %#v", key, got[key], value)
				}
			}
		})
	}
}

func TestImportAuditClaudeReenableAfterOverlayCapture(t *testing.T) {
	root := testutil.TempCwd(t)
	writeFile(t, filepath.Join(root, ".claude/settings.json"), `{"disabledMcpjsonServers":["manual"]}`)
	entry := spec.Entry{Kind: spec.KindMCP, Name: "generated", Meta: map[string]any{"command": "echo", "disabled": true}}
	adapter, _ := adapters.Get("claude")
	cfg := &config.Config{}
	if err := adapter.Emit(adapters.NewSession(), spec.NewBundle([]spec.Entry{entry}), cfg, false); err != nil {
		t.Fatal(err)
	}
	if err := importFromClaude(root, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatal(err)
	}
	entry.Meta["disabled"] = false
	for range 3 {
		if err := adapter.Emit(adapters.NewSession(), spec.NewBundle([]spec.Entry{entry}), cfg, false); err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(readFile(t, filepath.Join(root, ".claude/settings.json"))), &doc); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(doc["disabledMcpjsonServers"], []any{"manual"}) {
			t.Errorf("rejections=%v", doc["disabledMcpjsonServers"])
		}
	}
}

func TestImportAuditRuleActivation(t *testing.T) {
	for _, tc := range []struct{ target, dir, fields string }{
		{"cline", ".clinerules", "paths: ['src/{a,b}/**', 'ui/**']"},
		{"cline", ".clinerules", "paths: []"},
		{"continue", ".continue/rules", "globs: ['src/{a,b}/**', 'ui/**']\nregex: '^import React'"},
		{"continue", ".continue/rules", "globs: []\nregex: ['^import', 'React']"},
		{"continue", ".continue/rules", "globs: 'src/{a,b}/**'\nregex: ['^import', 'React']"},
	} {
		t.Run(tc.target+tc.fields, func(t *testing.T) {
			root := t.TempDir()
			testutil.Chdir(t, root)
			writeFile(t, filepath.Join(root, tc.dir, "frontend.md"), "---\n"+tc.fields+"\n---\nGuide.\n")
			fn := importFromCline
			if tc.target == "continue" {
				fn = importFromContinue
			}
			if err := fn(root, rootSources()); err != nil {
				t.Fatal(err)
			}
			got, _ := splitMdcFrontmatter([]byte(readFile(t, filepath.Join(root, "rules/frontend.md"))))
			want, _ := splitMdcFrontmatter([]byte("---\n" + tc.fields + "\n---\n"))
			if !reflect.DeepEqual(got["x-"+tc.target], want) {
				t.Errorf("native activation = %#v, want %#v", got, want)
			}
			cfg := &config.Config{Sources: rootSources()}
			bundle, err := spec.LoadBundle(root, cfg)
			if err != nil {
				t.Fatal(err)
			}
			adapter, ok := adapters.Get(tc.target)
			if !ok {
				t.Fatalf("unknown target %s", tc.target)
			}
			if err := adapter.Emit(adapters.NewSession(), bundle, cfg, false); err != nil {
				t.Fatal(err)
			}
			emitted, _ := splitMdcFrontmatter([]byte(readFile(t, filepath.Join(root, tc.dir, "frontend.md"))))
			for key, value := range want {
				if !reflect.DeepEqual(emitted[key], value) {
					t.Errorf("round-trip %s = %#v, want %#v", key, emitted[key], value)
				}
			}
		})
	}
}

func TestImportAuditClaudeConfiguredRejectionSettings(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "mcps"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".mcp.json"), `{"mcpServers":{"server":{"command":"echo"}}}`)
	writeFile(t, filepath.Join(root, "config/claude/settings.json"), `{"disabledMcpjsonServers":["server"]}`)
	if _, err := importClaudeMCPWithSettings(root, filepath.Join(root, "mcps"), "config/claude"); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(root, "mcps/server.yaml"))
	if !strings.Contains(got, "disabled: true") {
		t.Errorf("missing disabled state: %s", got)
	}
}

func TestImportAuditClineEmptyPreferredRootAndIdenticalDuplicates(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".clinerules"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".cline/rules/same.md"), "Same.\n")
	if err := importFromCline(root, rootSources()); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(root, "rules/same.md")); !strings.Contains(got, "Same.") {
		t.Errorf("missing fallback rule: %s", got)
	}
	writeFile(t, filepath.Join(root, ".clinerules/same.md"), "Same.\n")
	if err := importFromCline(root, rootSources()); err != nil {
		t.Fatal(err)
	}
}

func TestImportAuditClineBothRootsAndReservedDirectories(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".clinerules/frontend.md"), "Frontend.\n")
	writeFile(t, filepath.Join(root, ".cline/rules/backend/nested.md"), "Backend.\n")
	for _, dir := range []string{"skills", "workflows", "hooks"} {
		writeFile(t, filepath.Join(root, ".clinerules", dir, "hidden.md"), "Not a rule.\n")
	}
	if err := importFromCline(root, rootSources()); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"frontend.md", "backend/nested.md"} {
		if _, err := os.Stat(filepath.Join(root, "rules", path)); err != nil {
			t.Error(err)
		}
	}
	for _, dir := range []string{"skills", "workflows", "hooks"} {
		if _, err := os.Stat(filepath.Join(root, "rules", dir, "hidden.md")); !os.IsNotExist(err) {
			t.Errorf("reserved %s imported: %v", dir, err)
		}
	}
}

func TestImportAuditClineRejectsDistinctRuleCollisions(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".clinerules/same.md"), "First.\n")
	writeFile(t, filepath.Join(root, ".cline/rules/same.md"), "Second.\n")
	err := importFromCline(root, rootSources())
	if err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("want collision error, got %v", err)
	}
}

func TestImportAuditCompatibleSkillRoots(t *testing.T) {
	for _, tc := range []struct {
		target string
		roots  []string
		fn     func(string, config.Sources) error
	}{
		{"junie", []string{".junie/skills", ".agents/skills"}, importFromJunie},
		{"warp", []string{".agents/skills", ".warp/skills", ".claude/skills", ".codex/skills", ".cursor/skills", ".gemini/skills", ".copilot/skills", ".factory/skills", ".github/skills", ".opencode/skills"}, importFromWarp},
		{"opencode", []string{".opencode/skills", ".claude/skills", ".agents/skills"}, importFromOpencode},
	} {
		t.Run(tc.target, func(t *testing.T) {
			root := t.TempDir()
			for _, native := range tc.roots {
				name := strings.Split(native, "/")[0][1:]
				writeFile(t, filepath.Join(root, native, name, "SKILL.md"), "Skill "+name+".\n")
				writeFile(t, filepath.Join(root, native, name, "assets/data.txt"), name)
				writeFile(t, filepath.Join(root, native, "duplicate/SKILL.md"), native)
				writeFile(t, filepath.Join(root, native, "duplicate/assets/data.txt"), native)
				if tc.target != "junie" {
					writeFile(t, filepath.Join(root, "backend", native, name, "SKILL.md"), "Scoped.\n")
				}
			}
			if err := tc.fn(root, rootSources()); err != nil {
				t.Fatal(err)
			}
			for _, native := range tc.roots {
				name := strings.Split(native, "/")[0][1:]
				if got := readFile(t, filepath.Join(root, "skills", name, "assets/data.txt")); got != name {
					t.Errorf("asset = %q", got)
				}
				if tc.target != "junie" {
					if _, err := os.Stat(filepath.Join(root, "skills/backend", name, "SKILL.md")); err != nil {
						t.Error(err)
					}
				}
			}
			for _, file := range []string{"SKILL.md", "assets/data.txt"} {
				if got := readFile(t, filepath.Join(root, "skills/duplicate", file)); got != tc.roots[0] {
					t.Errorf("precedence = %q, want %q", got, tc.roots[0])
				}
			}
		})
	}
}

func TestImportAuditClaudeDisabledMCP(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "mcps"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".mcp.json"), `{"mcpServers":{"off":{"command":"echo"},"on":{"command":"echo"}}}`)
	writeFile(t, filepath.Join(root, ".claude/settings.json"), `{"disabledMcpjsonServers":["off","other"]}`)
	if _, err := importClaudeMCP(root, filepath.Join(root, "mcps")); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(root, "mcps/off.yaml")); !strings.Contains(got, "disabled: true") {
		t.Errorf("disabled state lost: %s", got)
	}
	if got := readFile(t, filepath.Join(root, "mcps/on.yaml")); strings.Contains(got, "disabled:") {
		t.Errorf("enabled server changed: %s", got)
	}
}
