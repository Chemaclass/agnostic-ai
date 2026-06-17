package cli

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// lintMissingSources warns for each `sources.<kind>` path explicitly
// declared in agnostic-ai.yaml that does not resolve to an existing
// directory under root. Only declared paths count: defaults that
// config.Load fills in are conventions, not a coverage claim, so a
// minimal project (just rules/) is never nagged about the kinds it never
// asked for.
//
// A declared-but-missing source emits nothing today and sync stays
// silent, so a config can advertise coverage it never delivers (#444).
// These surface as warnings, not errors, so a forward-looking scaffold
// that lists dirs before they exist still validates.
func lintMissingSources(root string) []validationIssue {
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
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(d.path)))
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
