package approvals

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// RuleProjectConfig names the ask when a project config file, whose
// sources.* settings may move spec folders, does not read.
const RuleProjectConfig = "project-config"

// specSourceKeys are the sources.* settings whose specs sync turns into
// agents, hooks, MCP servers or tool settings that run without a prompt.
var specSourceKeys = []string{"agents", "hooks", "mcps", "settings"}

// SpecSources returns the absolute folders that the project config
// files in dir, base and local alike, move agent, hook, MCP or settings
// specs to. It is the SpecSources a hook passes in a Policy. A file that
// exists but does not parse, a source that is not text, or one that
// names dir itself or a folder above it is an error, so the gate asks.
func SpecSources(dir string) ([]string, error) {
	var dirs []string
	for _, name := range []string{config.ConfigFileName, config.LegacyConfigFileName, config.LocalOverrideFileName} {
		path := filepath.Join(dir, name)
		found, err := specSources(path)
		if err != nil {
			return nil, err
		}
		for _, source := range found {
			if !filepath.IsAbs(source) {
				source = filepath.Join(dir, source)
			}
			source = filepath.Clean(source)
			if under(dir, source) {
				return nil, fmt.Errorf("%s: a spec source names %s or a folder above it", path, dir)
			}
			dirs = append(dirs, source)
		}
	}
	return dirs, nil
}

func specSources(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var file struct {
		Sources map[string]any `yaml:"sources"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var out []string
	for _, key := range specSourceKeys {
		value, set := file.Sources[key]
		if !set || value == nil {
			continue
		}
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("%s: sources.%s is not a path", path, key)
		}
		if text != "" {
			out = append(out, text)
		}
	}
	return out, nil
}
