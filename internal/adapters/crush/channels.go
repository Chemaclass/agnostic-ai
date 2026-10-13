package crush

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func channelFields(e spec.Entry) (map[string]any, error) {
	out := map[string]any{}
	extension, _ := e.Meta["x-crush"].(map[string]any)
	for _, key := range []string{"channel_enabled", "channel_reply"} {
		value, present := e.Meta[key]
		if override, ok := extension[key]; ok {
			value, present = override, true
		}
		if !present {
			continue
		}
		if key == "channel_enabled" {
			enabled, ok := value.(bool)
			if !ok {
				return nil, fmt.Errorf("%s must be a boolean", key)
			}
			out[key] = enabled
			continue
		}
		reply, err := channelReplyFields(value)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		out[key] = reply
	}
	return out, nil
}

func channelReplyFields(value any) (map[string]any, error) {
	input, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("must be an object")
	}
	out := map[string]any{}
	for key, value := range input {
		switch key {
		case "user", "group":
			route, err := channelReplyRoute(value)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}
			out[key] = route
		case "message_param":
			text, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("%s must be a string", key)
			}
			out[key] = text
		case "suppress_tools":
			var tools []string
			switch list := value.(type) {
			case []string:
				tools = list
			case []any:
				tools = make([]string, 0, len(list))
				for _, item := range list {
					text, ok := item.(string)
					if !ok {
						return nil, fmt.Errorf("%s must be a list of strings", key)
					}
					tools = append(tools, text)
				}
			default:
				return nil, fmt.Errorf("%s must be a list of strings", key)
			}
			if tools == nil {
				tools = []string{}
			}
			out[key] = tools
		default:
			return nil, fmt.Errorf("unknown field %q", key)
		}
	}
	return out, nil
}

func channelReplyRoute(value any) (map[string]any, error) {
	input, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("must be an object")
	}
	out := map[string]any{}
	for key, value := range input {
		switch key {
		case "tool", "target_param", "target_meta":
			text, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("%s must be a string", key)
			}
			out[key] = text
		default:
			return nil, fmt.Errorf("unknown field %q", key)
		}
	}
	for _, key := range []string{"tool", "target_param"} {
		if _, ok := out[key]; !ok {
			return nil, fmt.Errorf("%s is required", key)
		}
	}
	return out, nil
}
