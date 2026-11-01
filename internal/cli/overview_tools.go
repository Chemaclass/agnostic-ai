package cli

import (
	"os"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/overview"
)

// overviewTools builds the coding tool catalog for the overview: every
// tool the mode can write, in the init picker's order, with the state
// the shared or personal config gives it.
func overviewTools(root string, global bool) overview.ToolCatalog {
	h := &editHandler{root: root, global: global}
	team, listed, err := h.teamTargets()
	if err != nil {
		team, listed = h.defaultTargets(), false
	}
	source := "default"
	if listed {
		source = "shared"
	}
	effective := team
	_, local := h.builtinsConfigFiles()
	if data, err := os.ReadFile(local); err == nil {
		if personal, err := configList(data, "targets"); err == nil && personal != nil {
			effective, source = *personal, "personal"
		}
	}
	var detected []string
	if !global {
		detected = detectExistingTargets(root)
	}
	supported := h.supportedTargetNames()
	items := make([]overview.ToolChoice, 0, len(supported))
	for _, choice := range allTargets {
		if !slices.Contains(supported, choice.Name) {
			continue
		}
		on := slices.Contains(effective, choice.Name)
		items = append(items, overview.ToolChoice{
			Target:      choice.Name,
			Description: choice.Desc,
			Enabled:     on,
			Local:       source == "personal" && on != slices.Contains(team, choice.Name),
			Detected:    slices.Contains(detected, choice.Name),
		})
	}
	return overview.ToolCatalog{Source: source, Items: items}
}
