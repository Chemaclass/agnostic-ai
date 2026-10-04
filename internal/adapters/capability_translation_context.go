package adapters

import (
	"encoding/json"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

type permissionContextTranslator interface {
	TranslatePermissionContext(list, rule string, entry spec.Entry, settings []spec.Entry, cfg *config.Config) emit.CapabilityTranslation
}

func TranslateAgentCapabilityIn(target, rule string, entry spec.Entry) CapabilityTranslation {
	custom, _ := entry.Meta["x-"+target].(map[string]any)
	keys := []string{"tools"}
	if target == "kilo" {
		keys = []string{"permission"}
	} else if target == "windsurf" {
		keys = []string{"allowed-tools"}
	}
	for _, key := range keys {
		if value, overridden := custom[key]; overridden {
			if target == "cursor" {
				return CapabilityTranslation{}
			}
			if target == "codex" {
				tools, ok := value.(map[string]any)
				if !ok || len(tools) == 0 {
					return CapabilityTranslation{}
				}
			}
			body, _ := json.Marshal(value)
			return CapabilityTranslation{Native: []string{key + ": " + string(body)}, Supported: true, Override: "x-" + target + "." + key}
		}
	}
	return TranslateCapability(target, rule)
}

func TranslatePermissionCapabilityIn(target, list, rule string, entry spec.Entry, settings []spec.Entry, cfg *config.Config) CapabilityTranslation {
	normalized := spec.NewBundle(settings).For(target).Settings
	entry = entry.NativePermissions()
	names, problem := spec.CapabilityTools([]string{rule})
	if problem != "" {
		return CapabilityTranslation{}
	}
	if translator, ok := registry[target].(permissionContextTranslator); ok {
		result := CapabilityTranslation{Supported: true}
		for _, name := range names {
			translated := translator.TranslatePermissionContext(list, name, entry, normalized, cfg)
			result.Native = append(result.Native, translated.Native...)
			result.Widening = append(result.Widening, translated.Widening...)
			if !translated.Supported {
				result.Supported = false
			}
			if translated.Override != "" {
				result.Override = translated.Override
			}
		}
		return result
	}
	result := TranslatePermissionCapability(target, list, rule, cfg)
	if target == "opencode" {
		finalNative := map[string]any{}
		for _, candidate := range normalized {
			permission, _ := emit.SettingsCustomObject(candidate, target, "permission")
			for key, value := range permission {
				finalNative[key] = value
			}
		}
		for i, native := range result.Native {
			key, _, _ := strings.Cut(native, "(")
			if value, overridden := finalNative[key]; overridden {
				body, _ := json.Marshal(value)
				result.Native[i] = key + ": " + string(body)
				result.Override = "x-opencode.permission"
				result.Widening = nil
			}
		}
	}

	return result
}
