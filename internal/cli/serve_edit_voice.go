package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/overview"
)

func voiceEditField(key string) bool {
	return key == "voice.enabled" || key == "voice.mode" || key == "voice.language"
}

func decodeVoiceEdit(key string, raw json.RawMessage) (any, error) {
	if key == "voice.enabled" {
		var value bool
		if json.Unmarshal(raw, &value) != nil {
			return nil, errors.New("voice.enabled must be true or false")
		}
		return value, nil
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return nil, fmt.Errorf("%s must be a string", key)
	}
	if key == "voice.mode" && value != "hold" && value != "tap" {
		return nil, errors.New("voice.mode must be hold or tap")
	}
	if key == "voice.language" && strings.TrimSpace(value) == "" {
		return nil, errors.New("voice.language must be a non-empty string")
	}
	return value, nil
}

func setVoiceFieldAt(data []byte, root *yaml.Node, key string, value any) ([]byte, error) {
	if root.Anchor != "" || root.Style&yaml.FlowStyle != 0 {
		return nil, errors.New("flow-style or anchored settings are not safely editable")
	}
	var before map[string]any
	if err := yaml.Unmarshal(data, &before); err != nil {
		return nil, err
	}
	member := strings.TrimPrefix(key, "voice.")
	_, parent := yamlMapValue(root, "voice")
	_, present := overview.SettingsFieldValue(before, key)
	if value == nil && !present {
		return nil, errFieldNotInFile
	}
	var out []byte
	var err error
	switch {
	case parent == nil:
		if _, inherited := before["voice"]; inherited {
			return nil, errors.New("inherited settings are not safely editable")
		}
		out, err = editYAMLPair(data, root, "voice", map[string]any{member: value})
	case parent.Kind != yaml.MappingNode || parent.Anchor != "" || parent.Style&yaml.FlowStyle != 0:
		return nil, errors.New("voice must be a plain mapping to edit it")
	case value == nil:
		out, err = removeYAMLPath(data, key)
	default:
		_, node := yamlMapValue(parent, member)
		if node == nil {
			if present {
				return nil, errors.New("inherited settings are not safely editable")
			}
			out, err = editYAMLPair(data, parent, member, value)
		} else {
			if node.Kind != yaml.ScalarNode || node.Anchor != "" || node.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
				return nil, errors.New("multi-line, anchored, or non-scalar settings are not safely editable")
			}
			start, end, spanErr := yamlScalarSpan(data, node)
			if spanErr != nil {
				return nil, spanErr
			}
			raw, encodeErr := yaml.Marshal(value)
			if encodeErr != nil {
				return nil, encodeErr
			}
			out = spliceEditYAML(data, start, end, strings.TrimSpace(string(raw)))
		}
	}
	if err != nil {
		return nil, err
	}
	expected := maps.Clone(before)
	voice, _ := before["voice"].(map[string]any)
	voice = maps.Clone(voice)
	if voice == nil {
		voice = map[string]any{}
	}
	if value == nil {
		delete(voice, member)
	} else {
		voice[member] = value
	}
	if len(voice) == 0 {
		delete(expected, "voice")
	} else {
		expected["voice"] = voice
	}
	var actual map[string]any
	if err := yaml.Unmarshal(out, &actual); err != nil {
		return nil, fmt.Errorf("the edit does not parse: %w", err)
	}
	if !reflect.DeepEqual(expected, actual) {
		return nil, errors.New("the edit changes other values")
	}
	return out, nil
}
