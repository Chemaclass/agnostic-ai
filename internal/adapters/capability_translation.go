package adapters

import (
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

type CapabilityTranslation = emit.CapabilityTranslation

type capabilityWidener interface{ CapabilityWidening([]string) []string }

type agentCapabilityTranslator interface {
	TranslateAgentCapability(rule string) ([]string, bool)
}

type permissionCapabilityTranslator interface {
	TranslatePermission(list, rule string) ([]string, bool)
}

type toolCapabilityProvider interface {
	ToolCapabilities() emit.ToolCapabilityTable
}

func ToolCapabilityMatrix() map[string]map[string][]string {
	out := map[string]map[string][]string{}
	for target, a := range registry {
		provider, ok := a.(toolCapabilityProvider)
		if !ok {
			continue
		}
		out[target] = map[string][]string{}
		for _, key := range spec.Capabilities {
			row := provider.ToolCapabilities()[key]
			names := row.Agent
			if len(names) == 0 {
				names = row.Permission
			}
			for _, name := range names {
				if name != "" && !slices.Contains(out[target][key], name) {
					out[target][key] = append(out[target][key], name)
				}
			}
		}
	}
	return out
}

func TranslateCapability(target, rule string) CapabilityTranslation {
	return translateCapability(target, "", rule, nil)
}

func TranslatePermissionCapability(target, list, rule string, cfg *config.Config) CapabilityTranslation {
	return translateCapability(target, list, rule, cfg)
}

func translateCapability(target, list, rule string, cfg *config.Config) CapabilityTranslation {
	names, problem := spec.CapabilityTools([]string{rule})
	if problem != "" {
		return CapabilityTranslation{}
	}
	result := CapabilityTranslation{Supported: true}
	for _, name := range names {
		if list != "" && target != "codex" {
			if translator, ok := registry[target].(permissionCapabilityTranslator); ok {
				translated, supported := translator.TranslatePermission(list, name)
				if !supported {
					result.Supported = false
					continue
				}
				for _, native := range translated {
					if !slices.Contains(result.Native, native) {
						result.Native = append(result.Native, native)
					}
				}
				if target == "opencode" && name == "Write" {
					result.Widening = append(result.Widening, "OpenCode's edit permission also covers edit and patch")
				}
				continue
			}
		}
		if list == "" {
			if translator, ok := registry[target].(agentCapabilityTranslator); ok {
				native, supported := translator.TranslateAgentCapability(name)
				result.Native = append(result.Native, native...)
				if !supported {
					result.Supported = false
				}
				continue
			}
		}
		var native string
		var ok bool
		if target == "claude" || target == "qoder" || list == "" && slices.Contains([]string{"copilot", "junie", "trae"}, target) {
			native = name
			ok = name != "Delete"
		} else if target == "kiro" {
			native, ok = emit.MCPAtName(name)
		}
		if !ok {
			provider, provided := registry[target].(toolCapabilityProvider)
			if provided {
				native, ok = emit.CapabilityTool(provider.ToolCapabilities(), name, list != "")
			}
		}
		if !ok {
			result.Supported = false
			continue
		}
		if !slices.Contains(result.Native, native) {
			result.Native = append(result.Native, native)
		}

		if target == "opencode" && name == "Write" {
			result.Widening = append(result.Widening, "OpenCode's edit permission also covers edit and patch")
		}
	}
	if widener, ok := registry[target].(capabilityWidener); ok && list == "" {
		result.Widening = widener.CapabilityWidening(names)
	}
	if target == "codex" && list != "" && cfg != nil && cfg.Outputs[target].ExecPoliciesFromPermissions {
		result.Supported = false
		if translator, ok := registry[target].(permissionCapabilityTranslator); ok {
			for _, name := range names {
				native, supported := translator.TranslatePermission(list, name)
				if supported {
					result.Native = append(result.Native, native...)
					result.Supported = true
				}
			}
		}
		if list == "allow" && !strings.HasSuffix(rule, ":*)") && !strings.HasSuffix(rule, " *)") && result.Supported {
			result.Widening = []string{"Codex exec policies match command prefixes, so extra arguments are also allowed"}
		}
	}

	return result
}

func AgentCapabilitiesFromNative(target string, names []string) ([]string, bool) {
	provider, ok := registry[target].(toolCapabilityProvider)
	if !ok {
		return nil, false
	}
	table := provider.ToolCapabilities()
	var out []string
	for i := 0; i < len(names); {
		if target == "kiro" && strings.HasPrefix(names[i], "@") {
			rest := strings.TrimPrefix(names[i], "@")
			candidate := "mcp:" + rest
			if _, problem := spec.CapabilityTools([]string{candidate}); problem == "" {
				out = append(out, candidate)
				i++
				continue
			}
		}
		var matched string
		consumed := 0
		for _, capability := range spec.Capabilities {
			var native []string
			for _, name := range table[capability].Agent {
				if name != "" && !slices.Contains(native, name) {
					native = append(native, name)
				}
			}
			if len(native) == 0 || len(native) > len(names)-i || !slices.Equal(names[i:i+len(native)], native) {
				continue
			}
			if matched != "" {
				return nil, false
			}
			matched = capability
			consumed = len(native)
		}
		if matched == "" {
			return nil, false
		}
		out = append(out, matched)
		i += consumed
	}
	return out, true
}
