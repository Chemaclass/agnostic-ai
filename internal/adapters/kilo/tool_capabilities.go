package kilo

import (
	"encoding/json"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

var toolCapabilities = emit.ToolCapabilityTable{
	"read":      {Permission: []string{"read"}, Agent: []string{"read"}},
	"Glob":      {Permission: []string{"glob"}, Agent: []string{"glob"}},
	"Grep":      {Permission: []string{"grep"}, Agent: []string{"grep"}},
	"edit":      {Permission: []string{"edit"}, Agent: []string{"edit"}},
	"write":     {Permission: []string{"write"}, Agent: []string{"write"}},
	"shell":     {Permission: []string{"bash"}, Agent: []string{"bash"}},
	"web":       {Permission: []string{"webfetch", "websearch"}, Agent: []string{"webfetch", "websearch"}},
	"Task":      {Permission: []string{"task"}, Agent: []string{"task"}},
	"Skill":     {Permission: []string{"skill"}, Agent: []string{"skill"}},
	"TodoRead":  {Permission: []string{"todoread"}, Agent: []string{"todoread"}},
	"TodoWrite": {Permission: []string{"todowrite"}, Agent: []string{"todowrite"}},
}

func (Adapter) ToolCapabilities() emit.ToolCapabilityTable { return toolCapabilities }

func (Adapter) TranslatePermission(list, rule string) ([]string, bool) {
	tool, pattern, ok := permissionRule(rule)
	if !ok {
		return nil, false
	}
	var out []string
	for _, key := range restrictedKeys(tool, list) {
		if pattern != "*" {
			key += "(" + pattern + ")"
		}
		out = append(out, key)
	}

	return out, true
}

func (Adapter) TranslateAgentCapability(rule string) ([]string, bool) {
	tool, pattern, ok := permissionRule(rule)
	if !ok {
		return nil, false
	}
	if pattern != "*" {
		tool += "(" + pattern + ")"
	}
	return []string{tool}, true
}

func (Adapter) TranslatePermissionContext(list, rule string, entry spec.Entry, settings []spec.Entry, _ *config.Config) emit.CapabilityTranslation {
	names, ok := (Adapter{}).TranslatePermission(list, rule)
	result := emit.CapabilityTranslation{Native: names, Supported: ok}
	if custom, overridden := emit.SettingsCustomObject(entry, target, permissionKey); overridden {
		result.Native = nil
		result.Override = "x-kilo.permission"
		for _, native := range names {
			key, _, _ := strings.Cut(native, "(")
			if value, set := custom[key]; set {
				body, _ := json.Marshal(value)
				result.Native = append(result.Native, key+": "+string(body))
			}
		}
		return result
	}
	for _, candidate := range settings {
		custom, _ := emit.SettingsCustomObject(candidate, target, permissionKey)
		for i, native := range result.Native {
			key, _, _ := strings.Cut(native, "(")
			if value, overridden := custom[key]; overridden {
				body, _ := json.Marshal(value)
				result.Native[i] = key + ": " + string(body)
				result.Override = "x-kilo.permission"
			}
		}
	}
	return result
}
