package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	layerNameProject     = "project"
	layerNameProjectUser = "project-user"

	defaultProjectUser = ".agnostic-ai.local"
)

// defaultLayerSources is the fixed source layout used by the project-user
// layer. The project layer keeps its configurable
// `cfg.Sources` paths.
func defaultLayerSources() config.Sources {
	return config.Sources{
		Agents:       "agents",
		Skills:       "skills",
		Rules:        "rules",
		Hooks:        "hooks",
		MCPs:         "mcps",
		Commands:     "commands",
		Settings:     "settings",
		Reviews:      "reviews",
		Environments: "environments",
		Ignore:       "ignore",
	}
}

// resolveLayers returns the ordered list of layers to load, low- to
// high-precedence. Optional layers are skipped when their root does
// not exist.
func resolveLayers(projectRoot string, cfg *config.Config) []spec.Layer {
	var layers []spec.Layer
	layers = append(layers, resolvePacksLayers(projectRoot)...)
	layers = append(layers, resolveProjectLayer(projectRoot, cfg))
	if l, ok := resolveProjectUserLayer(projectRoot); ok {
		layers = append(layers, l)
	}
	return layers
}

// resolveProjectLayer returns the always-present project layer using
// the configurable source paths from cfg.
func resolveProjectLayer(projectRoot string, cfg *config.Config) spec.Layer {
	return spec.Layer{
		Name:    layerNameProject,
		Root:    projectRoot,
		Sources: cfg.Sources,
	}
}

// resolveProjectUserLayer returns the project-user layer when
// `<projectRoot>/.agnostic-ai.local` exists.
func resolveProjectUserLayer(projectRoot string) (spec.Layer, bool) {
	pu := filepath.Join(projectRoot, defaultProjectUser)
	if !dirExists(pu) {
		return spec.Layer{}, false
	}
	return spec.Layer{
		Name:    layerNameProjectUser,
		Root:    pu,
		Sources: defaultLayerSources(),
	}, true
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "! stat %s: %v\n", path, err)
		}
		return false
	}
	return info.IsDir()
}
