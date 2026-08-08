package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestImportQoder_RoundTripFixedPoint emits a bundle to qoder, wipes the
// source specs, imports the emitted tree back, then re-emits. The
// second emit must byte-match the first: import reconstructs rules
// (from `.qoder/rules/`), agents (from `.qoder/agents/`), and skills
// (from `.qoder/skills/<name>/SKILL.md`, target-audit 2026-08-08, #558).
//
// The agent carries a `tools` list deliberately: qoder is the one
// target that renders `tools` as a comma-separated string
// (`tools: Read, Grep, Bash`) instead of agnostic-ai's generic list
// form. Without splitting it back into a list on import, the
// re-emitted `tools` would still match (qoder joins either shape the
// same way), but the reconstructed *source* spec would carry a single
// string that every other target's generic tools passthrough silently
// mishandles on its own next sync: a regression this fixed point alone
// cannot see, so the source-side assertion below checks it directly.
func TestImportQoder_RoundTripFixedPoint(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [qoder]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r1.md"),
		"---\nname: r1\n---\n\nrule one body\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "reviewer.md"),
		"---\nname: reviewer\ndescription: Reviews diffs.\nmodel: sonnet\ntools: [Read, Grep, Bash]\n---\n\nagent body\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "my-skill", "SKILL.md"),
		"---\nname: my-skill\ndescription: An example skill\n---\n\nSkill body here.\n")

	execCLI(t, "sync", "-t", "qoder")
	first := snapshotEmitted(t, dir)
	if _, ok := first[".qoder/rules/r1.md"]; !ok {
		t.Fatalf("first emit produced no rule file: %v", keys(first))
	}
	agentOut, ok := first[".qoder/agents/reviewer.md"]
	if !ok {
		t.Fatalf("first emit produced no agent file: %v", keys(first))
	}
	if !strings.Contains(agentOut, "tools: Read, Grep, Bash") {
		t.Fatalf("expected qoder's comma-separated tools form, got:\n%s", agentOut)
	}
	if _, ok := first[".qoder/skills/my-skill/SKILL.md"]; !ok {
		t.Fatalf("first emit produced no skill folder: %v", keys(first))
	}

	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai")); err != nil {
		t.Fatalf("wipe source specs: %v", err)
	}
	execCLI(t, "import", "qoder")

	rule := readFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r1.md"))
	if !strings.Contains(rule, "rule one body") {
		t.Errorf("rule not reconstructed:\n%s", rule)
	}
	agent := readFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "reviewer.md"))
	if !strings.Contains(agent, "agent body") {
		t.Errorf("agent body not reconstructed:\n%s", agent)
	}
	if !strings.Contains(agent, "model: sonnet") {
		t.Errorf("agent model not reconstructed:\n%s", agent)
	}
	// The critical assertion: the reconstructed *source* spec must carry
	// a list, not qoder's on-disk comma-separated string, so every other
	// target's generic tools passthrough keeps working.
	if !strings.Contains(agent, "tools: [Read, Grep, Bash]") {
		t.Errorf("expected tools reconstructed as a list, got:\n%s", agent)
	}
	if strings.Contains(agent, "tools: Read, Grep, Bash\n") {
		t.Errorf("tools must not stay a bare comma string in the source spec, got:\n%s", agent)
	}
	skill := readFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "my-skill", "SKILL.md"))
	if !strings.Contains(skill, "description: An example skill") || !strings.Contains(skill, "Skill body here.") {
		t.Errorf("skill not reconstructed:\n%s", skill)
	}

	execCLI(t, "sync", "-t", "qoder")
	second := snapshotEmitted(t, dir)
	assertEmittedEqual(t, first, second)
}

// TestImportQoder_AgentWithoutToolsRoundTrips covers the common case
// (no tools at all): the frontmatter must not gain a spurious `tools:`
// key.
func TestImportQoder_AgentWithoutToolsRoundTrips(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [qoder]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "plain.md"),
		"---\nname: plain\n---\n\nplain agent body\n")

	execCLI(t, "sync", "-t", "qoder")
	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai")); err != nil {
		t.Fatalf("wipe source specs: %v", err)
	}
	execCLI(t, "import", "qoder")

	agent := readFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "plain.md"))
	if strings.Contains(agent, "tools:") {
		t.Errorf("expected no tools: key, got:\n%s", agent)
	}
	if !strings.Contains(agent, "plain agent body") {
		t.Errorf("agent body not reconstructed:\n%s", agent)
	}
}

// TestImportQoder_KnownSourceWiredIn guards the wiring: qoder moved off
// the generic rulesDirImporters map onto its own dedicated importer
// (so it can also reconstruct `.qoder/agents/`) and must stay a known
// source.
func TestImportQoder_KnownSourceWiredIn(t *testing.T) {
	if !isKnownImportSource("qoder") {
		t.Error("qoder should be a known import source")
	}
	sources := importSources()
	if !strings.Contains(sources, "qoder") {
		t.Errorf("importSources() missing %q: %s", "qoder", sources)
	}
}

func TestRewriteQoderAgentTools(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "comma string becomes a list",
			in:   "---\nname: a\ntools: Read, Grep, Bash\n---\n\nbody\n",
			want: "---\nname: a\ntools: [Read, Grep, Bash]\n---\n\nbody\n",
		},
		{
			name: "single tool",
			in:   "---\nname: a\ntools: Read\n---\n\nbody\n",
			want: "---\nname: a\ntools: [Read]\n---\n\nbody\n",
		},
		{
			name: "already a flow list is untouched",
			in:   "---\nname: a\ntools: [Read, Grep]\n---\n\nbody\n",
			want: "---\nname: a\ntools: [Read, Grep]\n---\n\nbody\n",
		},
		{
			name: "already a block sequence is untouched",
			in:   "---\nname: a\ntools:\n  - Read\n  - Grep\n---\n\nbody\n",
			want: "---\nname: a\ntools:\n  - Read\n  - Grep\n---\n\nbody\n",
		},
		{
			name: "no tools key is untouched",
			in:   "---\nname: a\ndescription: d\n---\n\nbody\n",
			want: "---\nname: a\ndescription: d\n---\n\nbody\n",
		},
		{
			name: "no frontmatter at all is untouched",
			in:   "plain body, no frontmatter\n",
			want: "plain body, no frontmatter\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := rewriteQoderAgentTools(c.in); got != c.want {
				t.Errorf("rewriteQoderAgentTools(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
