package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// seedOpenhandsBundle writes one spec per kind the openhands adapter
// emits: an always-on rule, a path-triggered rule, an agent, a skill, a
// multi-command hook group, and an environment whose `install` becomes
// `.openhands/setup.sh`.
func seedOpenhandsBundle(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [openhands]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"),
		"---\nname: style\n---\n\nKeep functions short.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "api.md"),
		"---\nname: api\npaths:\n  - src/api/**/*.ts\n  - '**/*.route.ts'\n---\n\nValidate every request with zod.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "reviewer.md"),
		"---\nname: reviewer\ndescription: Reviews diffs\nmodel: sonnet\n---\n\nReview the diff.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "release", "SKILL.md"),
		"---\nname: release\ndescription: Cut a release\n---\n\nTag and publish.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "gate.yaml"),
		"name: gate\nevent: Stop\nmatcher: \"*\"\ncommand:\n  - make lint\n  - make test\ntimeout: 120\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"),
		"name: guard\nevent: PreToolUse\nmatcher: terminal\ncommand: .openhands/hooks/guard.sh\nasync: true\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "environments", "dev.yaml"),
		"name: dev\ninstall: |\n  go mod download\n  make tools\n")
}

// TestImportOpenhands_RoundTripFixedPoint emits every kind the
// openhands adapter writes, wipes the specs, imports the tree back, and
// re-emits. The second emit must byte-match the first.
func TestImportOpenhands_RoundTripFixedPoint(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	seedOpenhandsBundle(t, dir)

	execCLI(t, "sync", "-t", "openhands")
	first := snapshotEmitted(t, dir)
	for _, want := range []string{
		".agents/skills/api/SKILL.md", ".agents/skills/release/SKILL.md",
		".agents/agents/reviewer.md", ".openhands/hooks.json", ".openhands/setup.sh",
	} {
		if _, ok := first[want]; !ok {
			t.Fatalf("first sync wrote no %s: %v", want, keys(first))
		}
	}

	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai")); err != nil {
		t.Fatalf("wipe source specs: %v", err)
	}
	execCLI(t, "import", "openhands")

	api := readFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "api.md"))
	if !strings.Contains(api, "src/api/**/*.ts") || !strings.Contains(api, "Validate every request with zod.") {
		t.Errorf("path-triggered rule not reconstructed as a rule:\n%s", api)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "skills", "api")); !os.IsNotExist(err) {
		t.Errorf("path-triggered rule also imported as a skill: %v", err)
	}
	env := readFile(t, filepath.Join(dir, ".agnostic-ai", "environments", "openhands-setup.yaml"))
	if !strings.Contains(env, "go mod download") || strings.Contains(env, "#!/bin/bash") {
		t.Errorf("setup script not reconstructed as an install body:\n%s", env)
	}
	execCLI(t, "sync", "-t", "openhands")
	second := snapshotEmitted(t, dir)
	assertEmittedEqual(t, first, second)
	if len(second) != len(first) {
		t.Errorf("re-emit wrote %d files, first emit wrote %d: %v", len(second), len(first), keys(second))
	}
}

// Sync no longer writes config.toml, but a team on legacy OpenHands (V0)
// still has one, so import keeps reading its `[mcp]` table into specs
// (#1259). A remote element has no name and takes its URL host.
func TestImportOpenhands_ReadsLegacyConfigTOMLMCP(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [openhands]\n")
	writeFile(t, filepath.Join(dir, "config.toml"), `[core]
workspace_base = "./workspace"

[mcp]
sse_servers = ["https://docs.example.test/sse"]
shttp_servers = [{ url = "https://search.example.test/mcp", api_key = "secret", timeout = 1800 }, { url = "https://mcp.notion.com/mcp", auth = "oauth" }]

[[mcp.stdio_servers]]
name = "fs"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-filesystem"]
env = { ROOT = "/tmp" }
`)

	execCLI(t, "import", "openhands")

	mcps := filepath.Join(dir, ".agnostic-ai", "mcps")
	for file, wants := range map[string][]string{
		"fs.yaml":                  {"command: npx", "@modelcontextprotocol/server-filesystem", "ROOT: ${ROOT}"},
		"docs-example-test.yaml":   {"type: sse", "url: https://docs.example.test/sse"},
		"search-example-test.yaml": {"type: http", "api_key: secret", "timeout: 1800"},
		"mcp-notion-com.yaml":      {"type: http", "auth: oauth"},
	} {
		got := readFile(t, filepath.Join(mcps, file))
		for _, want := range wants {
			if !strings.Contains(got, want) {
				t.Errorf("%s lacks %q:\n%s", file, want, got)
			}
		}
	}
}

// OpenHands documents snake_case event keys with no wrapper as its own
// layout, and the Claude shape as equally supported. A hand-written
// file in the native shape must import with PascalCase events.
func TestImportOpenhands_ReadsNativeSnakeCaseHooks(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [openhands]\n")
	writeFile(t, filepath.Join(dir, ".openhands", "hooks.json"), `{
  "stop": [{"matcher": "*", "hooks": [{"command": ".openhands/hooks/quality_gate.sh", "timeout": 120}]}],
  "pre_tool_use": [{"matcher": "terminal", "hooks": [{"type": "command", "command": "./guard.sh"}]}]
}`)

	execCLI(t, "import", "openhands")

	hooks := readDirNames(t, filepath.Join(dir, ".agnostic-ai", "hooks"))
	if len(hooks) != 2 {
		t.Fatalf("want 2 hook specs, got %v", hooks)
	}
	all := ""
	for _, h := range hooks {
		all += readFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", h))
	}
	for _, want := range []string{"event: Stop", "on: before-tool", "match: shell", "command: .openhands/hooks/quality_gate.sh", "timeout: 120"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in imported hooks:\n%s", want, all)
		}
	}
}

// OpenHands still reads the legacy `.openhands/microagents/` and
// `.openhands/skills/` trees. A flat file loads in full unless it
// carries `triggers` (keyword skill) or `paths` (path-triggered rule).
func TestImportOpenhands_ReadsLegacyMicroagents(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [openhands]\n")
	writeFile(t, filepath.Join(dir, ".openhands", "microagents", "repo.md"),
		"---\nagent: CodeActAgent\n---\n\nRun make test before committing.\n")
	writeFile(t, filepath.Join(dir, ".openhands", "microagents", "docker.md"),
		"---\ntriggers:\n  - docker\n  - container\n---\n\nUse docker compose v2.\n")
	writeFile(t, filepath.Join(dir, ".openhands", "skills", "migrations.md"),
		"---\npaths: db/migrations/**\n---\n\nKeep migrations reversible.\n")
	writeFile(t, filepath.Join(dir, ".openhands", "skills", "deploy", "SKILL.md"),
		"---\nname: deploy\ndescription: Deploy the app\n---\n\nRun the deploy script.\n")

	execCLI(t, "import", "openhands")

	repo := readFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "repo.md"))
	if !strings.Contains(repo, "Run make test before committing.") || strings.Contains(repo, "paths") {
		t.Errorf("always-on microagent not imported as an unscoped rule:\n%s", repo)
	}
	migrations := readFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "migrations.md"))
	if !strings.Contains(migrations, "db/migrations/**") {
		t.Errorf("path-triggered legacy file lost its paths:\n%s", migrations)
	}
	docker := readFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "docker", "SKILL.md"))
	for _, want := range []string{"name: docker", "x-openhands:", "- docker", "Use docker compose v2."} {
		if !strings.Contains(docker, want) {
			t.Errorf("keyword microagent missing %q:\n%s", want, docker)
		}
	}
	deploy := readFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "deploy", "SKILL.md"))
	if !strings.Contains(deploy, "Run the deploy script.") {
		t.Errorf("legacy skill folder not imported:\n%s", deploy)
	}
}

// `.agents/skills/` wins over the legacy trees on a name clash, the
// precedence OpenHands itself applies.
func TestImportOpenhands_AgentsSkillsWinOverLegacy(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [openhands]\n")
	writeFile(t, filepath.Join(dir, ".agents", "skills", "deploy", "SKILL.md"),
		"---\nname: deploy\ndescription: Deploy\n---\n\nCurrent steps.\n")
	writeFile(t, filepath.Join(dir, ".openhands", "skills", "deploy", "SKILL.md"),
		"---\nname: deploy\ndescription: Deploy\n---\n\nLegacy steps.\n")

	execCLI(t, "import", "openhands")

	got := readFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "deploy", "SKILL.md"))
	if !strings.Contains(got, "Current steps.") {
		t.Errorf("legacy skill overrode .agents/skills:\n%s", got)
	}
}

func TestImportOpenhands_DryRunMatchesRealImport(t *testing.T) {
	assertDryRunMatchesRealImport(t, "openhands", func(t *testing.T, dir string) {
		seedOpenhandsBundle(t, dir)
		execCLI(t, "sync", "-t", "openhands")
		if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai")); err != nil {
			t.Fatal(err)
		}
	})
}

func readDirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestImportOpenhands_PreservesAgentSkills(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [openhands]\n")
	path := filepath.Join(dir, ".agents/agents/reviewer.md")
	writeFile(t, path, "---\nname: reviewer\ndescription: Review code\nskills: [security-review]\n---\n\nReview changes.\n")
	execCLI(t, "import", "openhands")
	execCLI(t, "sync", "-t", "openhands")
	got := readFile(t, path)
	if !strings.Contains(got, "security-review") {
		t.Errorf("missing skill in %s", got)
	}
	execCLI(t, "sync", "--check", "-t", "openhands")
	execCLI(t, "lint")
	writeFile(t, path, "---\nname: reviewer\ndescription: Review code\n---\n\nReview changes.\n")
	execCLI(t, "import", "openhands")
	execCLI(t, "sync", "-t", "openhands")
	if got := readFile(t, path); strings.Contains(got, "security-review") {
		t.Errorf("deleted native field returned: %s", got)
	}
}

func TestImportOpenhands_PreservesAgentToolsPermissionModeAndColor(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [openhands]\n")
	path := filepath.Join(dir, ".agents/agents/editor.md")
	writeFile(t, path, "---\nname: editor\ndescription: Edit files\ntools: [file_editor]\npermission_mode: always_confirm\ncolor: blue\n---\n\nEdit files.\n")
	execCLI(t, "import", "openhands")
	execCLI(t, "sync", "-t", "openhands")
	got := readFile(t, path)
	for _, want := range []string{"file_editor", "permission_mode: always_confirm", "color: blue"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	execCLI(t, "sync", "--check", "-t", "openhands")
	execCLI(t, "lint")
	writeFile(t, path, "---\nname: editor\ndescription: Edit files\n---\n\nEdit files.\n")
	execCLI(t, "import", "openhands")
	execCLI(t, "sync", "-t", "openhands")
	got = readFile(t, path)
	for _, gone := range []string{"file_editor", "permission_mode", "color"} {
		if strings.Contains(got, gone) {
			t.Errorf("deleted native field %q returned: %s", gone, got)
		}
	}
}

func TestImportOpenhands_PreservesScopedNullModel(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [openhands]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/agents/reviewer.md"), "---\nname: reviewer\ndescription: Review code\nmodel: shared-model\nx-openhands:\n  model: null\n  skills: [review]\n---\n\nReview changes.\n")
	path := filepath.Join(dir, ".agents/agents/reviewer.md")
	writeFile(t, path, "---\nname: reviewer\ndescription: Review code\nskills: [review]\n---\n\nReview changes.\n")
	execCLI(t, "import", "openhands", "--overwrite")
	execCLI(t, "sync", "-t", "openhands")
	if got := readFile(t, path); strings.Contains(got, "shared-model") {
		t.Errorf("scoped null model was lost: %s", got)
	}
}

func TestImportOpenhands_KeepsFolderSkillTriggers(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [openhands]\n")
	skill := filepath.Join(dir, ".agents", "skills", "inspect")
	writeFile(t, filepath.Join(skill, "SKILL.md"),
		"---\nname: inspect\ndescription: Inspect the build\ntriggers:\n  - inspect\n---\n\nLook at the build.\n")
	writeFile(t, filepath.Join(skill, "scripts", "check.sh"), "echo ok\n")

	execCLI(t, "import", "openhands")

	spec := readFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "inspect", "SKILL.md"))
	if !strings.Contains(spec, "x-openhands:") || !strings.Contains(spec, "- inspect") {
		t.Errorf("triggers not kept under x-openhands:\n%s", spec)
	}
	if strings.Contains(strings.SplitN(spec, "x-openhands:", 2)[0], "triggers:") {
		t.Errorf("triggers left at the top level:\n%s", spec)
	}
	if got := readFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "inspect", "scripts", "check.sh")); got != "echo ok\n" {
		t.Errorf("bundled asset changed: %q", got)
	}

	execCLI(t, "sync", "-t", "openhands")
	native := readFile(t, filepath.Join(skill, "SKILL.md"))
	if !strings.Contains(native, "triggers:") || !strings.Contains(native, "- inspect") {
		t.Errorf("sync dropped triggers:\n%s", native)
	}
	execCLI(t, "sync", "--check", "-t", "openhands")
	execCLI(t, "lint")
	writeFile(t, filepath.Join(skill, "SKILL.md"),
		"---\nname: inspect\ndescription: Inspect the build\n---\n\nLook at the build.\n")
	execCLI(t, "import", "openhands")
	execCLI(t, "sync", "-t", "openhands")
	if got := readFile(t, filepath.Join(skill, "SKILL.md")); strings.Contains(got, "triggers") {
		t.Errorf("deleted triggers returned:\n%s", got)
	}
}

func TestImportOpenhands_PreservesAgentHooksAndMaxIterations(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [openhands]\n")
	path := filepath.Join(dir, ".agents/agents/runner.md")
	writeFile(t, path, "---\nname: runner\ndescription: Run tasks\nmax_iteration_per_run: 7\nmax_budget_per_run: 1.5\nprofile_store_dir: profiles\ncondenser:\n  type: llm\nmcp_config:\n  mcpServers: {}\nhooks:\n  stop:\n    - matcher: \"*\"\n      hooks:\n        - command: ./check.sh\n---\n\nRun tasks.\n")
	execCLI(t, "import", "openhands")
	execCLI(t, "sync", "-t", "openhands")
	got := readFile(t, path)
	for _, want := range []string{"max_iteration_per_run: 7", "hooks:", "./check.sh", "max_budget_per_run: 1.5", "profile_store_dir: profiles", "condenser:", "mcp_config:"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	execCLI(t, "sync", "--check", "-t", "openhands")
	execCLI(t, "lint")
	writeFile(t, path, "---\nname: runner\ndescription: Run tasks\nmax_iteration_per_run: 7\nmax_budget_per_run: 1.5\n---\n\nRun tasks.\n")
	execCLI(t, "import", "openhands")
	execCLI(t, "sync", "-t", "openhands")
	got = readFile(t, path)
	for _, gone := range []string{"hooks", "check.sh", "profile_store_dir", "condenser", "mcp_config"} {
		if strings.Contains(got, gone) {
			t.Errorf("deleted native field %q returned: %s", gone, got)
		}
	}
	for _, want := range []string{"max_iteration_per_run: 7", "max_budget_per_run: 1.5"} {
		if !strings.Contains(got, want) {
			t.Errorf("kept field %q lost in %s", want, got)
		}
	}
	writeFile(t, path, "---\nname: runner\ndescription: Run tasks\n---\n\nRun tasks.\n")
	execCLI(t, "import", "openhands")
	execCLI(t, "sync", "-t", "openhands")
	got = readFile(t, path)
	for _, gone := range []string{"max_iteration_per_run", "hooks", "check.sh", "max_budget_per_run", "profile_store_dir", "condenser", "mcp_config"} {
		if strings.Contains(got, gone) {
			t.Errorf("deleted native field %q returned: %s", gone, got)
		}
	}
}
