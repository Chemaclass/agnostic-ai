package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func codebuddyPrivateField(value any, path string, sensitive, inlineMCP bool) string {
	switch value := value.(type) {
	case map[string]any:
		for _, key := range codebuddyMapKeys(value) {
			next := key
			if path != "" {
				next = path + "." + key
			}
			if inlineMCP && key == "mcpServers" {
				if found := codebuddyInlineMCPField(value[key], next); found != "" {
					return found
				}
			}
			if found := codebuddyPrivateField(value[key], next, sensitive || mcpCredentialKey(key), inlineMCP); found != "" {
				return found
			}
		}
	case []any:
		for i, item := range value {
			if found := codebuddyPrivateField(item, path+"["+strconv.Itoa(i)+"]", sensitive, inlineMCP); found != "" {
				return found
			}
		}
	case string:
		if value == "" {
			return ""
		}
		tokens := spec.EnvRefTokens(value)
		refs := len(tokens) > 0
		known := true
		for _, token := range tokens {
			known = known && token.Known()
		}
		resolvable := inlineMCP && (strings.HasPrefix(path, "mcpServers.") || strings.HasPrefix(path, "mcpServers[") || strings.Contains(path, ".mcpServers.") || strings.Contains(path, ".mcpServers[")) && (strings.Contains(path, ".env.") || strings.Contains(path, ".headers.") || strings.HasSuffix(path, ".url") || strings.Contains(path, ".args["))
		referenceOnly := resolvable && refs && mcpRefOnly(value) && !strings.Contains(value, ":-") && !strings.Contains(value, "$${")
		if sensitive && !referenceOnly || mcpCredentialDetected(value) || refs && (!resolvable || !known) {
			return path
		}
	case nil:
	default:
		if sensitive {
			return path
		}
	}
	return ""
}

func codebuddyInlineMCPField(value any, path string) string {
	switch value := value.(type) {
	case []any:
		for i, item := range value {
			if found := codebuddyInlineMCPField(item, path+"["+strconv.Itoa(i)+"]"); found != "" {
				return found
			}
		}
	case map[string]any:
		probe, _ := jsonRoundTrip(value).(map[string]any)
		if refs := referenceMCPLiterals(probe); len(refs) > 0 {
			ref := refs[0]
			field := path + "." + ref.server + "." + ref.field
			if ref.key != "" {
				field += "." + ref.key
			}
			return field
		}
	}
	return ""
}

func guardCodebuddyValues(path string, value any, inlineMCP bool) error {
	if field := codebuddyNonStringKeyPath(value, ""); field != "" {
		return fmt.Errorf("%s: codebuddy: %s has non-string YAML keys; quote the keys before importing", path, field)
	}
	if field := codebuddyPrivateField(value, "", false, inlineMCP); field != "" {
		return fmt.Errorf("%s: codebuddy: %s contains a credential or an environment reference this native field cannot resolve; keep it outside imported specs", path, field)
	}
	return nil
}

func codebuddyNonStringKeyPath(value any, path string) string {
	switch value := value.(type) {
	case map[any]any:
		if path == "" {
			return "configuration"
		}
		return path
	case map[string]any:
		for _, key := range codebuddyMapKeys(value) {
			next := key
			if path != "" {
				next = path + "." + key
			}
			if field := codebuddyNonStringKeyPath(value[key], next); field != "" {
				return field
			}
		}
	case []any:
		for i, item := range value {
			if field := codebuddyNonStringKeyPath(item, path+"["+strconv.Itoa(i)+"]"); field != "" {
				return field
			}
		}
	}
	return ""
}
