package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const reviewerAgent = "---\nname: reviewer\ndescription: Reviews diffs.\ntools: [Read]\n---\n\nReview the diff.\n"

func agentsAsSkillsProject(t *testing.T, config string) {
	t.Helper()
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", config)
	writeAgnosticFile(t, "# Shared\n")
	writeFile(t, filepath.Join(".agnostic-ai", "agents", "reviewer.md"), reviewerAgent)
}

func TestSync_WritesAnAgentAsASkillOnATargetWithoutSubagents(t *testing.T) {
	for _, target := range []string{"amp", "crush", "warp", "zed"} {
		t.Run(target, func(t *testing.T) {
			agentsAsSkillsProject(t, "version: 1\ntargets: ["+target+"]\noutputs:\n  "+target+":\n    agents: skill\n")

			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			got := readFile(t, filepath.Join(".agents", "skills", "reviewer", "SKILL.md"))
			for _, want := range []string{"name: reviewer", "description: Reviews diffs.", "run this role inline", "Review the diff."} {
				if !strings.Contains(got, want) {
					t.Errorf("want %q in:\n%s", want, got)
				}
			}
			if strings.Contains(got, "tools") {
				t.Errorf("a skill carries no tools field:\n%s", got)
			}
			if out, err := runCLI(t, "sync", "--check"); err != nil {
				t.Errorf("sync --check: %v\n%s", err, out)
			}
			out, err := runCLI(t, "why", filepath.Join(".agents", "skills", "reviewer", "SKILL.md"))
			if err != nil || !strings.Contains(out, filepath.Join(".agnostic-ai", "agents", "reviewer.md")) {
				t.Errorf("why must name the agent spec, got %v\n%s", err, out)
			}
		})
	}
}

func TestSync_WithoutTheKeyStillDropsAgents(t *testing.T) {
	agentsAsSkillsProject(t, "version: 1\ntargets: [amp]\n")

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(".agents", "skills", "reviewer")); !os.IsNotExist(err) {
		t.Errorf("no agent skill without the key, stat err = %v", err)
	}
}

// Codex reads .agents/skills and has the reviewer agent natively, so a
// skill there would give it the role twice.
func TestSync_KeepsAgentsOffASkillsTreeATargetWithSubagentsReads(t *testing.T) {
	agentsAsSkillsProject(t, "version: 1\ntargets: [amp, codex]\noutputs:\n  amp:\n    agents: skill\n")
	var notes bytes.Buffer
	adapters.ResetCoverageNotes()
	adapters.SetWarner(&notes)
	t.Cleanup(func() { adapters.ResetCoverageNotes(); adapters.SetWarner(os.Stderr) })

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(".agents", "skills", "reviewer")); !os.IsNotExist(err) {
		t.Errorf("no agent skill where codex reads it, stat err = %v", err)
	}
	if !strings.Contains(notes.String(), "skill stays off: codex also reads that skills directory and has subagents") {
		t.Errorf("want a note naming codex, got:\n%s", notes.String())
	}
}

func TestValidate_FailsOnAnAgentAndSkillThatShareAFolder(t *testing.T) {
	agentsAsSkillsProject(t, "version: 1\ntargets: [amp]\noutputs:\n  amp:\n    agents: skill\n")
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "reviewer", "SKILL.md"), "---\nname: reviewer\ndescription: Skill.\n---\n\nS.\n")

	out, err := runCLI(t, "validate")
	if err == nil || !strings.Contains(out, `agent and skill "reviewer" both write the amp skill folder "reviewer"`) {
		t.Errorf("want the clash named, got %v\n%s", err, out)
	}
}

func TestLoad_RejectsTheKeyOnATargetWithSubagents(t *testing.T) {
	agentsAsSkillsProject(t, "version: 1\ntargets: [claude]\noutputs:\n  claude:\n    agents: skill\n")

	out, err := runCLI(t, "validate")
	if err == nil || !strings.Contains(err.Error()+out, "outputs.claude.agents: skill applies to amp, crush, warp, and zed") {
		t.Errorf("want the key rejected, got %v\n%s", err, out)
	}
}

// An agent written as a skill is invoked as a skill.
func TestRender_AgentReferenceUsesTheSkillPhrase(t *testing.T) {
	agentsAsSkillsProject(t, "version: 1\ntargets: [amp]\noutputs:\n  amp:\n    agents: skill\n")
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "ship", "SKILL.md"), "---\nname: ship\ndescription: Ships.\n---\n\nFirst run {{$AGENT:reviewer}}.\n")

	out, err := runCLI(t, "render", filepath.Join(".agnostic-ai", "skills", "ship", "SKILL.md"), "-t", "amp")
	if err != nil || !strings.Contains(out, "First run the reviewer skill.") {
		t.Errorf("want the skill phrase, got %v\n%s", err, out)
	}
}

func TestCompare_ReportsAgentsWrittenAsSkills(t *testing.T) {
	agentsAsSkillsProject(t, "version: 1\ntargets: [amp, claude]\noutputs:\n  amp:\n    agents: skill\n")

	out, err := runCLI(t, "compare", "amp", "claude")
	if err != nil {
		t.Fatalf("compare: %v\n%s", err, out)
	}
	for _, want := range []string{"amp: not as subagents (outputs.amp.agents: skill writes each as an on-demand skill)", ".agents/skills/reviewer/SKILL.md"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
}
