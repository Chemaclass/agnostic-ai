package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// The field sets decide whether import treats a missing key as a gap the
// native format cannot hold or as a deletion, so they have to match what
// the adapters actually write. Each case syncs a spec declaring every
// portable key and compares the emitted frontmatter against the table:
// an adapter that grows or loses a field fails here instead of silently
// changing what import preserves.
func TestSpecFields_MatchWhatSyncEmits(t *testing.T) {
	const skillSpec = "---\nname: probe\ndescription: d\nargument-hint: \"[x]\"\nallowed-tools: Read\nlicense: MIT\n---\n\nbody\n"
	const agentSpec = "---\nname: rev\ndescription: d\ntools: [Read]\nmodel: sonnet\neffort: high\nmemory: project\ncolor: blue\n---\n\nbody\n"

	cases := []struct {
		target  string
		kind    string
		spec    string
		emitted string
		fields  specFields
	}{
		{"copilot", "skills", skillSpec, ".github/skills/probe/SKILL.md", defaultSkillFields},
		{"opencode", "skills", skillSpec, ".opencode/skills/probe/SKILL.md", defaultSkillFields},
		{"kiro", "skills", skillSpec, ".kiro/skills/probe/SKILL.md", defaultSkillFields},
		{"zed", "skills", skillSpec, ".agents/skills/probe/SKILL.md", defaultSkillFields},
		{"cursor", "skills", skillSpec, ".cursor/skills/probe/SKILL.md", cursorSkillFields},
		{"cursor", "agents", agentSpec, ".cursor/agents/rev.md", cursorAgentFields},
		{"copilot", "agents", agentSpec, ".github/agents/rev.agent.md", copilotAgentFields},
		{"cline", "agents", agentSpec, ".cline/agents/rev.yml", clineAgentFields},
		{"kiro", "agents", agentSpec, ".kiro/agents/rev.md", kiroAgentFields},
		{"qoder", "agents", agentSpec, ".qoder/agents/rev.md", qoderAgentFields},
		{"trae", "agents", agentSpec, ".trae/agents/rev.md", traeAgentFields},
		{"goose", "agents", agentSpec, ".agents/agents/rev.md", gooseAgentFields},
		{"windsurf", "agents", agentSpec, ".devin/agents/rev.md", windsurfAgentFields},
		{"factory", "agents", agentSpec, ".factory/droids/rev.md", factoryAgentFields},
		{"antigravity", "agents", agentSpec, ".agents/agents/rev/agent.md", antigravityAgentFields},
	}
	for _, c := range cases {
		t.Run(c.target+"/"+c.kind, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+c.target+"]\n")
			if c.kind == "skills" {
				writeFile(t, ".agnostic-ai/skills/probe/SKILL.md", c.spec)
			} else {
				writeFile(t, ".agnostic-ai/agents/rev.md", c.spec)
			}

			execCLI(t, "sync")

			data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(c.emitted)))
			if err != nil {
				t.Fatalf("read emitted file: %v", err)
			}
			for _, key := range emittedFrontmatterKeys(t, data) {
				if !c.fields.expresses(key) {
					t.Errorf("%s writes %q but the field set calls it unexpressible, so import would restore a deleted value", c.emitted, key)
				}
			}
		})
	}
}

// emittedFrontmatterKeys returns the top-level frontmatter keys of an
// emitted markdown file.
func emittedFrontmatterKeys(t *testing.T, data []byte) []string {
	t.Helper()
	yamlBytes, _, ok := splitFrontmatter(data)
	if !ok {
		return nil
	}
	var keys []string
	for _, line := range strings.Split(string(yamlBytes), "\n") {
		if line == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "#") {
			continue
		}
		key, _, found := strings.Cut(line, ":")
		if found {
			keys = append(keys, strings.TrimSpace(key))
		}
	}
	sort.Strings(keys)
	return keys
}

// Deleting a field the target does write is a deliberate removal, so the
// spec loses it instead of resurrecting the old value on the next sync.
func TestImport_DeletingAFieldTheTargetWritesRemovesItFromTheSpec(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [qoder]\n")
	writeFile(t, ".agnostic-ai/agents/rev.md", "---\nname: rev\ndescription: d\nmodel: sonnet\neffort: high\n---\n\nbody\n")

	execCLI(t, "sync")
	writeFile(t, ".qoder/agents/rev.md", "---\nname: rev\ndescription: d\neffort: high\n---\n\nbody\n")
	execCLI(t, "import", "qoder")

	got := readFile(t, ".agnostic-ai/agents/rev.md")
	if strings.Contains(got, "model:") {
		t.Errorf("import restored a model the user deleted in the native agent:\n%s", got)
	}
	if !strings.Contains(got, "effort:") {
		t.Errorf("effort is not a qoder agent field and should have stayed:\n%s", got)
	}
}

// Claude rules keep a plain overwrite: removing `paths` to make a rule
// always-on has to reach the spec, or the next sync re-narrows it (#429).
func TestImportFromClaude_RemovingRulePathsUnscopesTheSpec(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	writeFile(t, ".agnostic-ai/rules/go-style.md", "---\nname: go-style\ndescription: d\npaths: src/**\n---\n\nbody\n")

	execCLI(t, "sync")
	writeFile(t, ".claude/rules/go-style.md", "---\nname: go-style\ndescription: d\n---\n\nbody\n")
	execCLI(t, "import", "claude")

	if got := readFile(t, ".agnostic-ai/rules/go-style.md"); strings.Contains(got, "paths:") {
		t.Errorf("import restored the scope the user removed:\n%s", got)
	}
}

// A chatmode carries description, tools and model, so an agent's
// portable-only keys survive an import from that surface.
func TestImportFromCopilot_ChatmodeKeepsPortableOnlyAgentFields(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [copilot]\n")
	writeFile(t, ".agnostic-ai/agents/rev.md", "---\nname: rev\ndescription: old\neffort: high\nmemory: project\n---\n\nold body\n")
	writeFile(t, ".github/chatmodes/rev.chatmode.md", "---\ndescription: new\ntools: [Read]\n---\n\nnew body\n")

	execCLI(t, "import", "copilot")

	got := readFile(t, ".agnostic-ai/agents/rev.md")
	for _, key := range []string{"effort:", "memory:"} {
		if !strings.Contains(got, key) {
			t.Errorf("%s dropped by the chatmode import:\n%s", key, got)
		}
	}
	if !strings.Contains(got, "description: new") {
		t.Errorf("the chatmode description did not win:\n%s", got)
	}
}
