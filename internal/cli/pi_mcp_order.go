package cli

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/pi"
)

type mcpSpecOrderHints map[string][]string

func piMCPMap(path string, data []byte, keys ...string) (map[string]any, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}, nil
	}
	data, _ = adapters.StripJSONC(data)
	if !strings.HasPrefix(strings.TrimSpace(string(data)), "{") {
		return nil, fmt.Errorf("parse %s: expected an MCP document object", path)
	}
	for _, key := range keys {
		object := adapters.NewOrderedJSON()
		if err := json.Unmarshal(data, object); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		raw, ok := object.Get(key)
		if !ok {
			return map[string]any{}, nil
		}
		data = raw
	}
	object := adapters.NewOrderedJSON()
	if err := json.Unmarshal(data, object); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	servers := map[string]any{}
	for _, name := range object.Keys() {
		raw, _ := object.Get(name)
		var server map[string]any
		if err := json.Unmarshal(raw, &server); err != nil {
			return nil, fmt.Errorf("parse %s MCP %s: %w", path, name, err)
		}
		if server == nil {
			return nil, fmt.Errorf("parse %s MCP %s: expected a server object", path, name)
		}
		fields := adapters.NewOrderedJSON()
		if err := json.Unmarshal(raw, fields); err != nil {
			return nil, fmt.Errorf("parse %s MCP %s: %w", path, name, err)
		}
		if exposure, exists := fields.Get("toolExposure"); exists {
			tools, ok := server["toolExposure"].(map[string]any)
			if !ok || !pi.ValidToolExposure(tools) {
				return nil, fmt.Errorf("parse %s MCP %s: invalid toolExposure object", path, name)
			}
			ordered := adapters.NewOrderedJSON()
			if err := json.Unmarshal(exposure, ordered); err != nil {
				return nil, fmt.Errorf("parse %s MCP %s toolExposure: %w", path, name, err)
			}
			server["toolExposure"] = ordered
		}
		servers[name] = server
	}
	return servers, nil
}

func readPiMCPMap(path string) (map[string]any, mcpSpecOrderHints, map[string]any, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil, nil, nil
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	ordered, err := piMCPMap(path, data, "mcpServers")
	if err != nil {
		return nil, nil, nil, err
	}
	orders := mcpSpecOrderHints{}
	for name, raw := range ordered {
		server, _ := raw.(map[string]any)
		if tools, ok := server["toolExposure"].(*adapters.OrderedJSON); ok {
			orders[name] = tools.Keys()
		}
	}
	plain, _ := jsonRoundTrip(ordered).(map[string]any)
	return plain, orders, ordered, nil
}

func marshalMCPSpec(doc map[string]any, name string, orders []mcpSpecOrderHints) ([]byte, error) {
	if len(orders) == 0 || len(orders[0][name]) == 0 {
		return yaml.Marshal(doc)
	}
	var node yaml.Node
	if err := node.Encode(doc); err != nil {
		return nil, err
	}
	native := mappingValue(&node, "x-pi")
	tools := mappingValue(native, "toolExposure")
	if tools != nil && tools.Kind == yaml.MappingNode {
		reordered := []*yaml.Node{}
		used := map[string]bool{}
		for _, key := range orders[0][name] {
			if i := mappingIndex(tools, key); i >= 0 && !used[key] {
				reordered = append(reordered, tools.Content[i], tools.Content[i+1])
				used[key] = true
			}
		}
		for i := 0; i+1 < len(tools.Content); i += 2 {
			if !used[tools.Content[i].Value] {
				reordered = append(reordered, tools.Content[i], tools.Content[i+1])
			}
		}
		tools.Content = reordered
	}
	return yaml.Marshal(&node)
}

type piExposureComparison struct {
	Exact        map[string]any
	Patterns     [][2]string
	UnknownOrder bool
}

func canonicalPiToolExposure(value any) any {
	if value == nil {
		return nil
	}
	comparison := piExposureComparison{Exact: map[string]any{}}
	var keys []string
	var tools map[string]any
	switch object := value.(type) {
	case *adapters.OrderedJSON:
		keys = object.Keys()
		tools, _ = jsonRoundTrip(object).(map[string]any)
	case map[string]any:
		tools = object
		keys = slices.Sorted(maps.Keys(object))
	default:
		return struct{ Invalid any }{value}
	}
	if len(tools) == 0 {
		return nil
	}
	for _, key := range keys {
		if strings.Contains(key, "*") {
			exposure, _ := tools[key].(string)
			if exposure == "codemode-deferred" {
				exposure = "codemode"
			}
			comparison.Patterns = append(comparison.Patterns, [2]string{key, exposure})
		} else {
			value := tools[key]
			if value == "codemode-deferred" {
				value = "codemode"
			}
			comparison.Exact[key] = value
		}
	}
	if _, ordered := value.(*adapters.OrderedJSON); !ordered && len(comparison.Patterns) > 1 {
		comparison.UnknownOrder = true
	}
	return comparison
}
