package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	layerNameProject     = "project"
	layerNameProjectUser = "local"
	// layerNameUser and layerNameUserLocal are the layers of the user
	// home and of its local/ folder.
	layerNameUser      = "user"
	layerNameUserLocal = "user-local"

	// defaultProjectUser sits inside the source dir, the same shape as the
	// global `~/.agnostic-ai/local/`.
	defaultProjectUser = config.SourceBaseDir + "/local"
)

// defaultLayerSources is the fixed source layout used by the local
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
func resolveLayers(projectRoot string, cfg *config.Config) ([]spec.Layer, error) {
	// Only sync --scope user writes the approvals hooks, into user-level files:
	// a committed project file must not hand them to a teammate.
	names := slices.DeleteFunc(slices.Clone(cfg.Builtins), func(name string) bool { return name == config.ApprovalsBuiltin })
	layers, err := resolveBuiltinLayers(names, projectRoot)
	if err != nil {
		return nil, err
	}
	layers = append(layers, resolvePacksLayers(projectRoot)...)
	layers = append(layers, resolveProjectLayer(projectRoot, cfg))
	if l, ok := resolveProjectUserLayer(projectRoot); ok {
		layers = append(layers, l)
	}
	return layers, nil
}

// resolveProjectLayer returns the always-present project layer using
// the configurable source paths from cfg.
func resolveProjectLayer(projectRoot string, cfg *config.Config) spec.Layer {
	return spec.Layer{
		Name:        layerNameProject,
		Root:        projectRoot,
		Sources:     cfg.Sources,
		IncludeRoot: projectRoot,
	}
}

// resolveProjectUserLayer returns the local layer when
// `<projectRoot>/.agnostic-ai/local` exists.
func resolveProjectUserLayer(projectRoot string) (spec.Layer, bool) {
	pu := filepath.Join(projectRoot, defaultProjectUser)
	if !dirExists(pu) {
		return spec.Layer{}, false
	}
	return spec.Layer{
		Name:        layerNameProjectUser,
		Root:        pu,
		Sources:     defaultLayerSources(),
		Extends:     true,
		IncludeRoot: projectRoot,
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
