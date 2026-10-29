package cli

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/preview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

var qwenCredentialEnvName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func guardQwenValues(path string, value any, resolveRefs bool) error {
	if field := qwenNonStringKeyPath(value, ""); field != "" {
		return fmt.Errorf("%s: qwen: %s has non-string YAML keys; quote the keys before importing", path, field)
	}
	if field := qwenPrivateField(value, "", false, resolveRefs); field != "" {
		return fmt.Errorf("%s: qwen: %s contains a credential or an environment reference this native field cannot resolve; keep it outside imported specs", path, field)
	}
	return nil
}

func qwenNonStringKeyPath(value any, path string) string {
	switch value := value.(type) {
	case map[any]any:
		if path == "" {
			return "configuration"
		}
		return path
	case map[string]any:
		for _, key := range slices.Sorted(maps.Keys(value)) {
			next := key
			if path != "" {
				next = path + "." + key
			}
			if field := qwenNonStringKeyPath(value[key], next); field != "" {
				return field
			}
		}
	case []any:
		for i, item := range value {
			if field := qwenNonStringKeyPath(item, path+"["+strconv.Itoa(i)+"]"); field != "" {
				return field
			}
		}
	}
	return ""
}

func qwenPrivateField(value any, path string, sensitive, resolveRefs bool) string {
	switch value := value.(type) {
	case map[string]any:
		for _, key := range slices.Sorted(maps.Keys(value)) {
			next := key
			if path != "" {
				next = path + "." + key
			}
			if key == "mcpServers" {
				if servers, ok := jsonRoundTrip(value[key]).(map[string]any); ok {
					if refs := referenceMCPLiterals(servers); len(refs) > 0 {
						field := next + "." + refs[0].server + "." + refs[0].field
						if refs[0].key != "" {
							field += "." + refs[0].key
						}
						return field
					}
				}
			}
			if resolveRefs && key == "envKey" && (strings.HasPrefix(path, "modelProviders.") || path == "memory.mem0") {
				if name, ok := value[key].(string); ok && qwenCredentialEnvName.MatchString(name) && !preview.CredentialDetected(name) {
					continue
				}
				return next
			}
			childSensitive := sensitive || preview.CredentialKey(key)
			if resolveRefs && path == "security" && key == "auth" {
				if _, object := value[key].(map[string]any); object {
					childSensitive = sensitive
				}
			}
			if field := qwenPrivateField(value[key], next, childSensitive, resolveRefs); field != "" {
				return field
			}
		}
	case []any:
		for i, item := range value {
			credentialArg := false
			if i > 0 && strings.HasSuffix(path, ".args") {
				if previous, ok := value[i-1].(string); ok {
					name, _, hasValue := preview.CredentialFlag(previous)
					credentialArg = !hasValue && preview.CredentialName(name)
				}
			}
			if field := qwenPrivateField(item, path+"["+strconv.Itoa(i)+"]", sensitive || credentialArg, resolveRefs); field != "" {
				return field
			}
		}
	case string:
		if value == "" {
			return ""
		}
		if _, flagValue, hasValue := preview.CredentialFlag(value); hasValue {
			return qwenPrivateField(flagValue, path, true, resolveRefs)
		}
		referenceOnly := resolveRefs && preview.RefOnly(value) && !strings.Contains(value, ":-") && !strings.Contains(value, "$${")
		for _, token := range spec.EnvRefTokens(value) {
			referenceOnly = referenceOnly && token.Known()
		}
		if sensitive && !referenceOnly || preview.CredentialDetected(value) {
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
