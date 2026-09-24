package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestImportWindsurf_RoundTripFixedPoint emits a bundle to windsurf,
// wipes the source specs, imports the emitted tree back, then
// re-emits. The second emit must byte-match the first: import
// reconstructs rules and agents (from `.devin/rules/`, reclassified by
// filename prefix), skills (from `.agents/skills/`, the shared
// cross-tool SKILL.md folder tree Devin Desktop scans), and MCP
// servers (from `.devin/mcp_config.json`'s `mcpServers` map, one
// stdio and one url-only entry so the url-implies-http inference
// round-trips too).
func TestImportWindsurf_RoundTripFixedPoint(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [windsurf]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r1.md"),
		"---\nname: r1\n---\n\nrule one body\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "reviewer.md"),
		"---\nname: reviewer\n---\n\nagent body\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "my-skill", "SKILL.md"),
		"---\nname: my-skill\ndescription: An example skill\n---\n\nSkill body here.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "fs.yaml"),
		"name: fs\ncommand: npx\nargs: [\"-y\", \"@modelcontextprotocol/server-filesystem\"]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "linear.yaml"),
		"name: linear\ntype: http\nurl: https://mcp.linear.app\nheaders:\n  Authorization: Bearer x\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "fmt.yaml"),
		"name: fmt\nevent: PostToolUse\nmatcher: exec\ncommand: gofmt -w\n")

	execCLI(t, "sync", "-t", "windsurf")
	first := snapshotEmitted(t, dir)
	if _, ok := first[".agents/skills/my-skill/SKILL.md"]; !ok {
		t.Fatalf("first emit produced no skill folder: %v", keys(first))
	}
	if _, ok := first[".devin/rules/r1.md"]; !ok {
		t.Fatalf("first emit produced no rule file: %v", keys(first))
	}
	if body, ok := first[".devin/mcp_config.json"]; !ok {
		t.Fatalf("first emit produced no mcp file: %v", keys(first))
	} else if !strings.Contains(body, `"transport": "http"`) {
		t.Fatalf("first emit's mcp file missing transport: http:\n%s", body)
	}
	if body, ok := first[".devin/hooks.v1.json"]; !ok {
		t.Fatalf("first emit produced no hooks file: %v", keys(first))
	} else {
		var doc map[string]any
		if err := json.Unmarshal([]byte(body), &doc); err != nil {
			t.Fatalf("hooks.v1.json is not valid json: %v\n%s", err, body)
		}
		if _, wrapped := doc["hooks"]; wrapped {
			t.Fatalf("hooks.v1.json must not carry a top-level hooks wrapper key:\n%s", body)
		}
		if _, ok := doc["PostToolUse"]; !ok {
			t.Fatalf("expected PostToolUse at the top level:\n%s", body)
		}
	}

	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai")); err != nil {
		t.Fatalf("wipe source specs: %v", err)
	}
	execCLI(t, "import", "windsurf")

	rule := readFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r1.md"))
	if !strings.Contains(rule, "rule one body") {
		t.Errorf("rule not reconstructed:\n%s", rule)
	}
	agent := readFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "reviewer.md"))
	if !strings.Contains(agent, "agent body") {
		t.Errorf("agent not reconstructed:\n%s", agent)
	}
	skill := readFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "my-skill", "SKILL.md"))
	if !strings.Contains(skill, "description: An example skill") || !strings.Contains(skill, "Skill body here.") {
		t.Errorf("skill not reconstructed:\n%s", skill)
	}
	fs := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "fs.yaml"))
	if !strings.Contains(fs, "command: npx") {
		t.Errorf("stdio mcp not reconstructed:\n%s", fs)
	}
	linear := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "linear.yaml"))
	if !strings.Contains(linear, "url: https://mcp.linear.app") || !strings.Contains(linear, "type: http") {
		t.Errorf("http mcp not reconstructed with type (renamed from Devin's own transport key):\n%s", linear)
	}
	if strings.Contains(linear, "transport:") {
		t.Errorf("Devin's transport key must not survive into the internal spec's own type meta:\n%s", linear)
	}
	hookPath := findOneHookFile(t, filepath.Join(dir, ".agnostic-ai", "hooks"), "posttooluse")
	hook := readFile(t, hookPath)
	for _, want := range []string{"event: PostToolUse", "matcher: exec", "command: gofmt -w"} {
		if !strings.Contains(hook, want) {
			t.Errorf("hook not reconstructed, missing %q:\n%s", want, hook)
		}
	}

	execCLI(t, "sync", "-t", "windsurf")
	second := snapshotEmitted(t, dir)
	assertEmittedEqual(t, first, second)
}

func TestImportWindsurf_ImportsEveryProjectSkillPathWithPrecedence(t *testing.T) {
	dir := t.TempDir()
	paths := []struct {
		dir  string
		name string
	}{
		{filepath.Join(".agents", "skills"), "agents"},
		{filepath.Join(".devin", "skills"), "devin"},
		{filepath.Join(".windsurf", "skills"), "windsurf"},
	}
	for _, path := range paths {
		writeFile(t, filepath.Join(dir, path.dir, path.name, "SKILL.md"),
			"---\nname: "+path.name+"\n---\n\n"+path.name+" body\n")
		writeFile(t, filepath.Join(dir, path.dir, "shared", "SKILL.md"),
			"---\nname: shared\n---\n\nfrom "+path.name+"\n")
	}

	if err := importFromWindsurf(dir, rootSources(), nil); err != nil {
		t.Fatal(err)
	}

	for _, path := range paths {
		got := readFile(t, filepath.Join(dir, "skills", path.name, "SKILL.md"))
		if !strings.Contains(got, path.name+" body") {
			t.Errorf("%s skill not imported:\n%s", path.name, got)
		}
	}
	shared := readFile(t, filepath.Join(dir, "skills", "shared", "SKILL.md"))
	if !strings.Contains(shared, "from agents") {
		t.Errorf(".agents/skills should win a same-name collision:\n%s", shared)
	}
}

func TestImportWindsurf_PreservesSkillTriggersUnderTargetMeta(t *testing.T) {
	values := map[string][]any{
		"user-only":  {"user"},
		"model-only": {"model"},
		"both":       {"user", "model"},
	}
	for name, triggers := range values {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [windsurf]\noutputs:\n  windsurf:\n    skills-dir: .devin/skills\n")
			native := "---\nname: " + name + "\ndescription: " + name + "\ntriggers:\n"
			for _, trigger := range triggers {
				native += "  - " + trigger.(string) + "\n"
			}
			native += "---\n\nSkill body.\n"
			writeFile(t, filepath.Join(".devin", "skills", name, "SKILL.md"), native)

			execCLI(t, "import", "windsurf")
			source := readFile(t, filepath.Join(".agnostic-ai", "skills", name, "SKILL.md"))
			meta, _ := splitMdcFrontmatter([]byte(source))
			if _, exists := meta["triggers"]; exists {
				t.Fatalf("native triggers must not stay portable at top level:\n%s", source)
			}
			targetMeta, ok := meta["x-windsurf"].(map[string]any)
			if !ok || !reflect.DeepEqual(targetMeta["triggers"], triggers) {
				t.Fatalf("x-windsurf.triggers = %#v, want %#v", targetMeta["triggers"], triggers)
			}

			execCLI(t, "sync", "-t", "windsurf")
			emitted := readFile(t, filepath.Join(".devin", "skills", name, "SKILL.md"))
			emittedMeta, _ := splitMdcFrontmatter([]byte(emitted))
			if !reflect.DeepEqual(emittedMeta["triggers"], triggers) {
				t.Errorf("emitted triggers = %#v, want %#v\n%s", emittedMeta["triggers"], triggers, emitted)
			}
			if _, exists := emittedMeta["x-windsurf"]; exists {
				t.Errorf("target namespace leaked into native skill:\n%s", emitted)
			}
		})
	}
}

// TestImportWindsurf_MCPTransportKeyRenamesToType covers a
// hand-authored `.devin/mcp_config.json` that spells the SSE legacy
// transport explicitly: Devin's own field is `transport`, so the
// importer must rename it to the `type` key every other importer and
// adapter reads, not carry the literal `transport` key into the spec.
func TestImportWindsurf_MCPTransportKeyRenamesToType(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [windsurf]\n")
	writeFile(t, filepath.Join(dir, ".devin", "mcp_config.json"), `{
  "mcpServers": {
    "legacy": {
      "url": "https://mcp.example.test/sse",
      "transport": "sse"
    }
  }
}
`)

	execCLI(t, "import", "windsurf")

	got := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "legacy.yaml"))
	if !strings.Contains(got, "type: sse") {
		t.Errorf("expected transport: sse to import as type: sse, got:\n%s", got)
	}
	if strings.Contains(got, "transport:") {
		t.Errorf("Devin's own transport key must not survive into the spec, got:\n%s", got)
	}
}

// TestImportWindsurf_LegacyFlatSkillStillImports covers a project
// synced before skills moved to the shared `.agents/skills/` tree: a
// flat `.devin/rules/skill-<name>.md` still round-trips as a skill.
func TestImportWindsurf_LegacyFlatSkillStillImports(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [windsurf]\n")
	writeFile(t, filepath.Join(dir, ".devin", "rules", "skill-legacy.md"),
		"# Skill: legacy\n\nlegacy skill body\n")

	execCLI(t, "import", "windsurf")

	got := readFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "legacy.md"))
	if !strings.Contains(got, "legacy skill body") {
		t.Errorf("legacy flat skill not imported:\n%s", got)
	}
}

// TestImportWindsurf_KnownSourceWiredIn guards the wiring: windsurf
// moved off the inline windsurfImportDir switch in runImport onto its
// own dedicated importer and must stay a known source.
// TestImportWindsurf_ReadsScopedRulesDirs covers the import half of
// #628: a rules dir sitting in a project sub-directory imports back to
// a scoped spec, not to a flat one.
func TestImportWindsurf_ReadsScopedRulesDirs(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [windsurf]\n")
	writeFile(t, filepath.Join(dir, ".devin", "rules", "root.md"), "# root\n\nroot body\n")
	writeFile(t, filepath.Join(dir, "backend", ".devin", "rules", "auth.md"), "# auth\n\nauth body\n")
	writeFile(t, filepath.Join(dir, "backend", "api", ".devin", "rules", "limits.md"), "# limits\n\nlimits body\n")
	writeFile(t, filepath.Join(dir, "backend", ".devin", "rules", "agent-delta.md"), "# Agent: delta\n\ndelta body\n")

	execCLI(t, "import", "windsurf")

	for _, p := range []string{
		".agnostic-ai/rules/root.md",
		".agnostic-ai/rules/backend/auth.md",
		".agnostic-ai/rules/backend/api/limits.md",
		".agnostic-ai/agents/backend/delta.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("missing imported spec %s: %v", p, err)
		}
	}
}

// TestImportWindsurf_TriggerBecomesAlwaysApply covers the other import
// half of #628: Devin's activation key translates back into the generic
// field, so the next sync re-emits the same trigger instead of
// defaulting the rule to always-on.
func TestImportWindsurf_TriggerBecomesAlwaysApply(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [windsurf]\n")
	writeFile(t, filepath.Join(dir, ".devin", "rules", "globbed.md"),
		"---\ndescription: Test conventions\nglobs: '**/*.test.ts'\ntrigger: glob\n---\n\n# globbed\n\nglobbed body\n")
	writeFile(t, filepath.Join(dir, ".devin", "rules", "always.md"),
		"---\ntrigger: always_on\n---\n\n# always\n\nalways body\n")

	execCLI(t, "import", "windsurf")

	globbed := readFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "globbed.md"))
	for _, want := range []string{"alwaysApply: false", "globs: '**/*.test.ts'", "description: Test conventions"} {
		if !strings.Contains(globbed, want) {
			t.Errorf("imported globbed.md missing %q:\n%s", want, globbed)
		}
	}
	if strings.Contains(globbed, "trigger:") {
		t.Errorf("imported globbed.md kept the target-native trigger key:\n%s", globbed)
	}
	always := readFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "always.md"))
	if !strings.Contains(always, "alwaysApply: true") {
		t.Errorf("imported always.md missing alwaysApply: true:\n%s", always)
	}
}

// TestImportWindsurf_HooksImportWithNoWrapperKey confirms the importer
// decodes `.devin/hooks.v1.json` correctly even though, unlike Claude
// Code's `.claude/settings.json`, the file carries no top-level
// `"hooks"` key: the event names sit directly at the document root
// (#629).
func TestImportWindsurf_HooksImportWithNoWrapperKey(t *testing.T) {
	dir := t.TempDir()
	doc := `{
  "PostToolUse": [
    {"matcher": "edit", "hooks": [{"type": "command", "command": "gofmt -w", "timeout": 10}]}
  ]
}`
	writeFile(t, filepath.Join(dir, ".devin", "hooks.v1.json"), doc)
	if err := importFromWindsurf(dir, rootSources(), nil); err != nil {
		t.Fatal(err)
	}
	path := findOneHookFile(t, filepath.Join(dir, "hooks"), "posttooluse")
	data := readFile(t, path)
	for _, want := range []string{"event: PostToolUse", "matcher: edit", "command: gofmt -w", "timeout: 10"} {
		if !strings.Contains(data, want) {
			t.Errorf("expected %q in %s", want, data)
		}
	}
}

// TestImportWindsurf_PromptTypeHookImportsPromptField confirms a
// `type: prompt` entry imports with `prompt:` in place of `command:`,
// since agnostic-ai's generic hook spec has no dedicated prompt field.
func TestImportWindsurf_PromptTypeHookImportsPromptField(t *testing.T) {
	dir := t.TempDir()
	doc := `{
  "UserPromptSubmit": [
    {"matcher": "", "hooks": [{"type": "prompt", "prompt": "Does this look destructive?"}]}
  ]
}`
	writeFile(t, filepath.Join(dir, ".devin", "hooks.v1.json"), doc)
	if err := importFromWindsurf(dir, rootSources(), nil); err != nil {
		t.Fatal(err)
	}
	path := findOneHookFile(t, filepath.Join(dir, "hooks"), "userpromptsubmit")
	data := readFile(t, path)
	for _, want := range []string{"event: UserPromptSubmit", "type: prompt", "prompt: Does this look destructive?"} {
		if !strings.Contains(data, want) {
			t.Errorf("expected %q in %s", want, data)
		}
	}
	if strings.Contains(data, "command:") {
		t.Errorf("a prompt hook must not import a command key:\n%s", data)
	}
}

// TestImportWindsurf_NoHooksFileIsNoOp confirms a missing
// `.devin/hooks.v1.json` produces no hooks and no error.
func TestImportWindsurf_NoHooksFileIsNoOp(t *testing.T) {
	dir := t.TempDir()
	n, err := importWindsurfHooks(dir, filepath.Join(dir, "hooks"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("expected 0 hooks imported, got %d", n)
	}
}

func TestImportWindsurf_KnownSourceWiredIn(t *testing.T) {
	if !isKnownImportSource("windsurf") {
		t.Error("windsurf should be a known import source")
	}
	sources := importSources()
	if !strings.Contains(sources, "windsurf") {
		t.Errorf("importSources() missing %q: %s", "windsurf", sources)
	}
}

// TestImportFromWindsurf_ReadsHiddenAndVendorScopedRulesDirs pins #1123:
// `CheckScopePath` accepts a scope like `.github`, `vendor`, or
// `node_modules`, so emission can write a scoped rule under any of
// them, and import must round-trip it instead of pruning the directory
// by a hidden-dir prefix or a hardcoded name list before ever looking
// inside it, the way an earlier draft of windsurfScopedRulesDirs did
// (matching antigravityScopedRulesDirs's own earlier draft, #1114).
func TestImportFromWindsurf_ReadsHiddenAndVendorScopedRulesDirs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".github", ".devin", "rules", "release.md"), "# release\n\nrelease body\n")
	writeFile(t, filepath.Join(dir, "vendor", ".devin", "rules", "pkg.md"), "# pkg\n\npkg body\n")
	writeFile(t, filepath.Join(dir, "node_modules", ".devin", "rules", "pkg2.md"), "# pkg2\n\npkg2 body\n")

	if err := importFromWindsurf(dir, rootSources(), nil); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{
		filepath.Join("rules", ".github", "release.md"),
		filepath.Join("rules", "vendor", "pkg.md"),
		filepath.Join("rules", "node_modules", "pkg2.md"),
	} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("missing imported spec %s: %v", p, err)
		}
	}
}

// TestImportFromWindsurf_NestedScopeMatchingSourceRootNameStillImports
// pins #1123's second finding: pruning by directory basename at every
// depth, not just the exact root-relative source path, treated
// `packages/api/config` as the configured `config/rules` source root
// and pruned it, so `packages/api/config/.devin/rules/auth.md` never
// imported.
func TestImportFromWindsurf_NestedScopeMatchingSourceRootNameStillImports(t *testing.T) {
	dir := t.TempDir()
	src := rootSources()
	src.Rules = filepath.Join("config", "rules")
	writeFile(t, filepath.Join(dir, "packages", "api", "config", ".devin", "rules", "auth.md"), "# auth\n\nauth body\n")

	if err := importFromWindsurf(dir, src, nil); err != nil {
		t.Fatal(err)
	}

	got := filepath.Join(dir, "config", "rules", "packages", "api", "config", "auth.md")
	if _, err := os.Stat(got); err != nil {
		t.Errorf("missing imported spec %s: %v", got, err)
	}
}

// TestImportFromWindsurf_UsesConfiguredRulesDir pins #1123's second
// finding for the root (unscoped) rules dir: `importFromWindsurf` never
// received `cfg`, so it read only `.devin/rules/` and the legacy
// `.windsurf/rules/`, and a project that set
// `outputs.windsurf.rules-dir` imported nothing from the path sync
// wrote there.
func TestImportFromWindsurf_UsesConfiguredRulesDir(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Outputs: map[string]config.Output{"windsurf": {RulesDir: filepath.Join("custom", "rules")}},
	}
	writeFile(t, filepath.Join(dir, "custom", "rules", "house.md"), "# house\n\nhouse body\n")

	if err := importFromWindsurf(dir, rootSources(), cfg); err != nil {
		t.Fatal(err)
	}

	got := filepath.Join(dir, "rules", "house.md")
	if _, err := os.Stat(got); err != nil {
		t.Errorf("missing imported spec %s: %v", got, err)
	}
}

// skipUnlessCanDenyDirReads skips a test that relies on chmod 0o000
// actually blocking a directory read: Windows ignores the Unix mode
// bits, and root ignores them too, so the test's own precondition
// would be silently false rather than exercising the unreadable-dir
// path.
func skipUnlessCanDenyDirReads(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("chmod 0o000 does not deny directory reads on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores mode 0o000")
	}
}

// TestImportFromWindsurf_SurvivesUnreadableDirUnderNodeModules pins the
// #1124 review regression: windsurfScopedRulesDirs used to prune
// node_modules outright, so it never read into it. #1123 stopped
// pruning by name (CheckScopePath accepts node_modules as a scope
// name too), so the shared scopedRulesDirs walker now descends into
// it, and an unrelated unreadable directory inside it (permission
// bits, a broken cache directory, ...) used to abort the whole scoped
// scan with the root rules already imported, leaving agents, skills,
// and every scoped rule unimported. The walk must warn and skip past
// it instead.
func TestImportFromWindsurf_SurvivesUnreadableDirUnderNodeModules(t *testing.T) {
	skipUnlessCanDenyDirReads(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".devin", "rules", "root.md"), "# root\n\nroot body\n")
	writeFile(t, filepath.Join(dir, "backend", ".devin", "rules", "auth.md"), "# auth\n\nauth body\n")
	unreadable := filepath.Join(dir, "node_modules", "cache")
	if err := os.MkdirAll(unreadable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o755) })

	var buf bytes.Buffer
	prev := logOut
	logOut = &buf
	defer func() { logOut = prev }()

	if err := importFromWindsurf(dir, rootSources(), nil); err != nil {
		t.Fatalf("import must survive an unreadable directory, got: %v", err)
	}
	if !strings.Contains(buf.String(), "node_modules") {
		t.Errorf("expected a warning naming the unreadable path, got: %s", buf.String())
	}
	for _, p := range []string{
		filepath.Join("rules", "root.md"),
		filepath.Join("rules", "backend", "auth.md"),
	} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("missing imported spec %s: %v", p, err)
		}
	}
}

// TestImportFromWindsurf_SurvivesUnreadableHiddenDir is the same
// #1124 review regression for a hidden directory: an earlier draft of
// windsurfScopedRulesDirs pruned every hidden directory outright, so
// it never read into one either.
func TestImportFromWindsurf_SurvivesUnreadableHiddenDir(t *testing.T) {
	skipUnlessCanDenyDirReads(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".devin", "rules", "root.md"), "# root\n\nroot body\n")
	writeFile(t, filepath.Join(dir, "backend", ".devin", "rules", "auth.md"), "# auth\n\nauth body\n")
	unreadable := filepath.Join(dir, ".cache")
	if err := os.MkdirAll(unreadable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o755) })

	var buf bytes.Buffer
	prev := logOut
	logOut = &buf
	defer func() { logOut = prev }()

	if err := importFromWindsurf(dir, rootSources(), nil); err != nil {
		t.Fatalf("import must survive an unreadable directory, got: %v", err)
	}
	if !strings.Contains(buf.String(), ".cache") {
		t.Errorf("expected a warning naming the unreadable path, got: %s", buf.String())
	}
	for _, p := range []string{
		filepath.Join("rules", "root.md"),
		filepath.Join("rules", "backend", "auth.md"),
	} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("missing imported spec %s: %v", p, err)
		}
	}
}

// TestImportFromWindsurf_ScopedRulesFollowRulesDirOverride pins #1123's
// second finding for a scoped rules dir: emission honors
// `outputs.windsurf.rules-dir` for a scoped rule too
// (`<scope>/<rules-dir>/<name>.md`), so import must resolve the same
// configured directory instead of only ever looking for `.devin/rules`
// / `.windsurf/rules`.
func TestImportFromWindsurf_ScopedRulesFollowRulesDirOverride(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Outputs: map[string]config.Output{"windsurf": {RulesDir: filepath.Join("custom", "rules")}},
	}
	writeFile(t, filepath.Join(dir, "backend", "custom", "rules", "auth.md"), "# auth\n\nauth body\n")

	if err := importFromWindsurf(dir, rootSources(), cfg); err != nil {
		t.Fatal(err)
	}

	got := filepath.Join(dir, "rules", "backend", "auth.md")
	if _, err := os.Stat(got); err != nil {
		t.Errorf("missing imported spec %s: %v", got, err)
	}
}
