package cli

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

func importGrokAgentMetadata(data []byte) ([]byte, error) {
	if !strings.HasPrefix(string(data), "---") {
		return data, nil
	}
	mapping, _, ok := frontmatterNode(data)
	if !ok {
		return nil, errors.New("invalid Grok agent frontmatter")
	}
	var meta map[string]any
	if err := mapping.Decode(&meta); err != nil {
		return nil, errors.New("invalid Grok agent frontmatter")
	}
	if err := validateGrokMetadataKeys(meta); err != nil {
		return nil, err
	}
	if raw, present := meta["mcpServers"]; present {
		refs, ok := raw.([]any)
		if !ok {
			return nil, errors.New("invalid Grok agent MCP metadata")
		}
		for _, raw := range refs {
			if _, named := raw.(string); named {
				continue
			}
			entry, ok := raw.(map[string]any)
			if !ok {
				return nil, errors.New("invalid Grok agent MCP metadata")
			}
			if len(entry) == 1 {
				for _, raw := range entry {
					entry, ok = raw.(map[string]any)
				}
			} else {
				_, ok = entry["name"].(string)
			}
			if !ok {
				return nil, errors.New("invalid Grok agent MCP metadata")
			}
			probe, _ := jsonRoundTrip(entry).(map[string]any)
			if probe == nil {
				return nil, errors.New("invalid Grok agent MCP metadata")
			}
			for _, field := range []string{"env", "headers"} {
				if list, ok := probe[field].([]any); ok {
					values := map[string]any{}
					for _, raw := range list {
						pair, ok := raw.(map[string]any)
						if !ok {
							return nil, errors.New("invalid Grok agent MCP metadata")
						}
						name, ok := pair["name"].(string)
						if !ok {
							return nil, errors.New("invalid Grok agent MCP metadata")
						}
						values[name] = pair["value"]
					}
					probe[field] = values
				}
			}
			if err := validateGrokRetainedCredentials(map[string]any{"inline": probe}); err != nil {
				return nil, err
			}
		}
	}
	if raw, present := meta["hooks"]; present {
		hooks, ok := raw.(map[string]any)
		if !ok {
			return nil, errors.New("invalid Grok agent hook metadata")
		}
		if err := validateGrokRetainedCredentials(map[string]any{"inline": hooks}); err != nil {
			return nil, err
		}
		if err := validateGrokHookCredentials(grokHookEntries(hooks)); err != nil {
			return nil, err
		}
	}
	return data, nil
}

func validateGrokMetadataKeys(value any) error {
	if field := grokNonStringKeyPath(value, ""); field != "" {
		return fmt.Errorf("%s has non-string YAML keys; quote the keys before importing", field)
	}
	return nil
}

func grokNonStringKeyPath(value any, path string) string {
	switch value := value.(type) {
	case map[any]any:
		if path == "" {
			return "metadata"
		}
		return path
	case map[string]any:
		for _, key := range slices.Sorted(maps.Keys(value)) {
			next := key
			if path != "" {
				next = path + "." + key
			}
			if field := grokNonStringKeyPath(value[key], next); field != "" {
				return field
			}
		}
	case []any:
		for i, item := range value {
			if field := grokNonStringKeyPath(item, path+"["+strconv.Itoa(i)+"]"); field != "" {
				return field
			}
		}
	}
	return ""
}

func validateGrokRetainedCredentials(entries map[string]any) error {
	unsafe := false
	walkMCPStrings("", "", entries, func(_, key, value string) {
		if credentialSettingValue(key, value) {
			unsafe = true
		}
	})
	for _, raw := range entries {
		entry, _ := raw.(map[string]any)
		args := toStringSlice(entry["args"])
		for i, arg := range args {
			flag, value, hasValue := mcpCredentialFlag(arg)
			if flag == "" {
				continue
			}
			if !hasValue && i+1 < len(args) {
				value = args[i+1]
			}
			if credentialSettingValue(flag, value) {
				unsafe = true
			}
		}
	}
	if unsafe {
		return errors.New("native metadata contains a credential; remove literal credentials before importing")
	}
	return withoutMCPCredentials("grok", withoutReferenceSettings(entries), func(_, _, _ string) error {
		return errors.New("native metadata contains a credential; remove literal credentials before importing")
	})
}

// withoutReferenceSettings returns a copy of entries with each `env` and
// `headers` value the shared setting rule reads as only references
// emptied, so the MCP filter, which reads `$NAME` as literal, checks the
// rest by the same rule.
func withoutReferenceSettings(entries map[string]any) map[string]any {
	probe, _ := jsonRoundTrip(entries).(map[string]any)
	for _, raw := range probe {
		entry, _ := raw.(map[string]any)
		for _, field := range []string{"env", "headers"} {
			values, _ := entry[field].(map[string]any)
			for key, value := range values {
				if text, ok := value.(string); ok && !literalSettingValue(key, text) {
					values[key] = ""
				}
			}
		}
	}
	return probe
}

func validateGrokHookCredentials(entries map[string]any) error {
	if err := validateGrokRetainedCredentials(entries); err != nil {
		return fmt.Errorf("%w; hook env values are literal, inherit credentials from the process and reference them in command or url", err)
	}
	return nil
}
