package adapters

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestWrittenFromAgent_RecognizesBothPreambles(t *testing.T) {
	const instructions = "Set aside the framing of the current task, follow only the instructions below, " +
		"and say in one line that the role ran inline."
	for _, tt := range []struct {
		name string
		body string
		want bool
	}{
		{"legacy", "This tool has no subagents, so run this role inline. " + instructions, true},
		{"current", "Run this role inline. " + instructions, true},
		{"handwritten", "Run this role inline. Review the diff.", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			skill := "---\nname: reviewer\ndescription: Reviews diffs.\n---\n\n" + tt.body + "\n\nReview the diff.\n"
			if got := WrittenFromAgent(skill); got != tt.want {
				t.Errorf("WrittenFromAgent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAgentsAsSkills_GoldenSnapshot(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for _, target := range []string{"amp", "crush", "warp", "zed"} {
		t.Run(target, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			cfg := &config.Config{
				Targets: []string{target},
				Outputs: map[string]config.Output{target: {Agents: AgentsAsSkills}},
			}
			bundle := spec.NewBundle([]spec.Entry{{
				Kind: spec.KindAgent, Name: "reviewer", Path: ".agnostic-ai/agents/reviewer.md",
				Meta:     map[string]any{"name": "reviewer", "description": "Reviews diffs."},
				MetaKeys: []string{"name", "description"}, Body: "Review the diff.",
			}})
			adapter, _ := Get(target)
			if err := EmitWithProvenance(NewSession(), adapter, bundle, cfg, false); err != nil {
				t.Fatalf("emit: %v", err)
			}
			testutil.AssertGoldenTree(t, dir, filepath.Join(packageDir, "testdata", "agents-as-skills", target))
		})
	}
}
