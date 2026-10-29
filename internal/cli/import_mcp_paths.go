package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// importMCPSpecPaths picks the spec file each MCP server name imports to:
// the spec in dstDir that already holds the name, so a re-import never
// adds a second spec for one server, or fileName(name) in dstDir. It
// refuses a new file name that Windows would store as a hidden stream
// before anything is written.
func importMCPSpecPaths(dstDir string, names []string, fileName func(string) string) (map[string]string, error) {
	held, err := heldMCPSpecPaths(dstDir)
	if err != nil {
		return nil, err
	}
	paths := make(map[string]string, len(names))
	for _, name := range names {
		if path, ok := held[name]; ok {
			paths[name] = path
			continue
		}
		path := filepath.Join(dstDir, fileName(name))
		if config.NamesDataStream(path) {
			return nil, fmt.Errorf("MCP server %q: %s: %w; rename the server, or keep it in a spec file whose name has no colon", name, path, errImportDataStream)
		}
		paths[name] = path
	}
	return paths, nil
}

// heldMCPSpecPaths maps each server name the MCP specs in dstDir hold to
// the first spec file, by name, that holds it.
func heldMCPSpecPaths(dstDir string) (map[string]string, error) {
	entries, err := os.ReadDir(dstDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dstDir, err)
	}
	held := map[string]string{}
	for _, e := range entries {
		if !e.Type().IsRegular() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(dstDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		var doc struct {
			Name string `yaml:"name"`
		}
		if yaml.Unmarshal(data, &doc) != nil || doc.Name == "" {
			continue
		}
		if _, seen := held[doc.Name]; !seen {
			held[doc.Name] = path
		}
	}
	return held, nil
}
