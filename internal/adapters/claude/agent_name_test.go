package claude

import (
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmitAgents_RejectsNamesClaudeCodeSkips(t *testing.T) {
	for _, name := range []string{"team:reviewer", "-dash"} {
		t.Run(name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			agents := []spec.Entry{{Kind: spec.KindAgent, Name: name, Body: "body"}}

			err := New().Emit(emit.NewSession(), spec.NewBundle(agents), &config.Config{}, false)
			if err == nil || !strings.Contains(err.Error(), `claude agent "`+name+`": name must `) {
				t.Fatalf("project emit error = %v", err)
			}
			err = New().EmitAgents(emit.NewSession(), agents, dir+"/global/agents", false)
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("global emit error = %v", err)
			}
			if _, statErr := os.Stat(".claude/agents"); !os.IsNotExist(statErr) {
				t.Errorf("agents written despite the error: %v", statErr)
			}
		})
	}
}

func TestEmitAgents_AcceptsOrdinaryNames(t *testing.T) {
	testutil.TempCwd(t)
	agents := []spec.Entry{{Kind: spec.KindAgent, Name: "code_reviewer-2", Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(agents), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
}
