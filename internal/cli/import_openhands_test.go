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
// multi-command hook group, one MCP server per `[mcp]` array, and an
// environment whose `install` becomes `.openhands/setup.sh`.
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
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "fs.yaml"),
		"name: fs\ncommand: npx\nargs:\n  - -y\n  - \"@modelcontextprotocol/server-filesystem\"\nenv:\n  ROOT: /tmp\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "docs.yaml"),
		"name: docs\ntype: sse\nurl: https://docs.example.test/sse\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "search.yaml"),
		"name: search\ntype: http\nurl: https://search.example.test/mcp\napi_key: secret\ntimeout: 1800\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "notion.yaml"),
		"name: notion\ntype: http\nurl: https://mcp.notion.com/mcp\noauth: true\n")
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
		".agents/agents/reviewer.md", ".openhands/hooks.json", ".openhands/setup.sh", "config.toml",
	} {
		if _, ok := first[want]; !ok {
			t.Fatalf("first sync wrote no %s: %v", want, keys(first))
		}
	}
	if !strings.Contains(first["config.toml"], `{ url = "https://mcp.notion.com/mcp", auth = "oauth" }`) {
		t.Fatalf("first sync did not emit the oauth shttp entry:\n%s", first["config.toml"])
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
	notion := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "mcp-notion-com.yaml"))
	if !strings.Contains(notion, "auth: oauth") {
		t.Errorf("oauth shttp entry not reconstructed with auth: oauth:\n%s", notion)
	}

	execCLI(t, "sync", "-t", "openhands")
	second := snapshotEmitted(t, dir)
	assertEmittedEqual(t, first, second)
	if len(second) != len(first) {
		t.Errorf("re-emit wrote %d files, first emit wrote %d: %v", len(second), len(first), keys(second))
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
	for _, want := range []string{"event: Stop", "event: PreToolUse", "command: .openhands/hooks/quality_gate.sh", "timeout: 120"} {
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
