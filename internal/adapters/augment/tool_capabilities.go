package augment

import (
	"encoding/json"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

var toolCapabilities = emit.ToolCapabilityTable{
	"read":  {Permission: []string{"read"}},
	"edit":  {Permission: []string{"edit"}},
	"write": {Permission: []string{"write"}},
	"shell": {Permission: []string{"terminal"}},
	"web":   {Permission: []string{"web-fetch", "web-search"}},
}

func (Adapter) ToolCapabilities() emit.ToolCapabilityTable { return toolCapabilities }

func (Adapter) TranslatePermission(list, rule string) ([]string, bool) {
	if list == "ask" {
		return nil, false
	}
	translated, ok := augmentRule(rule, list)
	if !ok {
		return nil, false
	}
	return []string{translated["toolName"].(string)}, true
}

func (Adapter) TranslatePermissionContext(list, rule string, _ spec.Entry, settings []spec.Entry, _ *config.Config) emit.CapabilityTranslation {
	translated, ok := augmentRule(rule, list)
	if list == "ask" || !ok {
		return emit.CapabilityTranslation{}
	}
	result := emit.CapabilityTranslation{Supported: true}
	for _, entry := range settings {
		for _, raw := range nativeRules(entry) {
			native, valid := raw.(map[string]any)
			if !valid || native["toolName"] != translated["toolName"] {
				continue
			}
			if _, valid := native["permission"].(map[string]any); !valid {
				continue
			}
			if native["shellInputRegex"] != nil {
				continue
			}
			body, _ := json.Marshal(native)
			result.Native = []string{string(body)}
			result.Override = "x-augment.toolPermissions"
			return result
		}
	}
	body, _ := json.Marshal(translated)
	result.Native = []string{string(body)}
	return result
}
