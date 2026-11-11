package emit

import (
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// The markers let import drop the block: the config is its only source.
const (
	ModelTiersStartMarker = "<!-- agnostic-ai:models:start -->"
	ModelTiersEndMarker   = "<!-- agnostic-ai:models:end -->"
)

// ModelTiersLine lists the tiers that carry a description, sorted by
// name. "" when none does.
func ModelTiersLine(tiers map[string]config.ModelTier) string {
	names := make([]string, 0, len(tiers))
	for name, tier := range tiers {
		if strings.TrimSpace(tier.Description) != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	slices.Sort(names)
	items := make([]string, 0, len(names))
	for _, name := range names {
		description := strings.TrimSuffix(strings.TrimSpace(tiers[name].Description), ".")
		items = append(items, "`"+name+"` "+description)
	}
	return "Model tiers: " + strings.Join(items, "; ") + "."
}

// RenderModelTiers renders the marked block. "" when no tier has a
// description.
func RenderModelTiers(tiers map[string]config.ModelTier) string {
	line := ModelTiersLine(tiers)
	if line == "" {
		return ""
	}
	return ModelTiersStartMarker + "\n\n" + line + "\n\n" + ModelTiersEndMarker + "\n"
}

// AppendModelTiers returns body with block after one blank line,
// replacing a block already there.
func AppendModelTiers(body, block string) string {
	if block == "" {
		return body
	}
	body = strings.TrimRight(StripModelTiers(body), "\n")
	if body == "" {
		return block
	}
	return body + "\n\n" + block
}

// StripModelTiers removes the marked model tiers block from body.
func StripModelTiers(body string) string {
	return stripMarkedBlock(body, ModelTiersStartMarker, ModelTiersEndMarker)
}
