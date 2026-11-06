package cli

import (
	"encoding/json"
	"reflect"

	"gopkg.in/yaml.v3"
)

// importKiroAgentServers rewrites a Kiro agent's inline `mcpServers` map,
// since the portable field is a list of MCP spec names. When every server
// matches its enabled entry in `.kiro/settings/mcp.json`, which import
// turns into MCP specs, the map becomes a list of their names and sync
// writes the same servers back. Any other map moves whole under
// `x-kiro.mcpServers`, which sync writes as is: Kiro reads the agent's map
// in place of the project list, so splitting it would drop servers.
func importKiroAgentServers(mapping *yaml.Node, project map[string]any) (bool, error) {
	at := -1
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == "mcpServers" && mapping.Content[i+1].Kind == yaml.MappingNode {
			at = i
		}
	}
	if at < 0 {
		return false, nil
	}
	inline := mapping.Content[at+1]
	if names, ok, err := kiroProjectServerNames(inline, project); err != nil {
		return false, err
	} else if ok {
		list := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
		for _, name := range names {
			list.Content = append(list.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name})
		}
		mapping.Content[at+1] = list
		return true, nil
	}
	mapping.Content = append(mapping.Content[:at], mapping.Content[at+2:]...)
	setMappingNode(xTargetMapping(mapping, "x-kiro"), "mcpServers", inline)
	return true, nil
}

// kiroProjectServerNames returns the server names of inline, in order,
// when each one equals an enabled server of the project MCP file.
func kiroProjectServerNames(inline *yaml.Node, project map[string]any) ([]string, bool, error) {
	if len(inline.Content) == 0 {
		return nil, false, nil
	}
	var names []string
	for i := 0; i+1 < len(inline.Content); i += 2 {
		name := inline.Content[i].Value
		shared, found := project[name].(map[string]any)
		if !found || shared["disabled"] == true {
			return nil, false, nil
		}
		var server any
		if err := inline.Content[i+1].Decode(&server); err != nil {
			return nil, false, err
		}
		// A JSON round trip gives YAML values the types the project file
		// decodes to, such as float64 for a number.
		data, err := json.Marshal(server)
		if err != nil {
			return nil, false, nil
		}
		var asJSON any
		if err := json.Unmarshal(data, &asJSON); err != nil || !reflect.DeepEqual(asJSON, shared) {
			return nil, false, nil
		}
		names = append(names, name)
	}
	return names, true, nil
}
