package emit

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func describedTiers() map[string]config.ModelTier {
	return map[string]config.ModelTier{
		"strong":   {Models: map[string]string{"claude": "opus"}, Description: "for everyday coding."},
		"fast":     {Models: map[string]string{"claude": "haiku"}, Description: "  for lookups  "},
		"balanced": {Models: map[string]string{"claude": "sonnet"}},
		"frontier": {Models: map[string]string{"claude": "fable"}, Description: "   "},
	}
}

func TestModelTiersLine_ListsDescribedTiersSortedByName(t *testing.T) {
	got := ModelTiersLine(describedTiers())

	if want := "Model tiers: `fast` for lookups; `strong` for everyday coding."; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
}

func TestModelTiersLine_EmptyWithoutDescriptions(t *testing.T) {
	tiers := map[string]config.ModelTier{"fast": {Models: map[string]string{"claude": "haiku"}}}

	if got := ModelTiersLine(tiers); got != "" {
		t.Errorf("line = %q, want empty", got)
	}
	if got := RenderModelTiers(tiers); got != "" {
		t.Errorf("block = %q, want empty", got)
	}
	if got := AppendModelTiers("# Body\n", RenderModelTiers(tiers)); got != "# Body\n" {
		t.Errorf("body changed: %q", got)
	}
}

func TestAppendModelTiers_ReplacesTheBlockAlreadyThere(t *testing.T) {
	block := RenderModelTiers(describedTiers())

	once := AppendModelTiers("# Body\n", block)
	twice := AppendModelTiers(once, block)

	if twice != once {
		t.Errorf("second append changed the body:\n%s", twice)
	}
	if want := "# Body\n\n" + ModelTiersStartMarker + "\n\n" + ModelTiersLine(describedTiers()) + "\n\n" + ModelTiersEndMarker + "\n"; once != want {
		t.Errorf("body = %q, want %q", once, want)
	}
}

func TestAppendModelTiers_EmptyBodyGetsTheBlockAlone(t *testing.T) {
	block := RenderModelTiers(describedTiers())

	if got := AppendModelTiers("", block); got != block {
		t.Errorf("body = %q, want the block alone", got)
	}
}

func TestStripGeneratedAppendices_DropsTheModelTiersBlock(t *testing.T) {
	body := "# Body\n\n" + RenderModelTiers(describedTiers()) + "\nTail\n"

	got := StripGeneratedAppendices(body)

	if strings.Contains(got, "Model tiers") || strings.Contains(got, ModelTiersStartMarker) {
		t.Errorf("block survived:\n%s", got)
	}
	if !strings.Contains(got, "# Body") || !strings.Contains(got, "Tail") {
		t.Errorf("body lost:\n%s", got)
	}
}
