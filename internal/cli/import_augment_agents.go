package cli

import (
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/augment"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// augmentNativeAgentKeys are the Auggie agent keys with no portable
// field. They move under `x-augment` so the next sync writes them back.
var augmentNativeAgentKeys = []string{"tools", "disabled_tools", "max_turns", "replace_system_prompt", "skills", "disable_skills", "disable_rules", "isolation", "run_in_background", "enabled"}

// importAugmentAgents reads `.augment/agents/*.md`. The Auggie 1.x
// `thinking` key becomes `effort` when it holds one of the six levels
// sync writes it from; any other value stays under `x-augment`.
func importAugmentAgents(root string, src config.Sources) (int, error) {
	return importFlatMarkdownFilesWith(filepath.Join(root, ".augment", "agents"), importSourcePath(root, src.Agents), augmentAgentFields, func(data []byte) ([]byte, error) {
		natives := augmentNativeAgentKeys
		if mapping, rest, ok := frontmatterNode(data); ok {
			if node := mappingValue(mapping, "thinking"); node != nil && mappingValue(mapping, "effort") == nil {
				if slices.Contains(augment.Adapter{}.SettingsEffortLevels(), node.Value) {
					for i := 0; i+1 < len(mapping.Content); i += 2 {
						if mapping.Content[i].Value == "thinking" {
							mapping.Content[i].Value = "effort"
						}
					}
					out, err := encodeFrontmatter(mapping, rest, "augment")
					if err != nil {
						return nil, err
					}
					data = out
				}
			}
		}
		if mapping, _, ok := frontmatterNode(data); ok && mappingValue(mapping, "thinking") != nil {
			natives = append(slices.Clone(natives), "thinking")
		}
		return importNativeAgentFields(data, "augment", natives...)
	})
}
