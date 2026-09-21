package trae

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_ValidatesNativeAgentNames(t *testing.T) {
	for _, name := range []string{"1-reviewer", "reviewer-", "review_er", strings.Repeat("a", 51), "a", "Review-1", strings.Repeat("a", 50)} {
		t.Run(name, func(t *testing.T) {
			testutil.TempCwd(t)
			err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{{Kind: spec.KindAgent, Name: name, Body: "Review."}}), &config.Config{}, false)
			valid := name == "a" || name == "Review-1" || len(name) == 50
			if (err == nil) != valid {
				t.Errorf("name %q: error %v, valid %v", name, err, valid)
			}
		})
	}
}
