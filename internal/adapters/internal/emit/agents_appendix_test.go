package emit

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func agentBundle() spec.Bundle {
	return spec.Bundle{Agents: []spec.Entry{
		{Kind: spec.KindAgent, Name: "a1", Path: "agents/a1.md", Body: "agent one body"},
		{Kind: spec.KindAgent, Name: "a2", Path: "agents/a2.md", Body: "agent two body"},
	}}
}

func TestRenderAgentsAppendix_EmitsEachAgentBody(t *testing.T) {
	t.Parallel()
	got := RenderAgentsAppendix(agentBundle())
	for _, want := range []string{
		AgentsStartMarker, AgentsEndMarker,
		"## Agents", "### a1", "agent one body", "### a2", "agent two body",
		"<!-- source: agents/a1.md -->",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("appendix missing %q:\n%s", want, got)
		}
	}
}

func TestRenderAgentsAppendix_EmptyWhenNoAgents(t *testing.T) {
	t.Parallel()
	if got := RenderAgentsAppendix(spec.Bundle{}); got != "" {
		t.Errorf("want empty appendix for agentless bundle, got %q", got)
	}
}

func TestAppendAgentsAppendix_DoesNotStack(t *testing.T) {
	t.Parallel()
	body := "# Pointer body\n"
	app := RenderAgentsAppendix(agentBundle())
	once := AppendAgentsAppendix(body, app)
	twice := AppendAgentsAppendix(once, app)
	if once != twice {
		t.Errorf("re-appending stacked the block:\nonce:\n%s\ntwice:\n%s", once, twice)
	}
	if n := strings.Count(twice, AgentsStartMarker); n != 1 {
		t.Errorf("want exactly one agents block, got %d", n)
	}
}

func TestStripAgentsAppendix_RoundTrips(t *testing.T) {
	t.Parallel()
	body := "# Pointer body\n"
	withAgents := AppendAgentsAppendix(body, RenderAgentsAppendix(agentBundle()))
	if got := StripAgentsAppendix(withAgents); got != body {
		t.Errorf("strip did not restore body.\nwant %q\ngot  %q", body, got)
	}
}

// TestAppendRulesAndAgentsAppendix_Coexist confirms the two blocks nest
// independently in the same file (junie.go's `.junie/AGENTS.md` carries
// both): appending one must not disturb the other, and stripping either
// must leave the other intact.
func TestAppendRulesAndAgentsAppendix_Coexist(t *testing.T) {
	t.Parallel()
	body := "# Pointer body\n"
	body = AppendRulesAppendix(body, RenderRulesAppendix(ruleBundle()))
	body = AppendAgentsAppendix(body, RenderAgentsAppendix(agentBundle()))

	for _, want := range []string{"### r1", "rule one body", "### a1", "agent one body"} {
		if !strings.Contains(body, want) {
			t.Errorf("combined body missing %q:\n%s", want, body)
		}
	}

	rulesOnly := StripAgentsAppendix(body)
	if strings.Contains(rulesOnly, "### a1") {
		t.Errorf("StripAgentsAppendix left agent content behind:\n%s", rulesOnly)
	}
	if !strings.Contains(rulesOnly, "### r1") {
		t.Errorf("StripAgentsAppendix must not remove the rules block:\n%s", rulesOnly)
	}

	agentsOnly := StripRulesAppendix(body)
	if strings.Contains(agentsOnly, "### r1") {
		t.Errorf("StripRulesAppendix left rule content behind:\n%s", agentsOnly)
	}
	if !strings.Contains(agentsOnly, "### a1") {
		t.Errorf("StripRulesAppendix must not remove the agents block:\n%s", agentsOnly)
	}

	if got := StripGeneratedAppendices(body); got != "# Pointer body\n" {
		t.Errorf("StripGeneratedAppendices did not restore the canonical body.\nwant %q\ngot  %q", "# Pointer body\n", got)
	}
}
