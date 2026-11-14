package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// execCLI drives one agnostic-ai command in-process, failing the test on
// error. Output is expected to be silenced by the caller.
func execCLI(t *testing.T, args ...string) {
	t.Helper()
	root := NewRootCmd("test")
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
}

// snapshotEmitted reads every emitted file under root (skipping the
// `.agnostic-ai/` source tree and agnostic-ai.yaml) into a map keyed by
// slash-form relative path, so two syncs can be compared for a
// round-trip fixed point.
func snapshotEmitted(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "agnostic-ai.yaml" || strings.HasPrefix(rel, ".agnostic-ai/") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

// assertEmittedEqual fails the test when two emitted-tree snapshots
// differ, naming the first offending path.
func assertEmittedEqual(t *testing.T, want, got map[string]string) {
	t.Helper()
	for rel, wantBody := range want {
		gotBody, ok := got[rel]
		if !ok {
			t.Errorf("re-emit dropped %s", rel)
			continue
		}
		if gotBody != wantBody {
			t.Errorf("re-emit diverged at %s\n--- first ---\n%s\n--- second ---\n%s",
				rel, wantBody, gotBody)
		}
	}
	for rel := range got {
		if _, ok := want[rel]; !ok {
			t.Errorf("re-emit added %s", rel)
		}
	}
}

// TestImportKiro_RoundTripFixedPoint emits a kit-sink bundle to kiro,
// wipes the source specs, imports the emitted tree back, then re-emits.
// The second emit must byte-match the first: import reconstructs every
// spec the kiro adapter can carry.
func TestImportKiro_RoundTripFixedPoint(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [kiro]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r-go.md"),
		"---\nname: r-go\nglobs: \"**/*.go\"\n---\n\nGo rule body here.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r-always.md"),
		"---\nname: r-always\n---\n\nAlways rule body here.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "my-agent.md"),
		"---\nname: my-agent\nx-kiro:\n  name: Display Agent\n---\n\nAgent body here.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "my-skill", "SKILL.md"),
		"---\nname: my-skill\ndescription: An example skill\n---\n\nSkill body here.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "stdio-server.yaml"),
		"name: stdio-server\ncommand: npx\nargs:\n  - -y\n  - \"@modelcontextprotocol/server-filesystem\"\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "http-server.yaml"),
		"name: http-server\ntype: http\nurl: https://example.test/mcp\n")

	execCLI(t, "sync", "-t", "kiro")
	first := snapshotEmitted(t, dir)
	if _, ok := first[".kiro/steering/r-go.md"]; !ok {
		t.Fatalf("first emit produced no kiro steering files: %v", keys(first))
	}
	if _, ok := first[".kiro/agents/my-agent.md"]; !ok {
		t.Fatalf("first emit produced no native kiro agent file: %v", keys(first))
	}
	if _, ok := first[".kiro/skills/my-skill/SKILL.md"]; !ok {
		t.Fatalf("first emit produced no native kiro skill file: %v", keys(first))
	}

	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai")); err != nil {
		t.Fatalf("wipe source specs: %v", err)
	}
	execCLI(t, "import", "kiro")

	// Rules: fileMatch -> globs, always -> unscoped.
	rGo := readFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r-go.md"))
	if !strings.Contains(rGo, "globs:") || !strings.Contains(rGo, "**/*.go") {
		t.Errorf("r-go rule lost its glob on import:\n%s", rGo)
	}
	if !strings.Contains(rGo, "Go rule body here.") {
		t.Errorf("r-go rule lost its body:\n%s", rGo)
	}
	rAlways := readFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r-always.md"))
	if strings.Contains(rAlways, "globs:") {
		t.Errorf("always rule should be unscoped (no globs):\n%s", rAlways)
	}
	// Agent round-trips through the native `.kiro/agents/` surface. The
	// filename remains the canonical identity.
	agent := readFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "my-agent.md"))
	if !strings.Contains(agent, "Agent body here.") || !strings.Contains(agent, "x-kiro:") || !strings.Contains(agent, "name: Display Agent") {
		t.Errorf("agent not reconstructed:\n%s", agent)
	}
	skill := readFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "my-skill", "SKILL.md"))
	if !strings.Contains(skill, "description: An example skill") {
		t.Errorf("skill lost its description:\n%s", skill)
	}
	// MCP: mcpServers stdio (no type) + http (type http).
	stdio := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "stdio-server.yaml"))
	if !strings.Contains(stdio, "command: npx") {
		t.Errorf("stdio mcp not reconstructed:\n%s", stdio)
	}
	httpMCP := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "http-server.yaml"))
	if !strings.Contains(httpMCP, "type: http") || !strings.Contains(httpMCP, "url: https://example.test/mcp") {
		t.Errorf("http mcp not reconstructed:\n%s", httpMCP)
	}

	execCLI(t, "sync", "-t", "kiro")
	second := snapshotEmitted(t, dir)
	assertEmittedEqual(t, first, second)
}

func TestImportKiroAgent_PreservesDisplayNameSeparatelyFromPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, kiroAgentsDir, "safe-id.md"), "---\nname: Display Name\ndescription: Reviews code.\n---\n\nReview changes.\n")
	dst := filepath.Join(dir, ".agnostic-ai/agents")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	count, err := importKiroAgents(dir, dst, nil)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	got := readFile(t, filepath.Join(dst, "safe-id.md"))
	for _, want := range []string{"x-kiro:", "name: Display Name", "description: Reviews code.", "Review changes."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in imported agent:\n%s", want, got)
		}
	}
	if strings.HasPrefix(got, "---\nname: Display Name") {
		t.Errorf("display name replaced the canonical filename identity:\n%s", got)
	}
}

func TestImportFromKiro_PreservesPromptTemplatesInConfiguredCommandsSource(t *testing.T) {
	for _, layout := range []string{"default", "relative", "absolute"} {
		t.Run(layout, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			silence(t)
			src := rootSources()
			switch layout {
			case "relative":
				src.Commands = "portable/prompts"
			case "absolute":
				src.Commands = filepath.Join(t.TempDir(), "prompts")
			}
			const body = "# Review\n\nReview ${1} with ${10}; full input: $ARGUMENTS or ${@}.\n"
			writeFile(t, filepath.Join(dir, ".kiro/prompts/review.md"), body)
			writeFile(t, filepath.Join(dir, ".kiro/prompts/notes.txt"), "not a command\n")
			writeFile(t, filepath.Join(dir, ".kiro/prompts/nested/ignored.md"), "not a top-level command\n")

			if err := importFromKiro(dir, &config.Config{Sources: src}); err != nil {
				t.Fatal(err)
			}
			dst := importSourcePath(dir, src.Commands)
			got := readFile(t, filepath.Join(dst, "review.md"))
			if got != body {
				t.Errorf("prompt templates changed on import:\ngot:\n%s\nwant:\n%s", got, body)
			}
			entries, err := os.ReadDir(dst)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 {
				t.Errorf("imported %d entries, want only review.md", len(entries))
			}
		})
	}
}

func TestImportFromKiro_MissingPromptsDirectoryImportsNoCommands(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	if err := importFromKiro(dir, &config.Config{Sources: rootSources()}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "commands"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("missing prompts directory imported %d commands", len(entries))
	}
}

func TestImportFromKiro_LeadingYAMLRemainsLiteralPromptBody(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	src := rootSources()
	const body = "---\nname: other\n---\nReview $ARGUMENTS.\n"
	writeFile(t, filepath.Join(dir, ".kiro/prompts/review.md"), body)
	if err := importFromKiro(dir, &config.Config{Sources: src}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, src.Commands, "review.md")); err != nil {
		t.Fatalf("missing filename-based command source: %v", err)
	}
	bundle, err := spec.LoadLayered([]spec.Layer{{Root: dir, Sources: src}})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Commands) != 1 {
		t.Fatalf("imported %d commands, want 1", len(bundle.Commands))
	}
	command := bundle.Commands[0]
	if command.Name != "review" {
		t.Errorf("literal YAML changed command identity: got %q, want review", command.Name)
	}
	if command.Body != body {
		t.Errorf("literal YAML was consumed as portable frontmatter:\ngot:\n%s\nwant:\n%s", command.Body, body)
	}
}

func TestImportFromKiro_GeneratedPromptsPreserveLeadingBlankLines(t *testing.T) {
	for _, tc := range []struct {
		name, leading string
	}{
		{"none", ""},
		{"one", "\n"},
		{"three", "\n\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			silence(t)
			src := rootSources()
			body := tc.leading + "# Review\nReview $ARGUMENTS.\n"
			generated := header.Line(header.FormatMarkdown) + "\n" + body
			writeFile(t, filepath.Join(dir, ".kiro/prompts/review.md"), generated)
			if err := importFromKiro(dir, &config.Config{Sources: src}); err != nil {
				t.Fatal(err)
			}
			got := readFile(t, filepath.Join(dir, src.Commands, "review.md"))
			if got != body {
				t.Errorf("import changed leading body blank lines: got %q, want %q", got, body)
			}
			bundle, err := spec.LoadLayered([]spec.Layer{{Root: dir, Sources: src}})
			if err != nil {
				t.Fatal(err)
			}
			if len(bundle.Commands) != 1 {
				t.Fatalf("imported %d commands, want 1", len(bundle.Commands))
			}
			if bundle.Commands[0].Body != body {
				t.Errorf("portable command lost leading body blank lines: got %q, want %q", bundle.Commands[0].Body, body)
			}
		})
	}
}

// TestImportKiro_NativeSkillFolderWinsOverLegacySteeringFile covers a
// project carrying both the pre-#642 flattened
// `.kiro/steering/skill-<name>.md` and the native
// `.kiro/skills/<name>/SKILL.md` for the same skill name: the native
// copy must win, since importSkillFolders runs after
// importKiroSteering (see importFromKiro).
func TestImportKiro_NativeSkillFolderWinsOverLegacySteeringFile(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [kiro]\n")
	writeFile(t, filepath.Join(dir, ".kiro", "steering", "skill-my-skill.md"),
		"---\ninclusion: auto\nname: my-skill\ndescription: stale\n---\n\nSTALE legacy body.\n")
	writeFile(t, filepath.Join(dir, ".kiro", "skills", "my-skill", "SKILL.md"),
		"---\nname: my-skill\ndescription: current\n---\n\nCURRENT native body.\n")

	execCLI(t, "import", "kiro")

	skill := readFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "my-skill", "SKILL.md"))
	if !strings.Contains(skill, "CURRENT native body.") {
		t.Errorf("expected the native skill folder to win, got:\n%s", skill)
	}
	if strings.Contains(skill, "STALE") {
		t.Errorf("expected no trace of the legacy flattened body, got:\n%s", skill)
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestImportKiro_SkipsOnDemandSteeringRules(t *testing.T) {
	for _, mode := range []string{"manual", "auto"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			silence(t)
			var logged strings.Builder
			prev := logOut
			logOut = &logged
			defer func() { logOut = prev }()

			writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [kiro]\n")
			writeFile(t, filepath.Join(dir, ".kiro", "steering", "troubleshooting.md"),
				"---\ninclusion: "+mode+"\ndescription: Debugging steps\n---\n\nTroubleshooting notes.\n")
			writeFile(t, filepath.Join(dir, ".kiro", "steering", "style.md"),
				"---\ninclusion: always\n---\n\nStyle rules.\n")

			execCLI(t, "import", "kiro")

			if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "rules", "troubleshooting.md")); !os.IsNotExist(err) {
				t.Errorf("expected the %s steering file to stay out of the always-on rules, stat err=%v", mode, err)
			}
			if !strings.Contains(readFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md")), "Style rules.") {
				t.Error("expected the always steering file to import as a rule")
			}
			if !strings.Contains(logged.String(), "troubleshooting.md") || !strings.Contains(logged.String(), "inclusion: "+mode) {
				t.Errorf("expected a skip note naming the file and its mode, got:\n%s", logged.String())
			}
		})
	}
}

func TestImportKiro_OnDemandSteeringWarnsWhenARuleWouldOverwriteIt(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	var logged strings.Builder
	prev := logOut
	logOut = &logged
	defer func() { logOut = prev }()

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [kiro]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "troubleshooting.md"), "---\nname: troubleshooting\n---\n\nOld rule.\n")
	writeFile(t, filepath.Join(dir, ".kiro", "steering", "troubleshooting.md"),
		"---\ninclusion: manual\n---\n\nTroubleshooting notes.\n")

	execCLI(t, "import", "kiro")

	if !strings.Contains(logged.String(), "sync will overwrite it") {
		t.Errorf("expected a warning that the same-named rule overwrites the steering file, got:\n%s", logged.String())
	}
	if !strings.Contains(readFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "troubleshooting.md")), "Old rule.") {
		t.Error("expected the existing rule source to stay untouched")
	}
}

const kiroNativeAgentFull = "---\nname: backend-dev\ndescription: Backend agent\ntools: [read, shell]\nexcludedTools: [knowledge]\nincludeMcpJson: true\nincludePowers: false\nresources:\n  - file://./ARCHITECTURE.md\n  - skill://backend-patterns\npermissions:\n  rules:\n    - capability: shell\n      match: [\"npm *\"]\n      effect: allow\nwelcomeMessage: Ready to work.\nhooks:\n  - name: git-status\n    trigger: SessionStart\n    action:\n      type: command\n      command: git status\n---\n\nBuild the backend.\n"

func TestImportKiro_PreservesNativeAgentFieldsAndDropsDeletedOnes(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [kiro]\n")
	path := filepath.Join(dir, kiroAgentsDir, "backend-dev.md")
	resync := func(native string) string {
		writeFile(t, path, native)
		execCLI(t, "import", "kiro")
		execCLI(t, "sync", "-t", "kiro")
		return readFile(t, path)
	}
	got := resync(kiroNativeAgentFull)
	for _, want := range []string{"excludedTools:", "includeMcpJson: true", "includePowers: false", "skill://backend-patterns", "capability: shell", "effect: allow", "welcomeMessage: Ready to work.", "trigger: SessionStart", "command: git status"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q after import and sync:\n%s", want, got)
		}
	}
	execCLI(t, "sync", "--check", "-t", "kiro")

	got = resync(strings.Replace(kiroNativeAgentFull, "excludedTools: [knowledge]\n", "", 1))
	if strings.Contains(got, "excludedTools") || !strings.Contains(got, "includeMcpJson: true") || !strings.Contains(got, "capability: shell") {
		t.Errorf("removing excludedTools alone must keep the rest:\n%s", got)
	}

	got = resync("---\nname: backend-dev\ndescription: Backend agent\ntools: [read, shell]\n---\n\nBuild the backend.\n")
	for _, gone := range []string{"excludedTools", "includeMcpJson", "includePowers", "resources", "permissions", "welcomeMessage", "hooks", "git status"} {
		if strings.Contains(got, gone) {
			t.Errorf("deleted native field %q returned:\n%s", gone, got)
		}
	}
}

func TestImportKiro_KeepsPortableHooksOnExistingSpec(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [kiro]\n")
	specPath := filepath.Join(dir, ".agnostic-ai/agents/backend-dev.md")
	writeFile(t, specPath, "---\nname: backend-dev\ndescription: Backend agent\nhooks:\n  - name: portable-hook\n---\n\nBuild the backend.\n")
	writeFile(t, filepath.Join(dir, kiroAgentsDir, "backend-dev.md"), "---\nname: backend-dev\ndescription: Backend agent\n---\n\nBuild the backend.\n")
	execCLI(t, "import", "kiro", "--overwrite")
	if got := readFile(t, specPath); !strings.Contains(got, "portable-hook") {
		t.Errorf("portable top-level hooks lost:\n%s", got)
	}
}

func kiroAgentsDst(t *testing.T, dir string) string {
	t.Helper()
	dst := filepath.Join(dir, ".agnostic-ai/agents")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestImportKiroAgents_ImportsNestedAgentUnderHyphenatedName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, kiroAgentsDir, "team", "planner.md"), "---\ndescription: Plans.\n---\n\nPlan.\n")
	writeFile(t, filepath.Join(dir, kiroAgentsDir, "a", "b", "c.md"), "---\ndescription: Deep.\n---\n\nDeep body.\n")
	dst := kiroAgentsDst(t, dir)
	log := captureLog(t)
	count, err := importKiroAgents(dir, dst, nil)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if got := readFile(t, filepath.Join(dst, "team-planner.md")); !strings.Contains(got, "Plan.") || !strings.Contains(got, "description: Plans.") {
		t.Errorf("nested agent not imported:\n%s", got)
	}
	if got := readFile(t, filepath.Join(dst, "a-b-c.md")); !strings.Contains(got, "Deep body.") {
		t.Errorf("deeply nested agent not imported:\n%s", got)
	}
	want := "note: imported .kiro/agents/team/planner.md as spec agent team-planner; sync writes .kiro/agents/team-planner.md, so remove the original"
	if !strings.Contains(log.String(), want) {
		t.Errorf("missing %q in output:\n%s", want, log.String())
	}
}

func TestImportKiroAgents_ImportsJSONProfile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, kiroAgentsDir, "reviewer.json"), `{
  "name": "Code Reviewer",
  "description": "Reviews code.",
  "model": "claude-sonnet-4.6",
  "tools": ["read", "shell"],
  "includeMcpJson": true,
  "prompt": "Review changes.\nBe brief."
}
`)
	dst := kiroAgentsDst(t, dir)
	log := captureLog(t)
	count, err := importKiroAgents(dir, dst, nil)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	got := readFile(t, filepath.Join(dst, "reviewer.md"))
	for _, want := range []string{"description: Reviews code.", "model: claude-sonnet-4.6", "x-kiro:", "name: Code Reviewer", "includeMcpJson: true", "Review changes.\nBe brief."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in imported agent:\n%s", want, got)
		}
	}
	if strings.Contains(got, "prompt:") {
		t.Errorf("prompt key stayed in frontmatter:\n%s", got)
	}
	if !strings.Contains(log.String(), "note: imported .kiro/agents/reviewer.json as spec agent reviewer") {
		t.Errorf("missing remove-original note:\n%s", log.String())
	}
}

func TestImportKiroAgents_JSONMatchesMarkdownMapping(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, kiroAgentsDir, "viajson.json"), `{"name":"X","description":"D.","tools":["read"],"prompt":"Body."}`)
	writeFile(t, filepath.Join(dir, kiroAgentsDir, "viamd.md"), "---\nname: X\ndescription: D.\ntools:\n  - read\n---\n\nBody.\n")
	dst := kiroAgentsDst(t, dir)
	if _, err := importKiroAgents(dir, dst, nil); err != nil {
		t.Fatal(err)
	}
	if a, b := readFile(t, filepath.Join(dst, "viajson.md")), readFile(t, filepath.Join(dst, "viamd.md")); a != b {
		t.Errorf("JSON and Markdown imports differ:\n%s\n---\n%s", a, b)
	}
}

func TestImportKiroAgents_InvalidJSONFails(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, kiroAgentsDir, "bad.json"), "[1, 2]\n")
	if _, err := importKiroAgents(dir, kiroAgentsDst(t, dir), nil); err == nil || !strings.Contains(err.Error(), "bad.json") {
		t.Errorf("err = %v, want a failure naming bad.json", err)
	}
}

func TestImportKiroAgents_ReportsNameClashesWithoutOverwriting(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, kiroAgentsDir, "team-planner.md"), "---\ndescription: Top.\n---\n\nTop body.\n")
	writeFile(t, filepath.Join(dir, kiroAgentsDir, "team", "planner.md"), "---\ndescription: Nested.\n---\n\nNested body.\n")
	writeFile(t, filepath.Join(dir, kiroAgentsDir, "a.md"), "---\ndescription: MD.\n---\n\nMD body.\n")
	writeFile(t, filepath.Join(dir, kiroAgentsDir, "a.json"), `{"description":"JSON.","prompt":"JSON body."}`)
	dst := kiroAgentsDst(t, dir)
	log := captureLog(t)
	count, err := importKiroAgents(dir, dst, nil)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if got := readFile(t, filepath.Join(dst, "team-planner.md")); !strings.Contains(got, "Top body.") {
		t.Errorf("nested agent overwrote the top-level one:\n%s", got)
	}
	if got := readFile(t, filepath.Join(dst, "a.md")); !strings.Contains(got, "MD body.") {
		t.Errorf("JSON agent overwrote the Markdown one:\n%s", got)
	}
	for _, want := range []string{
		"note: skipped .kiro/agents/team/planner.md: spec agent team-planner already imported from .kiro/agents/team-planner.md",
		"note: skipped .kiro/agents/a.json: spec agent a already imported from .kiro/agents/a.md",
	} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("missing %q in output:\n%s", want, log.String())
		}
	}
}

func TestImportKiro_NestedAndJSONAgentsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [kiro]\n")
	writeFile(t, filepath.Join(dir, kiroAgentsDir, "team", "planner.md"), "---\ndescription: Plans.\n---\n\nPlan.\n")
	writeFile(t, filepath.Join(dir, kiroAgentsDir, "reviewer.json"), `{"description":"Reviews.","prompt":"Review."}`)

	execCLI(t, "import", "kiro")
	planner := filepath.Join(dir, ".agnostic-ai", "agents", "team-planner.md")
	reviewer := filepath.Join(dir, ".agnostic-ai", "agents", "reviewer.md")
	firstPlanner, firstReviewer := readFile(t, planner), readFile(t, reviewer)

	execCLI(t, "sync", "-t", "kiro")
	for _, name := range []string{"team-planner.md", "reviewer.md"} {
		if got := readFile(t, filepath.Join(dir, kiroAgentsDir, name)); !strings.Contains(got, "description:") {
			t.Errorf("sync wrote no %s:\n%s", name, got)
		}
	}
	execCLI(t, "sync", "-t", "kiro", "--check")

	execCLI(t, "import", "kiro", "--overwrite")
	if got := readFile(t, planner); got != firstPlanner {
		t.Errorf("re-import changed team-planner:\n%s\n---\n%s", firstPlanner, got)
	}
	if got := readFile(t, reviewer); got != firstReviewer {
		t.Errorf("re-import changed reviewer:\n%s\n---\n%s", firstReviewer, got)
	}
	execCLI(t, "sync", "-t", "kiro", "--check")
}
