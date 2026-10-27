package opencode

import (
	"fmt"
	"maps"
	"slices"

	"gopkg.in/yaml.v3"
)

var agentV2Keys = []string{"permissions", "request", "system", "disabled"}

// agentV2 reports whether OpenCode 2 reads the file as V2, which ignores V1 fields.
func agentV2(front map[string]any) bool {
	return slices.ContainsFunc(agentV2Keys, func(key string) bool {
		_, present := front[key]
		return present
	})
}

// agentV2Temperature moves temperature to request.body, where OpenCode puts
// it when it migrates a V1 file. A temperature already in request.body wins.
func agentV2Temperature(front map[string]any) error {
	temperature, present := front["temperature"]
	if !present || !agentV2(front) {
		return nil
	}
	delete(front, "temperature")
	request := map[string]any{}
	if raw, set := front["request"]; set {
		existing, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("agent request: expected an object with headers and body")
		}
		request = maps.Clone(existing)
	}
	body := map[string]any{}
	if raw, set := request["body"]; set {
		existing, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("agent request.body: expected an object")
		}
		body = maps.Clone(existing)
	}
	if _, set := body["temperature"]; !set {
		body["temperature"] = temperature
	}
	request["body"] = body
	front["request"] = request
	return nil
}

func agentV2Permissions(front map[string]any, keys *[]string) error {
	if !agentV2(front) {
		return nil
	}
	legacy, present := front[permissionKey]
	if !present {
		return nil
	}
	node := &yaml.Node{}
	if err := node.Encode(legacy); err != nil {
		return fmt.Errorf("encode agent permission: %w", err)
	}
	rules := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	migrator := &agentProject{native: true}
	add := func(action, resource, effect string) error {
		rule := &yaml.Node{}
		if err := rule.Encode(map[string]string{"action": migrator.actionKey(action), "resource": resource, "effect": effect}); err != nil {
			return fmt.Errorf("encode agent permissions: %w", err)
		}
		rules.Content = append(rules.Content, rule)
		return nil
	}
	if node.Kind == yaml.ScalarNode {
		if err := add(catchAllPattern, catchAllPattern, node.Value); err != nil {
			return err
		}
	} else {
		for i := 0; i+1 < len(node.Content); i += 2 {
			action, value := node.Content[i].Value, node.Content[i+1]
			if value.Kind == yaml.ScalarNode {
				if err := add(action, catchAllPattern, value.Value); err != nil {
					return err
				}
				continue
			}
			for j := 0; j+1 < len(value.Content); j += 2 {
				if err := add(action, value.Content[j].Value, value.Content[j+1].Value); err != nil {
					return err
				}
			}
		}
	}
	if native, present := front["permissions"]; present {
		node := &yaml.Node{}
		if err := node.Encode(native); err != nil {
			return fmt.Errorf("encode agent permissions: %w", err)
		}
		if node.Kind != yaml.SequenceNode {
			return fmt.Errorf("agent permissions: expected an array of action/resource/effect rules")
		}
		rules.Content = append(rules.Content, node.Content...)
	}
	delete(front, permissionKey)
	front["permissions"] = rules
	if !slices.Contains(*keys, "permissions") {
		*keys = append(*keys, "permissions")
	}
	return nil
}
