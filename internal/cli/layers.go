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
	layerNameUserGlobal  = "user-global"
	layerNameProject     = "project"
	layerNameProjectUser = "project-user"

	envUserGlobalRoot  = "AGNOSTIC_AI_HOME"
	defaultUserGlobal  = ".agnostic-ai"
	defaultProjectUser = ".agnostic-ai.local"
)

// defaultLayerSources is the fixed source layout used by user-global
// and project-user layers. The project layer keeps its configurable
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
	if l, ok := resolveUserGlobalLayer(); ok {
		layers = append(layers, l)
	}
	layers = append(layers, resolvePacksLayers(projectRoot)...)
	layers = append(layers, resolveProjectLayer(projectRoot, cfg))
	if l, ok := resolveProjectUserLayer(projectRoot); ok {
		layers = append(layers, l)
	}
	return layers
}

// resolveUserGlobalLayer returns the user-global layer when
// AGNOSTIC_AI_HOME or ~/.agnostic-ai resolves to an existing directory.
func resolveUserGlobalLayer() (spec.Layer, bool) {
	root, ok := userGlobalRoot()
	if !ok {
		return spec.Layer{}, false
	}
	return spec.Layer{
		Name:    layerNameUserGlobal,
		Root:    root,
		Sources: defaultLayerSources(),
	}, true
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

// userGlobalRoot resolves the user-global layer root. AGNOSTIC_AI_HOME
// wins if set and the dir exists; otherwise ~/.agnostic-ai when present.
// A set-but-unusable AGNOSTIC_AI_HOME is reported on stderr so typos
// don't silently disable the user-global layer.
func userGlobalRoot() (string, bool) {
	if env := os.Getenv(envUserGlobalRoot); env != "" {
		if dirExists(env) {
			return env, true
		}
		fmt.Fprintf(os.Stderr, "! %s=%s: not a directory; user-global layer skipped\n", envUserGlobalRoot, env)
		return "", false
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", false
	}
	root := filepath.Join(home, defaultUserGlobal)
	if dirExists(root) {
		return root, true
	}
	return "", false
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
