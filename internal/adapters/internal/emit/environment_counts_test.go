package emit

import (
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestEnvironmentsWithSetup_CountsEitherField(t *testing.T) {
	envs := []spec.Entry{
		{Meta: map[string]any{"setup": "make setup"}},
		{Meta: map[string]any{"setup-windows": []any{"npm ci"}}},
		{Meta: map[string]any{"setup": ""}},
		{Meta: map[string]any{"install": "npm ci"}},
		{Meta: map[string]any{"x-cursor": map[string]any{"setup": "cursor only"}}},
	}
	if got := EnvironmentsWithSetup("amp", envs); got != 2 {
		t.Errorf("EnvironmentsWithSetup = %d, want 2", got)
	}
}
