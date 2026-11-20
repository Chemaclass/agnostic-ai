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

// defaultSpecDirs are the spec folders, under a project config's own
// folder, whose specs sync turns into agents, hooks, servers or settings.
var defaultSpecDirs = []string{"agents", "hooks", "mcps", "settings", "overlays", "scripts", "packs"}

// SpecSources returns the absolute folders that the project config
// files in dir, base and local alike, keep agent, hook, MCP or settings
// specs in: the default folders under dir/.agnostic-ai when any config
// file is there, and each folder sources.* moves them to. It is the
// SpecSources a hook passes in a Policy. A file that exists but does not
// parse, a source that is not text, or one that names dir itself or a
// folder above it is an error, so the gate asks.
func SpecSources(dir string) ([]string, error) {
	var dirs []string
	for _, name := range []string{config.ConfigFileName, config.LegacyConfigFileName, config.LocalOverrideFileName} {
		path := filepath.Join(dir, name)
		found, err := specSources(path)
		if err != nil {
			return nil, err
		}
		if found != nil && len(dirs) == 0 {
			for _, sub := range defaultSpecDirs {
				dirs = append(dirs, filepath.Join(dir, config.SourceBaseDir, sub), filepath.Join(dir, config.SourceBaseDir, "local", sub))
			}
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

// specSources returns the sources.* folders path names. found is nil
// when path does not exist and empty, not nil, when it names none.
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
	out := []string{}
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
