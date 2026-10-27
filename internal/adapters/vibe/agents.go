package vibe

import (
	"fmt"
	"maps"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

var tools = map[string]string{"Bash": "bash", "Read": "read_file", "Write": "write_file", "Edit": "edit"}

// Vibe 3.0.3 refuses an agent file whose stem is main in any case: a
// post_agent hook's match = "main" selects the user's agent.
const reservedAgentName = "main"

func IsReservedAgentName(name string) bool { return strings.EqualFold(name, reservedAgentName) }

func (Adapter) EmitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	for _, a := range agents {
		path := filepath.Join(dir, a.Name+".toml")
		if IsReservedAgentName(a.Name) {
			return fmt.Errorf("vibe: agent %s: Vibe reserves the name main for hook matching and does not load %s; rename the agent", a.Name, path)
		}
		if _, ok := a.Meta["can"]; ok {
			native, reason := a.NativeToolsFor(target)
			if reason != "" {
				return fmt.Errorf("vibe: agent %s: %s", a.Name, reason)
			}
			a = native
		}
		m := emit.ResolveMeta(a.Meta, target)
		doc := native(a.Meta)
		doc["agent_type"] = "subagent"
		doc["description"] = a.Name
		if d, ok := m["description"]; ok {
			doc["description"] = d
		}
		doc["instructions"] = strings.TrimSpace(a.Body)
		if v, ok := m["model"]; ok {
			doc["active_model"] = v
		}
		_, nativeTools := native(a.Meta)["enabled_tools"]
		if can, ok := a.Meta["tools"]; ok && !nativeTools {
			var list []string
			var omitted []string
			for _, name := range stringsList(can) {
				tool, ok := tools[name]
				if !ok {
					omitted = append(omitted, name)
					continue
				}
				list = append(list, tool)
			}
			if len(list) == 0 {
				return fmt.Errorf("vibe: agent %s has no supported allowed tools; an empty list would enable all tools; set x-vibe.enabled_tools explicitly", a.Name)
			}
			if len(omitted) > 0 {
				emit.NoteAgentToolsNoOp(target, []spec.Entry{a}, "unsupported names "+strings.Join(omitted, ", ")+" are omitted; native enabled_tools keeps the supported restriction")
			}
			doc["enabled_tools"] = list
		}
		maps.Copy(doc, native(a.Meta))
		if v, ok := doc["enabled_tools"]; ok && len(stringsList(v)) == 0 {
			return fmt.Errorf("vibe: agent %s empty enabled_tools would enable all tools", a.Name)
		}
		if err := writeDocument(sess, path, doc, true, dryRun); err != nil {
			return err
		}
	}
	return nil
}
func native(meta map[string]any) map[string]any {
	m, _ := meta["x-vibe"].(map[string]any)
	return maps.Clone(nonNil(m))
}
func nonNil(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}
func stringsList(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		var r []string
		for _, a := range x {
			if s, ok := a.(string); ok {
				r = append(r, s)
			}
		}
		return r
	case string:
		return strings.FieldsFunc(x, func(c rune) bool { return c == ',' || c == ' ' })
	}
	return nil
}
func (Adapter) ToolCapabilities() emit.ToolCapabilityTable {
	return emit.ToolCapabilityTable{
		"read": {Agent: []string{"read_file"}}, "edit": {Agent: []string{"edit"}}, "write": {Agent: []string{"write_file"}}, "shell": {Agent: []string{"bash"}},
	}
}

func (Adapter) TranslateAgentCapability(name string) ([]string, bool) {
	if tool, ok := tools[name]; ok {
		return []string{tool}, true
	}
	return nil, false
}
