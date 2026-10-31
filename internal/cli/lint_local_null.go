package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"gopkg.in/yaml.v3"
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
	var keys []string
	var walk func(map[string]any, string)
	walk = func(m map[string]any, prefix string) {
		for key, value := range m {
			name := key
			if prefix != "" {
				name = prefix + "." + key
			}
			if value == nil {
				keys = append(keys, name)
				continue
			}
			if nested, ok := value.(map[string]any); ok {
				walk(nested, name)
			}
		}
	}
	walk(values, "")
	slices.Sort(keys)
	var findings []lintFinding
	for _, key := range keys {
		findings = append(findings, lintFinding{Code: "LINT055", Severity: lintWarn, Path: path, Message: fmt.Sprintf("%s is null and can replace the shared value; remove the key to inherit it. A future release will inherit on null", key)})
	}
	return findings, nil
}
