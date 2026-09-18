package cli

import (
	"strings"
	"testing"
)

// Devin reads `.agents/agents/` as well as its own `.devin/agents/`,
// and the targets owning that tree write an agent at a different path
// inside it, so the content collision check never sees the overlap. One
// spec becomes two profiles claiming the same name, and only the
// `.devin/` copy carries allowed-tools (#863).
func TestWarnSharedAgentsTreeReaders(t *testing.T) {
	cases := []struct {
		name    string
		owners  map[string][]string
		targets []string
		want    string
	}{
		{
			name: "antigravity nests a profile Devin also reads",
			owners: map[string][]string{
				".agents/agents/reviewer/agent.md": {"antigravity"},
				".devin/agents/reviewer.md":        {"windsurf"},
			},
			targets: []string{"antigravity", "windsurf"},
			want:    ".agents/agents/reviewer/agent.md",
		},
		{
			name: "goose and openhands write a flat one",
			owners: map[string][]string{
				".agents/agents/reviewer.md": {"goose", "openhands"},
				".devin/agents/reviewer.md":  {"windsurf"},
			},
			targets: []string{"goose", "openhands", "windsurf"},
			want:    "goose, openhands",
		},
		{
			name: "no windsurf, no overlap to report",
			owners: map[string][]string{
				".agents/agents/reviewer.md": {"goose"},
			},
			targets: []string{"goose"},
			want:    "",
		},
		{
			name: "windsurf alone owns nothing in that tree",
			owners: map[string][]string{
				".devin/agents/reviewer.md": {"windsurf"},
			},
			targets: []string{"windsurf"},
			want:    "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			buf := captureSummary(t)
			warnSharedAgentsTreeReaders(c.owners, c.targets)
			out := buf.String()
			if c.want == "" {
				if out != "" {
					t.Errorf("expected no note, got %q", out)
				}
				return
			}
			if !strings.Contains(out, c.want) {
				t.Errorf("note missing %q:\n%s", c.want, out)
			}
			if !strings.Contains(out, "allowed-tools") {
				t.Errorf("note does not say what is actually lost:\n%s", out)
			}
		})
	}
}
