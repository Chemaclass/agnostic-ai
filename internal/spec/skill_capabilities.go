package spec

import (
	"fmt"
	"maps"
	"strings"
)

func SkillCapabilityProblem(meta map[string]any) string {
	raw, set := meta["allowed-tools"]
	if !set {
		return ""
	}
	_, problem := skillCapabilityTools(raw)
	return problem
}

func skillCapabilityTools(raw any) ([]string, string) {
	var list []any
	switch value := raw.(type) {
	case []any:
		list = value
	case string:
		for _, item := range SplitToolList(value) {
			list = append(list, item)
		}
	default:
		return nil, "allowed-tools: must be a list of capability names"
	}
	var out []string
	for _, item := range list {
		name, ok := item.(string)
		if !ok {
			return nil, fmt.Sprintf("allowed-tools: entry %v is not a capability name", item)
		}
		names, problem := capabilityToolsOf(name, capabilityForm{field: "allowed-tools:", paths: true})
		if problem != "" {
			return nil, problem
		}
		out = append(out, names...)
	}
	return out, ""
}

func (e Entry) NativeAllowedTools() (Entry, string) {
	if e.Kind != KindSkill {
		return e, ""
	}
	raw, set := e.Meta["allowed-tools"]
	if !set {
		return e, ""
	}
	names, problem := skillCapabilityTools(raw)
	if problem != "" {
		return e, problem
	}
	if _, scalar := raw.(string); scalar {
		e.Meta = maps.Clone(e.Meta)
		e.Meta["allowed-tools"] = strings.Join(names, ", ")
	} else {
		tools := make([]any, len(names))
		for i, name := range names {
			tools[i] = name
		}
		e.Meta = maps.Clone(e.Meta)
		e.Meta["allowed-tools"] = tools
	}
	return e, ""
}
