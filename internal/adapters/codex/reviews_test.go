package codex

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Unscoped reviews miss the root AGENTS.md only when the legacy
// rules-file is that file: with any other path, sync still writes
// AGENTS.md and its review section.
func TestEmit_NotesUnscopedReviewsOnlyWhenRulesFileOwnsAgentsMD(t *testing.T) {
	for _, tc := range []struct {
		rulesFile string
		wantNote  bool
	}{
		{"AGENTS.md", true},
		{"CODEX_RULES.md", false},
	} {
		t.Run(tc.rulesFile, func(t *testing.T) {
			testutil.TempCwd(t)
			emit.ResetCoverageNotes()
			t.Cleanup(emit.ResetCoverageNotes)
			buf := &strings.Builder{}
			prev := emit.Warner
			emit.Warner = buf
			t.Cleanup(func() { emit.Warner = prev })

			cfg := &config.Config{Targets: []string{"codex"}, Outputs: map[string]config.Output{"codex": {RulesFile: tc.rulesFile}}}
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindReview, Name: "review", Body: "Check it."}})
			if err := New().Emit(emit.NewSession(), b, cfg, false); err != nil {
				t.Fatal(err)
			}
			emit.FlushCoverageNotes()
			if got := strings.Contains(buf.String(), "review"); got != tc.wantNote {
				t.Errorf("note printed = %v, want %v:\n%s", got, tc.wantNote, buf.String())
			}
		})
	}
}
