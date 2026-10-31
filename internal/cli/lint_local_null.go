package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func lintLocalNulls() ([]lintFinding, error) {
	path := config.LocalOverrideFileName
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var values map[string]any
	if err := yaml.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var shared map[string]any
	basePath, _, err := config.ResolveConfigPath(".")
	if err != nil {
		return nil, err
	}
	baseData, err := os.ReadFile(basePath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", basePath, err)
	}
	if err := yaml.Unmarshal(baseData, &shared); err != nil {
		return nil, fmt.Errorf("parse %s: %w", basePath, err)
	}
	messages := map[string]string{}
	var walk func(local, base map[string]any, prefix string)
	walk = func(local, base map[string]any, prefix string) {
		for key, value := range local {
			name := key
			if prefix != "" {
				name = prefix + "." + key
			}
			baseValue, inBase := base[key]
			isTier := prefix == "models"
			if value == nil {
				switch {
				case name == "targets" && !inBase:
					messages[name] = name + " is empty (null) and replaces the default targets; remove the key to inherit them"
				case !inBase, baseValue == nil, name == "requires":
				case isYAMLMap(baseValue) && len(baseValue.(map[string]any)) == 0:
				case isYAMLMap(baseValue) && isTier:
					messages[name] = name + " is empty (null) and is ignored, so the shared tier stays; remove the key. A tier written as {} fails to load"
				case isYAMLMap(baseValue):
					messages[name] = name + " is empty (null) and is ignored, so the shared value stays; remove the key, or write {} to clear it"
				default:
					messages[name] = name + " is empty (null) and replaces the shared value; remove the key to inherit it"
				}
				continue
			}
			if nested, ok := value.(map[string]any); ok {
				baseNested, _ := baseValue.(map[string]any)
				if isTier {
					baseNested = nil
				}
				walk(nested, baseNested, name)
			}
		}
	}
	walk(values, shared, "")
	keys := make([]string, 0, len(messages))
	for key := range messages {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	var findings []lintFinding
	for _, key := range keys {
		findings = append(findings, lintFinding{Code: "LINT055", Severity: lintWarn, Path: path, Message: messages[key]})
	}
	return findings, nil
}

func isYAMLMap(value any) bool {
	_, ok := value.(map[string]any)
	return ok
}
