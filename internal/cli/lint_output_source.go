package cli

import (
	"os"

	"gopkg.in/yaml.v3"
)

// outputKeySource returns the config file that sets outputs.<target>.<key>:
// the last of paths that sets it, since a later file overrides an earlier
// one, or paths[0] when none can be read.
func outputKeySource(paths []string, target, key string) string {
	source := paths[0]
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var doc struct {
			Outputs map[string]map[string]any `yaml:"outputs"`
		}
		if yaml.Unmarshal(data, &doc) != nil {
			continue
		}
		if v, ok := doc.Outputs[target][key]; ok && v != nil && v != "" {
			source = path
		}
	}
	return source
}
