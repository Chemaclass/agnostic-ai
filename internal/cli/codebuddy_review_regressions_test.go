package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestCodebuddy_ImportRejectsNestedCredentialsBeforeWritingSpecs(t *testing.T) {
	cases := map[string]struct{ path, body, field string }{
		"settings":                    {".codebuddy/settings.json", `{"model":"lite","gateway":{"password":"credential-fixture-123"}}`, "gateway.password"},
		"agent-env":                   {".codebuddy/agents/reviewer.md", "---\nname: reviewer\ndescription: Review\nmcpServers:\n  - docs:\n      type: stdio\n      command: docs-server\n      env:\n        API_TOKEN: credential-fixture-123\n---\nReview.\n", "mcpServers[0].docs.env.API_TOKEN"},
		"agent-header":                {".codebuddy/agents/reviewer.md", "---\nname: reviewer\ndescription: Review\nmcpServers:\n  - docs:\n      type: http\n      url: https://example.com/mcp\n      headers:\n        Authorization: Bearer credential-fixture-123\n---\nReview.\n", "mcpServers[0].docs.headers.Authorization"},
		"settings-reference":          {".codebuddy/settings.json", `{"gateway":{"password":"${TOKEN}"}}`, "gateway.password"},
		"agent-foreign-url-reference": {".codebuddy/agents/reviewer.md", "---\nname: reviewer\ndescription: Review\nmcpServers:\n  - docs:\n      type: http\n      url: 'https://${env:HOST}/mcp'\n---\nReview.\n", "mcpServers[0].docs.url"},
		"settings-array":              {".codebuddy/settings.json", `{"gateway":[{"password":12345}]}`, "gateway[0].password"},
		"agent-argument":              {".codebuddy/agents/reviewer.md", "---\nname: reviewer\ndescription: Review\nmcpServers:\n  - docs:\n      type: stdio\n      command: docs-server\n      args: ['--password', 'credential-fixture-123']\n---\nReview.\n", "mcpServers[0].docs.args"},
		"agent-reference-default":     {".codebuddy/agents/reviewer.md", "---\nname: reviewer\ndescription: Review\nmcpServers:\n  - docs:\n      type: stdio\n      command: docs-server\n      env:\n        API_TOKEN: '${TOKEN:-credential-fixture-123}'\n---\nReview.\n", "mcpServers[0].docs.env.API_TOKEN"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			root := testutil.TempCwd(t)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codebuddy]\n")
			writeFile(t, c.path, c.body)
			out, err := runRoot(t, "import", "codebuddy")
			if err == nil {
				t.Fatal("credential import succeeded")
			}
			if !strings.Contains(err.Error(), c.field) {
				t.Errorf("diagnostic lacks field path: %v", err)
			}
			if strings.Contains(out+err.Error(), "credential-fixture-123") {
				t.Fatal("diagnostic exposed a literal")
			}
			for _, p := range []string{".agnostic-ai/settings/codebuddy.yaml", ".agnostic-ai/agents/reviewer.md"} {
				if _, err := os.Stat(filepath.Join(root, p)); !os.IsNotExist(err) {
					t.Errorf("source was written: %s", p)
				}
			}
		})
	}
}
func TestCodebuddy_ImportKeepsResolvableInlineMCPReferences(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codebuddy]\n")
	writeFile(t, ".codebuddy/agents/reviewer.md", "---\nname: reviewer\ndescription: Review\nmcpServers:\n  - docs:\n      type: http\n      url: https://example.com/mcp\n      headers:\n        Authorization: Bearer ${TOKEN}\n---\nReview.\n")
	if out, err := runRoot(t, "import", "codebuddy"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	data, err := os.ReadFile(".agnostic-ai/agents/reviewer.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Bearer ${TOKEN}") {
		t.Fatal("reference changed")
	}
}
func TestCodebuddy_GlobalRejectsInactiveOrUnsupportedHooks(t *testing.T) {
	for _, extra := range []string{"disabled: true\n", "type: prompt\n"} {
		t.Run(extra, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			mustWriteGlobalTest(t, filepath.Join(source, "hooks/guard.yaml"), "event: PreToolUse\ncommand: .agnostic-ai/scripts/guard.sh\n"+extra)
			mustWriteGlobalTest(t, filepath.Join(source, "scripts/guard.sh"), "#!/bin/sh\nexit 0\n")
			if _, _, err := runGlobalAgentTest("--only", "codebuddy"); err == nil {
				t.Fatal("inactive/unsupported hook became active")
			}
			for _, p := range []string{".codebuddy/settings.json", ".codebuddy/hooks/guard.sh"} {
				if _, err := os.Stat(filepath.Join(home, p)); !os.IsNotExist(err) {
					t.Errorf("active output written: %s", p)
				}
			}
		})
	}
}
func TestCodebuddy_ScopedInactiveRulesNeverBecomePlainInstructions(t *testing.T) {
	for _, control := range []string{"enabled: false", "alwaysApply: false"} {
		t.Run(control, func(t *testing.T) {
			testutil.TempCwd(t)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codebuddy]\n")
			writeFile(t, ".agnostic-ai/rules/backend/disabled.md", "---\nname: disabled\nscope: backend\nx-codebuddy:\n  "+control+"\n---\nINACTIVE_POLICY_BODY\n")
			mustSync(t)
			data, err := os.ReadFile("backend/CODEBUDDY.md")
			if err == nil && strings.Contains(string(data), "INACTIVE_POLICY_BODY") {
				meta, _ := splitMdcFrontmatter(data)
				if strings.HasPrefix(control, "enabled:") && meta["enabled"] != false || strings.HasPrefix(control, "alwaysApply:") && meta["alwaysApply"] != false {
					t.Fatal("inactive body became plain scoped instructions")
				}
			}
		})
	}
}
func TestCodebuddy_ImportPreservesInstructionActivation(t *testing.T) {
	for _, path := range []string{"CODEBUDDY.md", "backend/CODEBUDDY.md"} {
		for _, control := range []string{"enabled: false", "alwaysApply: false", "alwaysApply: false\npaths: ['**/*.go']", "paths: ['**/*.go']", "alwaysApply: true\npaths: ['**/*.go']"} {
			t.Run(path+control, func(t *testing.T) {
				testutil.TempCwd(t)
				silence(t)
				writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codebuddy]\n")
				writeFile(t, path, "---\n"+control+"\n---\nCONTROLLED_POLICY_BODY\n")
				if out, err := runRoot(t, "import", "codebuddy"); err != nil {
					t.Fatalf("import: %v\n%s", err, out)
				}
				if shared, _ := os.ReadFile(".agnostic-ai/AGNOSTIC_AI.md"); strings.Contains(string(shared), "CONTROLLED_POLICY_BODY") {
					t.Fatal("controlled native body became shared instructions")
				}
				mustSync(t)
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				meta, body := splitMdcFrontmatter(data)
				if !strings.Contains(body, "CONTROLLED_POLICY_BODY") {
					t.Fatal("controlled body lost")
				}
				if strings.Contains(control, "enabled:") && meta["enabled"] != false {
					t.Fatal("disabled activation lost")
				}
				if strings.Contains(control, "alwaysApply: false") && meta["alwaysApply"] != false {
					t.Fatal("conditional activation lost")
				}
				if strings.Contains(control, "alwaysApply: true") && meta["alwaysApply"] != true || strings.HasPrefix(control, "paths:") && meta["alwaysApply"] != nil {
					t.Fatal("native default activation changed")
				}
				stash := filepath.Join(t.TempDir(), "native")
				if err := os.Rename(path, stash); err != nil {
					t.Fatal(err)
				}
				mustSync(t)
				data, err = os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				meta, _ = splitMdcFrontmatter(data)
				if strings.Contains(control, "enabled:") && meta["enabled"] != false {
					t.Fatal("fresh disabled activation lost")
				}
				if strings.Contains(control, "alwaysApply: false") && meta["alwaysApply"] != false {
					t.Fatal("fresh conditional activation lost")
				}
				if strings.Contains(control, "paths:") && len(meta["paths"].([]any)) != 1 {
					t.Fatal("native paths changed")
				}
				if _, exists := meta["instruction-file"]; exists {
					t.Fatal("source placement hint leaked into native output")
				}
				if out, err := runRoot(t, "sync", "--check"); err != nil {
					t.Fatalf("check: %v\n%s", err, out)
				}
				if out, err := runRoot(t, "lint"); err != nil {
					t.Fatalf("lint: %v\n%s", err, out)
				}
				if out, err := runRoot(t, "import", "codebuddy"); err != nil {
					t.Fatalf("reimport: %v\n%s", err, out)
				}
				if out, err := runRoot(t, "sync", "--check"); err != nil {
					t.Fatalf("reimport check: %v\n%s", err, out)
				}
			})
		}
	}
}

func TestCodebuddy_ControlledRootRejectsEffectiveContributionsBeforeWrites(t *testing.T) {
	for _, contribution := range []string{"shared", "local", "rule", "overview", "other-reader"} {
		t.Run(contribution, func(t *testing.T) {
			testutil.TempCwd(t)
			silence(t)
			cfg := &config.Config{Targets: []string{"codebuddy"}}
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "native", Path: "rules/native.md", Meta: map[string]any{"x-codebuddy": map[string]any{"instruction-file": "CODEBUDDY.md", "enabled": false}}}})
			switch contribution {
			case "shared":
				writeAgnosticFile(t, "Shared policy.\n")
			case "local":
				writeFile(t, ".agnostic-ai/local/AGNOSTIC_AI.md", "Local policy.\n")
			case "rule":
				b.Rules = append(b.Rules, spec.Entry{Kind: spec.KindRule, Name: "plain", Body: "Plain policy."})
			case "overview":
				cfg.Sync.TargetOverview = true
			case "other-reader":
				cfg.Targets = append(cfg.Targets, "codex")
				cfg.Outputs = map[string]config.Output{"codex": {File: "CODEBUDDY.md"}}
			}
			before, beforeErr := os.ReadFile(".agnostic-ai/AGNOSTIC_AI.md")
			if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, b, cfg.Targets, false); err == nil {
				t.Fatalf("expected policy conflict, got %v", err)
			}
			after, afterErr := os.ReadFile(".agnostic-ai/AGNOSTIC_AI.md")
			if string(before) != string(after) || os.IsNotExist(beforeErr) != os.IsNotExist(afterErr) {
				t.Fatal("conflict changed or created the shared source")
			}
			if _, err := os.Stat("CODEBUDDY.md"); !os.IsNotExist(err) {
				t.Fatal("conflict wrote native output")
			}
		})
	}
}

func TestCodebuddy_ControlledRootAllowsOtherTargetFences(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codebuddy]\n")
	writeFile(t, "CODEBUDDY.md", "---\nenabled: false\n---\nControlled policy.\n")
	if out, err := runRoot(t, "import", "codebuddy"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	writeAgnosticFile(t, "::target claude\nClaude policy.\n::end\n")
	mustSync(t)
	data, err := os.ReadFile("CODEBUDDY.md")
	if err != nil || strings.Contains(string(data), "Claude policy.") || !strings.Contains(string(data), "enabled: false") {
		t.Fatalf("fenced content changed native policy: %v\n%s", err, data)
	}
}

func TestCodebuddy_InstructionAlternativesRequireExplicitSelection(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codebuddy]\n")
	writeFile(t, "CODEBUDDY.md", "Root policy.\n")
	writeFile(t, ".codebuddy/CODEBUDDY.md", "Hidden policy.\n")
	writeFile(t, ".codebuddy/rules/example.md", "Rule policy.\n")
	if _, err := runRoot(t, "import", "codebuddy"); err == nil || !strings.Contains(err.Error(), "outputs.codebuddy.file") {
		t.Fatalf("ambiguous instructions imported: %v", err)
	}
	for _, path := range []string{".agnostic-ai/AGNOSTIC_AI.md", ".agnostic-ai/rules/example.md"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("ambiguous import wrote %s", path)
		}
	}
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codebuddy]\noutputs:\n  codebuddy:\n    file: .codebuddy/CODEBUDDY.md\n")
	if out, err := runRoot(t, "import", "codebuddy"); err != nil {
		t.Fatalf("explicit import: %v\n%s", err, out)
	}
	data, err := os.ReadFile(".agnostic-ai/AGNOSTIC_AI.md")
	if err != nil || !strings.Contains(string(data), "Hidden policy.") || strings.Contains(string(data), "Root policy.") {
		t.Fatalf("explicit source selection lost: %v\n%s", err, data)
	}
}

func TestCodebuddy_ScopedPoliciesCannotActivateEachOther(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codebuddy]\n")
	writeFile(t, ".agnostic-ai/rules/plain.md", "---\nname: plain\nscope: backend\n---\nPlain policy.\n")
	writeFile(t, ".agnostic-ai/rules/disabled.md", "---\nname: disabled\nscope: backend\nx-codebuddy:\n  enabled: false\n---\nInactive policy.\n")
	if _, err := runRoot(t, "sync"); err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("mixed activation policies merged: %v", err)
	}
	if _, err := os.Stat("backend/CODEBUDDY.md"); !os.IsNotExist(err) {
		t.Fatal("mixed policies wrote an active instruction file")
	}
}
func TestCodebuddy_ImportFindsToolDirectoryInstructions(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codebuddy]\n")
	writeFile(t, ".codebuddy/CODEBUDDY.md", "Tool directory instructions.\n")
	if out, err := runRoot(t, "import", "codebuddy"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if err := os.Rename(".codebuddy/CODEBUDDY.md", filepath.Join(t.TempDir(), "original")); err != nil {
		t.Fatal(err)
	}
	mustSync(t)
	data, err := os.ReadFile(".codebuddy/CODEBUDDY.md")
	if err != nil || !strings.Contains(string(data), "Tool directory instructions.") {
		t.Fatal("alternative instructions lost")
	}
	if _, err := os.Stat("CODEBUDDY.md"); !os.IsNotExist(err) {
		t.Fatal("hidden native instructions gained a conflicting root file")
	}
	if out, err := runRoot(t, "sync", "--check"); err != nil {
		t.Fatalf("check: %v\n%s", err, out)
	}
}
