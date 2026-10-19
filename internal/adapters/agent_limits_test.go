package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// The portable `maxTurns` and `background` land under each target's own
// key, and a value set under `x-<target>` wins over the portable one.
func TestEmitAgents_PortableTurnLimitAndBackground(t *testing.T) {
	cases := []struct {
		target string
		meta   map[string]any
		want   []string
		absent []string
	}{
		{"claude", map[string]any{"maxTurns": 12, "background": true}, []string{"maxTurns: 12", "background: true"}, nil},
		{"junie", map[string]any{"maxTurns": 12}, []string{"maxTurns: 12"}, nil},
		{"qoder", map[string]any{"maxTurns": 12, "background": true}, []string{"maxTurns: 12", "background: true"}, nil},
		{"gemini", map[string]any{"maxTurns": 12, "background": true}, []string{"max_turns: 12"}, []string{"maxTurns", "background"}},
		{"kilo", map[string]any{"maxTurns": 12}, []string{"steps: 12"}, []string{"maxTurns"}},
		{"opencode", map[string]any{"maxTurns": 12}, []string{"steps: 12"}, []string{"maxTurns"}},
		{"cursor", map[string]any{"maxTurns": 12, "background": true}, []string{"is_background: true"}, []string{"maxTurns", "\nbackground:"}},
		{"claude", map[string]any{"maxTurns": 12, "x-claude": map[string]any{"maxTurns": 3}}, []string{"maxTurns: 3"}, []string{"maxTurns: 12"}},
		{"gemini", map[string]any{"maxTurns": 12, "x-gemini": map[string]any{"max_turns": 3}}, []string{"max_turns: 3"}, []string{"max_turns: 12", "maxTurns"}},
		{"gemini", map[string]any{"x-gemini": map[string]any{"maxTurns": 4}}, []string{"max_turns: 4"}, []string{"maxTurns"}},
		{"kilo", map[string]any{"maxTurns": 12, "x-kilo": map[string]any{"steps": 3}}, []string{"steps: 3"}, []string{"steps: 12", "maxTurns"}},
		{"opencode", map[string]any{"maxTurns": 12, "x-opencode": map[string]any{"steps": 3}}, []string{"steps: 3"}, []string{"steps: 12", "maxTurns"}},
		{"cursor", map[string]any{"background": true, "x-cursor": map[string]any{"is_background": false}}, []string{"is_background: false"}, []string{"is_background: true"}},
		{"cursor", map[string]any{"x-cursor": map[string]any{"background": true}}, []string{"is_background: true"}, []string{"\nbackground:"}},
		{"gemini", map[string]any{"max_turns": 5, "x-gemini": map[string]any{"maxTurns": 3}}, []string{"max_turns: 3"}, []string{"max_turns: 5"}},
		{"gemini", map[string]any{"maxTurns": 12, "x-gemini": map[string]any{"maxTurns": "lots"}}, []string{"max_turns: 12"}, []string{"lots"}},
		{"kilo", map[string]any{"maxTurns": 12, "x-kilo": map[string]any{"maxTurns": "lots"}}, []string{"steps: 12"}, []string{"lots"}},
		{"opencode", map[string]any{"maxTurns": 12, "x-opencode": map[string]any{"maxTurns": 0}}, []string{"steps: 12"}, []string{"steps: 0"}},
		{"cursor", map[string]any{"background": true, "x-cursor": map[string]any{"background": "no"}}, []string{"is_background: true"}, []string{": no"}},
		{"cursor", map[string]any{"is_background": false, "x-cursor": map[string]any{"background": true}}, []string{"is_background: true"}, []string{"is_background: false"}},
	}
	for _, c := range cases {
		t.Run(c.target, func(t *testing.T) {
			a, ok := Get(c.target)
			if !ok {
				t.Fatalf("no adapter %s", c.target)
			}
			meta := map[string]any{"description": "Review changes"}
			for k, v := range c.meta {
				meta[k] = v
			}
			dir := t.TempDir()
			agents := []spec.Entry{{Kind: spec.KindAgent, Name: "reviewer", Meta: meta, Body: "Find bugs."}}
			if err := a.(AgentEmitter).EmitAgents(NewSession(), agents, dir, false); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "reviewer.md"))
			if err != nil {
				t.Fatal(err)
			}
			got := string(data)
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in:\n%s", w, got)
				}
			}
			for _, w := range c.absent {
				if strings.Contains(got, w) {
					t.Errorf("unexpected %q in:\n%s", w, got)
				}
			}
		})
	}
}

// An x-<target> override of the wrong type is not written, and sync says
// the shared value applies instead of dropping it silently.
func TestEmitAgents_InvalidTurnLimitOverrideNotes(t *testing.T) {
	var buf strings.Builder
	prev := emit.Warner
	emit.Warner = &buf
	t.Cleanup(func() { emit.Warner = prev })
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	a, _ := Get("gemini")
	agents := []spec.Entry{{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"description": "d", "maxTurns": 12, "x-gemini": map[string]any{"maxTurns": "lots"}}, Body: "b"}}
	if err := a.(AgentEmitter).EmitAgents(NewSession(), agents, t.TempDir(), false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if want := "gemini: x-gemini.maxTurns on agent reviewer is not a whole number of 1 or more, so sync wrote the shared maxTurns"; !strings.Contains(buf.String(), want) {
		t.Errorf("notes:\n%s\nwant:\n%s", buf.String(), want)
	}
}
