package cli

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// Only declared paths count; a fresh clone omits empty source directories.
func missingSourceNotes(root string) []validationIssue {
	data, ok := readConfigFile(root)
	if !ok {
		return nil
	}
	var raw struct {
		Sources config.Sources `yaml:"sources"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil
	}
	declared := []struct{ kind, path string }{
		{"agents", raw.Sources.Agents},
		{"skills", raw.Sources.Skills},
		{"rules", raw.Sources.Rules},
		{"hooks", raw.Sources.Hooks},
		{"mcps", raw.Sources.MCPs},
		{"commands", raw.Sources.Commands},
		{"settings", raw.Sources.Settings},
		{"reviews", raw.Sources.Reviews},
		{"environments", raw.Sources.Environments},
		{"ignore", raw.Sources.Ignore},
	}
	var out []validationIssue
	for _, d := range declared {
		if d.path == "" {
			continue
		}
		info, err := os.Stat(config.ResolveSourcePath(root, filepath.FromSlash(d.path)))
		if err == nil && info.IsDir() {
			continue
		}
		out = append(out, validationIssue{
			Path:    d.path,
			Field:   "sources." + d.kind,
			Message: "directory not found (no " + d.kind + " will be emitted)",
		})
	}
	return out
}

// readConfigFile returns the raw bytes of the project config, trying the
// current name then the legacy one. The bool reports whether a file was
// read.
func readConfigFile(root string) ([]byte, bool) {
	for _, name := range []string{config.ConfigFileName, config.LegacyConfigFileName} {
		if data, err := os.ReadFile(filepath.Join(root, name)); err == nil {
			return data, true
		}
	}
	return nil, false
}
