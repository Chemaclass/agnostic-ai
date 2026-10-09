package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/chemaclass/agnostic-ai/internal/builtins"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const layerNameBuiltin = "builtin"

type builtinRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type specSourceRef struct {
	Kind    string      `json:"kind"`
	Name    string      `json:"name"`
	Path    string      `json:"path"`
	Layer   string      `json:"layer,omitempty"`
	Builtin *builtinRef `json:"builtin,omitempty"`
}

type builtinMaterialization struct {
	name    string
	root    string
	cleanup func() error
}

var builtinLayersMu sync.Mutex
var builtinMaterializations = map[string]builtinMaterialization{}

func validateBuiltinNames(names []string, source string) error {
	for _, name := range names {
		if !slices.Contains(builtins.Names(), name) {
			return errs.Coded(errs.CodeConfigDecode, "%s: builtins: unknown name %q; valid names: %s", source, name, strings.Join(builtins.Names(), ", "))
		}
	}
	return nil
}

func resolveBuiltinLayers(names []string, projectRoot string) ([]spec.Layer, error) {
	if err := validateBuiltinNames(names, "builtins"); err != nil {
		return nil, err
	}
	var layers []spec.Layer
	for _, name := range slices.Compact(slices.Sorted(slices.Values(names))) {
		root, err := materializeBuiltin(name)
		if err != nil {
			return nil, err
		}
		layers = append(layers, spec.Layer{Name: layerNameBuiltin, Root: root, Sources: defaultLayerSources(), IncludeRoot: projectRoot, RuleRoot: projectRoot})
	}
	return layers, nil
}

func materializeBuiltin(name string) (string, error) {
	builtinLayersMu.Lock()
	defer builtinLayersMu.Unlock()
	cacheDir, _ := os.UserCacheDir()
	key := cacheDir + "\x00" + name
	if m, ok := builtinMaterializations[key]; ok {
		if builtins.IsIntact(name, m.root) {
			return m.root, nil
		}
		if err := m.cleanup(); err != nil {
			return "", fmt.Errorf("replace builtin %s: %w", name, err)
		}
		delete(builtinMaterializations, key)
	}
	root, cleanup, err := builtins.Materialize(name)
	if err != nil {
		return "", fmt.Errorf("materialize builtin %s: %w", name, err)
	}
	builtinMaterializations[key] = builtinMaterialization{name: name, root: root, cleanup: cleanup}
	return root, nil
}

func cleanupBuiltinLayers() error {
	builtinLayersMu.Lock()
	defer builtinLayersMu.Unlock()
	var err error
	for _, m := range builtinMaterializations {
		err = errors.Join(err, m.cleanup())
	}
	builtinMaterializations = map[string]builtinMaterialization{}
	return err
}

func builtinVersion() string {
	if version, err := normalizeReleaseVersion(runningVersion); err == nil {
		return version
	}
	return "dev"
}

func builtinForEntry(e spec.Entry) *builtinRef {
	if e.Layer != layerNameBuiltin {
		return nil
	}
	builtinLayersMu.Lock()
	defer builtinLayersMu.Unlock()
	for _, m := range builtinMaterializations {
		if pathWithin(m.root, e.Path) {
			return &builtinRef{Name: m.name, Version: builtinVersion()}
		}
	}
	return &builtinRef{Name: e.Name, Version: builtinVersion()}
}

func (b builtinRef) String() string {
	version := b.Version
	if version != "dev" {
		version = "v" + version
	}
	return "builtin:" + b.Name + " (agnostic-ai " + version + ")"
}

func entrySourceRef(e spec.Entry) specSourceRef {
	r := specSourceRef{Kind: string(e.Kind), Name: e.Name, Path: filepath.ToSlash(e.Path), Layer: e.Layer, Builtin: builtinForEntry(e)}
	if r.Builtin != nil {
		r.Path = ""
	}
	return r
}

func entrySourceText(e spec.Entry) string {
	if b := builtinForEntry(e); b != nil {
		return b.String()
	}
	return filepath.ToSlash(e.Path)
}

func withoutBuiltinEntries(b spec.Bundle) spec.Bundle {
	for _, entries := range []*[]spec.Entry{
		&b.Agents, &b.Skills, &b.Rules, &b.Hooks, &b.MCPs, &b.Commands,
		&b.Settings, &b.Reviews, &b.Environments, &b.Ignores, &b.Shadowed,
	} {
		*entries = slices.DeleteFunc(slices.Clone(*entries), func(e spec.Entry) bool { return e.Layer == layerNameBuiltin })
	}
	return b
}

func withoutBuiltinLayers(layers []spec.Layer) []spec.Layer {
	return slices.DeleteFunc(slices.Clone(layers), func(l spec.Layer) bool { return l.Name == layerNameBuiltin })
}

func withoutBuiltinFindings(findings []lintFinding, b spec.Bundle) []lintFinding {
	paths := map[string]bool{}
	for _, e := range append(b.All(), b.Shadowed...) {
		if e.Layer == layerNameBuiltin {
			paths[filepath.Clean(e.Path)] = true
		}
	}
	return slices.DeleteFunc(findings, func(f lintFinding) bool { return paths[filepath.Clean(f.Path)] })
}
