package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestCodebuddy_DefaultSyncPreservesExistingNumericAgentNames(t *testing.T) {
	root := testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\n")
	writeFile(t, ".agnostic-ai/agents/review2.md", "---\nname: review2\ndescription: Review changes\n---\nRead the complete patch.\n")

	if out, err := runRoot(t, "sync"); err != nil {
		t.Fatalf("default sync rejects an existing agent: %v\n%s", err, out)
	}
	if got := readFile(t, filepath.Join(root, ".claude/agents/review2.md")); !strings.Contains(got, "Read the complete patch.") {
		t.Error("default sync lost the existing agent")
	}
	if _, err := os.Stat(filepath.Join(root, ".codebuddy")); !os.IsNotExist(err) {
		t.Error("default sync wrote CodeBuddy output")
	}
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codebuddy]\n")
	if out, err := runRoot(t, "sync"); err == nil || !strings.Contains(out+err.Error(), "lowercase letters and hyphens") {
		t.Error("explicit CodeBuddy sync must enforce its native agent name limit")
	}
}

func TestCodebuddy_ImportToFreshOutputs(t *testing.T) {
	root := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: [codebuddy]\noutputs:\n  codebuddy:\n    root-mcp-file: .mcp.json\n")
	files := map[string]string{
		"CODEBUDDY.md":                                  "# Team\n\nCheck every change.\n",
		"backend/CODEBUDDY.md":                          "Keep backend changes local.\n",
		".codebuddy/rules/go.md":                        "---\nenabled: true\nalwaysApply: false\npaths: ['**/*.go']\n---\n\nCheck errors.\n",
		".codebuddy/agents/reviewer.md":                 "---\nname: reviewer\ndescription: Review code\ntools: Read, Grep\nmodel: lite\neffort: high\nmaxTurns: 3\nbackground: true\ninitialPrompt: Start by reading\n---\n\nReview changes.\n",
		".codebuddy/skills/review/SKILL.md":             "---\nname: review\ndescription: Review code\nallowed-tools: Read\ncontext: fork\nmodel: lite\nuser-invocable: false\n---\n\nRead references/guide.txt.\n",
		".codebuddy/skills/review/references/guide.txt": "Read the complete diff.\n",
		".codebuddy/commands/check.md":                  "---\ndescription: Check code\nargument-hint: files\nmodel: lite\n---\n\nCheck $ARGUMENTS.\n",
		".codebuddy/settings.json":                      `{"model":"lite","reasoningEffort":"high","nativeControl":true,"permissions":{"deny":["Bash(rm *)"],"defaultMode":"plan"},"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo guard","timeout":5}]}]}}`,
		".mcp.json":                                     `{"mcpServers":{"local":{"type":"stdio","command":"node","args":["server.js"],"defer_loading":true},"remote":{"type":"http","url":"https://example.com/mcp","headers":{"X-Mode":"test"},"tools":{"get":{"defer_loading":false}}}}}`,
	}
	for p, body := range files {
		writeFile(t, filepath.Join(root, p), body)
	}
	if out, err := runRoot(t, "import", "codebuddy"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	// Re-emission uses an empty tree, so unmanaged originals cannot hide dropped data.
	for _, p := range []string{".codebuddy", "backend/CODEBUDDY.md", "CODEBUDDY.md", ".mcp.json"} {
		if err := os.Rename(filepath.Join(root, p), filepath.Join(root, "saved-"+strings.ReplaceAll(p, "/", "-"))); err != nil {
			t.Fatal(err)
		}
	}
	mustSync(t)
	for p, body := range map[string]string{
		"CODEBUDDY.md": "Check every change.", "backend/CODEBUDDY.md": "Keep backend changes local.",
		".codebuddy/rules/go.md": "alwaysApply: false", ".codebuddy/agents/reviewer.md": "initialPrompt: Start by reading",
		".codebuddy/skills/review/SKILL.md": "user-invocable: false", ".codebuddy/skills/review/references/guide.txt": "Read the complete diff.",
		".codebuddy/commands/check.md": "Check $ARGUMENTS.", ".codebuddy/settings.json": "echo guard", ".mcp.json": "defer_loading",
	} {
		got, err := os.ReadFile(filepath.Join(root, p))
		if err != nil {
			t.Error(err)
			continue
		}
		if !strings.Contains(string(got), body) {
			t.Errorf("%s lost %q:\n%s", p, body, got)
		}
	}
	var settings map[string]any
	data, _ := os.ReadFile(filepath.Join(root, ".codebuddy/settings.json"))
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	if settings["nativeControl"] != true || settings["model"] != "lite" || settings["reasoningEffort"] != "high" {
		t.Errorf("settings: %s", data)
	}
	for _, args := range [][]string{{"sync", "--check"}, {"lint"}} {
		if out, err := runRoot(t, args...); err != nil {
			t.Errorf("%v: %v\n%s", args, err, out)
		}
	}
}
func TestCodebuddy_ScopedAndConditionalRulesStaySeparate(t *testing.T) {
	root := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: [codebuddy]\n")
	writeFile(t, filepath.Join(root, ".agnostic-ai/rules/root.md"), "---\nname: root\n---\n\nAlways here.\n")
	writeFile(t, filepath.Join(root, ".agnostic-ai/rules/backend/local.md"), "---\nname: local\nscope: backend\n---\n\nBackend only.\n")
	writeFile(t, filepath.Join(root, ".agnostic-ai/rules/go.md"), "---\nname: go\nglobs: '**/*.go'\n---\n\nGo only.\n")
	mustSync(t)
	data, err := os.ReadFile(filepath.Join(root, "CODEBUDDY.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Always here.") || strings.Contains(string(data), "Go only.") || strings.Contains(string(data), "Backend only.") {
		t.Errorf("root broadened rules:\n%s", data)
	}
	data, err = os.ReadFile(filepath.Join(root, "backend/CODEBUDDY.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Backend only.") {
		t.Fatal(string(data))
	}
	if _, err := os.Stat(filepath.Join(root, ".codebuddy/rules/root.md")); !os.IsNotExist(err) {
		t.Errorf("duplicate always-on rule: %v", err)
	}
	data, err = os.ReadFile(filepath.Join(root, ".codebuddy/rules/go.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "alwaysApply: false") {
		t.Fatal(string(data))
	}
}

func TestCodebuddy_PortableHookDecisionRunsAndImports(t *testing.T) {
	skipWithoutPOSIXShell(t)
	root := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: [codebuddy]\n")
	writeFile(t, filepath.Join(root, ".agnostic-ai/hooks/guard.yaml"), "name: guard\non: before-tool\nmatch: shell\ndecision: stdout\ncommand: .agnostic-ai/scripts/guard.sh\n")
	writeFile(t, filepath.Join(root, ".agnostic-ai/scripts/guard.sh"), "#!/bin/sh\nprintf '%s\\n' '{\"decision\":\"deny\",\"reason\":\"blocked\"}'\n")
	if err := os.Chmod(filepath.Join(root, ".agnostic-ai/scripts/guard.sh"), 0755); err != nil {
		t.Fatal(err)
	}
	mustSync(t)
	if out, err := runHookRun(t, "guard", "--bash", "git push", "--expect", "block", "--include-assumed"); err != nil {
		t.Fatalf("hook run: %v\n%s", err, out)
	}
	if err := os.Remove(filepath.Join(root, ".agnostic-ai/hooks/guard.yaml")); err != nil {
		t.Fatal(err)
	}
	if out, err := runRoot(t, "import", "codebuddy"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	got, err := os.ReadFile(filepath.Join(root, ".agnostic-ai/hooks/codebuddy-pretooluse-1.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "agnostic-ai-hook") || !strings.Contains(string(got), "decision: stdout") {
		t.Fatalf("wrapper copied into spec: %s", got)
	}
}

func TestCodebuddy_PromptHookRoundTrips(t *testing.T) {
	root := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: [codebuddy]\n")
	writeFile(t, filepath.Join(root, ".agnostic-ai/hooks/stop-check.yaml"), "name: stop-check\nevent: Stop\ntype: prompt\nprompt: 'Check tests pass: $ARGUMENTS'\ntimeout: 30\ncontinueOnBlock: true\n")
	mustSync(t)
	before, err := os.ReadFile(filepath.Join(root, ".codebuddy/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, ".agnostic-ai/hooks/stop-check.yaml")); err != nil {
		t.Fatal(err)
	}
	if out, err := runRoot(t, "import", "codebuddy"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	got, err := os.ReadFile(filepath.Join(root, ".agnostic-ai/hooks/codebuddy-stop-1.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"type: prompt", "prompt: 'Check tests pass: $ARGUMENTS'", "timeout: 30", "continueOnBlock: true"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("imported spec lacks %q:\n%s", want, got)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".agnostic-ai/settings/codebuddy.yaml")); err == nil {
		t.Error("prompt hook stayed under x-codebuddy")
	}
	mustSync(t)
	after, err := os.ReadFile(filepath.Join(root, ".codebuddy/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("settings changed after round trip:\nbefore %s\nafter  %s", before, after)
	}
}

func TestCodebuddy_DeleteCapabilityCannotBecomeInheritedTools(t *testing.T) {
	root := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: [codebuddy]\n")
	writeFile(t, filepath.Join(root, ".agnostic-ai/agents/reviewer.md"), "---\nname: reviewer\ncan: [delete]\n---\nReview.\n")
	if out, err := runRoot(t, "sync"); err == nil {
		t.Fatalf("delete became unrestricted tools: %s", out)
	}
}

func TestCodebuddy_OverridesMoveNativeFilesAndScripts(t *testing.T) {
	root := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: [codebuddy]\noutputs:\n  codebuddy:\n    file: native/CODEBUDDY.md\n    dir: native\n    rules-dir: conditional\n    agents-dir: agents\n    skills-dir: skills\n    commands-dir: commands\n    conf-file: settings.json\n    root-mcp-file: servers.json\n")
	files := map[string]string{
		".agnostic-ai/rules/go.md":            "---\nname: go\nglobs: '**/*.go'\n---\nGo only.\n",
		".agnostic-ai/agents/reviewer.md":     "---\nname: reviewer\ncan: [read]\n---\nReview.\n",
		".agnostic-ai/skills/check/SKILL.md":  "---\nname: check\ndescription: Check changes\n---\nCheck.\n",
		".agnostic-ai/commands/check.md":      "---\ndescription: Check changes\n---\nCheck.\n",
		".agnostic-ai/settings/defaults.yaml": "model: lite\n",
		".agnostic-ai/hooks/guard.yaml":       "event: PreToolUse\nmatcher: Bash\ncommand: .agnostic-ai/scripts/guard.sh\n",
		".agnostic-ai/scripts/guard.sh":       "#!/bin/sh\nexit 0\n",
		".agnostic-ai/mcps/docs.yaml":         "command: docs-mcp\n",
	}
	for p, body := range files {
		writeFile(t, filepath.Join(root, p), body)
	}
	mustSync(t)
	for _, p := range []string{"native/CODEBUDDY.md", "conditional/go.md", "agents/reviewer.md", "skills/check/SKILL.md", "commands/check.md", "settings.json", "servers.json", "native/hooks/guard.sh"} {
		data, err := os.ReadFile(filepath.Join(root, p))
		if err != nil || len(data) == 0 {
			t.Errorf("missing output %s: %v", p, err)
		}
	}
	got, err := os.ReadFile(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "native/hooks/guard.sh") {
		t.Fatal(string(got))
	}
	if out, err := runRoot(t, "sync", "--check"); err != nil {
		t.Fatalf("check: %v\n%s", err, out)
	}
}

func TestCodebuddy_ImportKeepsNativeSkillToolPatternsScoped(t *testing.T) {
	root := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: [codebuddy]\n")
	writeFile(t, filepath.Join(root, ".codebuddy/skills/check/SKILL.md"), "---\nname: check\ndescription: Check changes\nallowed-tools: Bash(git *)\ncontext: fork\nmodel: lite\n---\nCheck changes.\n")
	if out, err := runRoot(t, "import", "codebuddy"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(root, ".agnostic-ai/skills/check/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	meta, _ := splitMdcFrontmatter(data)
	if meta["model"] != nil || meta["allowed-tools"] != nil {
		t.Fatalf("native fields leaked to other targets: %s", data)
	}
	mustSync(t)
	data, err = os.ReadFile(filepath.Join(root, ".codebuddy/skills/check/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"allowed-tools: Bash(git *)", "model: lite"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("lost native field %s: %s", want, data)
		}
	}
}
func TestCodebuddy_ImportPreservesSharedModelForOtherTargets(t *testing.T) {
	root := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: [codebuddy]\n")
	writeFile(t, filepath.Join(root, ".agnostic-ai/skills/check/SKILL.md"), "---\nname: check\ndescription: Check changes\nmodel:\n  claude: sonnet\n  codebuddy: lite\nx-codebuddy:\n  context: fork\n---\nCheck changes.\n")
	mustSync(t)
	path := filepath.Join(root, ".codebuddy/skills/check/SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, strings.Replace(string(data), "model: lite", "model: reasoning", 1))
	if out, err := runRoot(t, "import", "codebuddy", "--overwrite"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	data, err = os.ReadFile(filepath.Join(root, ".agnostic-ai/skills/check/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	meta, _ := splitMdcFrontmatter(data)
	models, _ := meta["model"].(map[string]any)
	if models["claude"] != "sonnet" {
		t.Fatalf("other target model lost: %s", data)
	}
	mustSync(t)
	data, err = os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "model: reasoning") {
		t.Fatalf("native model edit lost: %s, %v", data, err)
	}
}

func TestCodebuddy_ImportLeavesEnvironmentSettingsNative(t *testing.T) {
	root := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: [codebuddy]\n")
	writeFile(t, filepath.Join(root, ".codebuddy/settings.json"), `{"model":"lite","env":{"EXAMPLE_MODE":"private"},"apiKeyHelper":"example-key-helper","nativeControl":true}`)
	if out, err := runRoot(t, "import", "codebuddy"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(root, ".agnostic-ai/settings/codebuddy.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "EXAMPLE_MODE") || strings.Contains(string(data), "example-key-helper") {
		t.Fatalf("environment settings copied: %s", data)
	}
	if !strings.Contains(string(data), "nativeControl") {
		t.Fatal("native controls lost")
	}
}

func TestCodebuddy_ImportEffortPlansAgainstOtherSettingsSpecs(t *testing.T) {
	cases := []struct {
		name, other, wantSpec, missing string
	}{
		{"another spec sets a different effort", "effort: high\n", "x-codebuddy:\n    nativeControl: true\n    reasoningEffort: low\n", "effort:"},
		{"no other spec sets effort", "model: lite\n", "effort: low\n", "reasoningEffort"},
		{"another spec already resolves to the same effort", "effort: low\n", "x-codebuddy:\n    nativeControl: true\n", "ffort"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := testutil.TempCwd(t)
			silence(t)
			writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: [codebuddy]\n")
			writeFile(t, filepath.Join(root, ".agnostic-ai/settings/default.yaml"), tc.other)
			writeFile(t, filepath.Join(root, ".codebuddy/settings.json"), `{"reasoningEffort":"low","nativeControl":true}`)
			if out, err := runRoot(t, "import", "codebuddy"); err != nil {
				t.Fatalf("import: %v\n%s", err, out)
			}
			imported, err := os.ReadFile(filepath.Join(root, ".agnostic-ai/settings/codebuddy.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(imported), tc.wantSpec) || strings.Contains(string(imported), tc.missing) {
				t.Errorf("settings/codebuddy.yaml wants %q without %q:\n%s", tc.wantSpec, tc.missing, imported)
			}
			mustSync(t)
			var settings map[string]any
			data, _ := os.ReadFile(filepath.Join(root, ".codebuddy/settings.json"))
			if err := json.Unmarshal(data, &settings); err != nil {
				t.Fatal(err)
			}
			if settings["reasoningEffort"] != "low" {
				t.Errorf("reasoningEffort = %v, want low: %s", settings["reasoningEffort"], data)
			}
			if out, err := runRoot(t, "sync", "--check"); err != nil {
				t.Errorf("sync --check: %v\n%s", err, out)
			}
		})
	}
}
