package emit

import (
	"strings"
	"testing"
)

// agentsBlock is the agents appendix an adapter version between #552 and
// #604 wrote into an entry-point file, the input StripAgentsAppendix
// still has to recognize.
const agentsBlock = AgentsStartMarker + "\n\n## Agents\n\n" +
	"### a1\n\n<!-- source: agents/a1.md -->\n\nagent one body\n\n" +
	"### a2\n\n<!-- source: agents/a2.md -->\n\nagent two body\n\n" +
	AgentsEndMarker + "\n"

func TestStripAgentsAppendix_RoundTrips(t *testing.T) {
	t.Parallel()
	body := "# Pointer body\n"
	if got := StripAgentsAppendix(body + "\n" + agentsBlock); got != body {
		t.Errorf("strip did not restore body.\nwant %q\ngot  %q", body, got)
	}
}

func TestStripAgentsAppendix_LeavesBodyWithoutBlock(t *testing.T) {
	t.Parallel()
	body := "# Pointer body\n"
	if got := StripAgentsAppendix(body); got != body {
		t.Errorf("strip changed a body with no agents block.\nwant %q\ngot  %q", body, got)
	}
}

// TestStripRulesAndAgentsAppendix_Coexist confirms the two blocks nest
// independently in the same file (junie's `.junie/AGENTS.md` carried
// both): stripping either must leave the other intact.
func TestStripRulesAndAgentsAppendix_Coexist(t *testing.T) {
	t.Parallel()
	body := AppendRulesAppendix("# Pointer body\n", RenderRulesAppendix(ruleBundle()))
	body = strings.TrimRight(body, "\n") + "\n\n" + agentsBlock

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
