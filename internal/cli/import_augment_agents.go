package cli

import (
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/augment"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// augmentNativeAgentKeys are the Auggie agent keys with no portable
// field. They move under `x-augment` so the next sync writes them back.
var augmentNativeAgentKeys = []string{"tools", "disabled_tools", "max_turns", "replace_system_prompt", "skills", "disable_skills", "disable_rules", "isolation", "run_in_background", "enabled"}

// importAugmentAgents reads `.augment/agents/*.md`. The Auggie 1.x
// `thinking` key becomes `effort` when it holds one of the six levels
// sync writes it from, unless the spec scopes `thinking` under
// `x-augment`; any other `thinking` stays under `x-augment`.
func importAugmentAgents(root string, src config.Sources) (int, error) {
	return importFlatMarkdownFilesAt(filepath.Join(root, ".augment", "agents"), importSourcePath(root, src.Agents), augmentAgentFields, func(data []byte, dst string) ([]byte, error) {
		mapping, rest, ok := frontmatterNode(data)
		if !ok {
			return data, nil
		}
		natives := augmentNativeAgentKeys
		if thinking := mappingValue(mapping, "thinking"); thinking != nil {
			portable := slices.Contains(augment.Adapter{}.SettingsEffortLevels(), thinking.Value) &&
				mappingValue(mapping, "effort") == nil && !specScopesKey(dst, "augment", "thinking")
			if portable {
				renameMappingKey(mapping, "thinking", "effort")
				var err error
				if data, err = encodeFrontmatter(mapping, rest, "augment"); err != nil {
					return nil, err
				}
			} else {
				natives = append(slices.Clone(natives), "thinking")
			}
		}
		return importNativeAgentFields(data, "augment", natives...)
	})
}

// preserveAugmentEffort keeps the spec's `effort` when Augment's file
// cannot show it: a value that is not one of the six levels, or a level
// the spec overrides with `x-augment.thinking`.
func preserveAugmentEffort(existing, imported *yaml.Node) {
	effort := perTargetValue(mappingValue(existing, "effort"), "augment")
	if effort == nil || effort.Kind != yaml.ScalarNode || mappingValue(imported, "effort") != nil {
		return
	}
	overridden := mappingValue(mappingValue(existing, "x-augment"), "thinking") != nil
	if overridden || !slices.Contains(augment.Adapter{}.SettingsEffortLevels(), effort.Value) {
		imported.Content = append(imported.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "effort"}, effort)
	}
}
