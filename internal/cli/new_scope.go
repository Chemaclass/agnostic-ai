package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/gitquery"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// newPlace is where new writes a spec at one scope.
type newPlace struct {
	scope string
	path  string
	// override is a local spec that edits a shared one of the same kind
	// and name. It starts with the name only, since local fields merge
	// into the shared ones.
	override bool
	// cfg is the project config; nil at user scope.
	cfg *config.Config
}

// resolveNewPlace returns the file new creates for kind and name at
// scope. A project or local spec needs a project; a user spec goes to
// the home sync --scope user reads, from any directory.
func resolveNewPlace(scope, kind, name string) (newPlace, error) {
	if scope == specScopeUser {
		source, err := globalSourceRoot()
		if err != nil {
			return newPlace{}, err
		}
		path, err := newSpecPathIn(source, globalSources, kind, name)
		if err != nil {
			return newPlace{}, err
		}
		if path == "" {
			return newPlace{}, fmt.Errorf("--scope %s has no %s specs; it takes: %s", specScopeUser, kind, strings.Join(userScopeKinds(), ", "))
		}
		return newPlace{scope: scope, path: path}, nil
	}
	if err := refuseGlobalHome(".", globalHomeNewRemedy); err != nil {
		return newPlace{}, err
	}
	if scope != specScopeLocal {
		cfg, err := config.Load(".")
		if err != nil {
			return newPlace{}, err
		}
		path, err := newSpecPath(cfg, kind, name)
		if err != nil {
			return newPlace{}, err
		}
		return newPlace{scope: scope, path: path, cfg: cfg}, nil
	}
	// A local spec merges into a lower one by kind and name, wherever
	// that file sits, so the loaded specs decide what this one changes.
	cfg, bundle, err := loadProject(".")
	if err != nil {
		return newPlace{}, err
	}
	path, err := newSpecPathIn(defaultProjectUser, defaultLayerSources(), kind, name)
	if err != nil {
		return newPlace{}, err
	}
	place := newPlace{scope: scope, path: path, cfg: cfg}
	for _, e := range bundle.All() {
		if string(e.Kind) != kind || e.Name != name {
			continue
		}
		if e.Layer == layerNameProjectUser {
			return newPlace{}, fmt.Errorf("%s already exists", filepath.ToSlash(e.Path))
		}
		place.override = true
		place.path = localPathOver(cfg, kind, path, e)
	}
	return place, nil
}

// localPathOver is where the local spec that changes base goes: the
// same place under the local folder as base has under its own, so a
// rule in a subfolder keeps its directory and a skill kept as a folder
// stays one. flat is the path for a spec outside the project's sources,
// such as a built-in.
func localPathOver(cfg *config.Config, kind, flat string, base spec.Entry) string {
	shared, err := sourceDirForKind(cfg, kind)
	local, localErr := sourceDirForKind(&config.Config{Sources: defaultLayerSources()}, kind)
	if err == nil && localErr == nil && shared != "" {
		if rel, err := filepath.Rel(shared, base.Path); err == nil && filepath.IsLocal(rel) {
			return filepath.Join(defaultProjectUser, local, rel)
		}
	}
	if filepath.Base(base.Path) == "SKILL.md" {
		return filepath.Join(strings.TrimSuffix(flat, filepath.Ext(flat)), "SKILL.md")
	}
	return flat
}

// newSpecPathIn is the file for kind and name under root, laid out as
// sources. It returns "" when sources has no folder for the kind.
func newSpecPathIn(root string, sources config.Sources, kind, name string) (string, error) {
	rel, err := newSpecPath(&config.Config{Sources: sources}, kind, name)
	if err != nil {
		return "", err
	}
	if filepath.Dir(rel) == "." {
		return "", nil
	}
	return filepath.Join(root, rel), nil
}

// userScopeKinds lists the kinds sync --scope user loads, in the order
// new names them.
func userScopeKinds() []string {
	var kinds []string
	for _, kind := range newSpecKinds {
		if path, err := newSpecPathIn("", globalSources, kind, "x"); err == nil && path != "" {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

// newOverrideTemplate is a local spec that changes a shared one: the
// name, and room for the fields to change.
func newOverrideTemplate(path, name string) string {
	const hint = "# Local override: write only the fields you change. The shared spec keeps the rest."
	if filepath.Ext(path) == ".md" {
		return fmt.Sprintf("---\n%s\nname: %s\n---\n", hint, name)
	}
	return fmt.Sprintf("%s\nname: %s\n", hint, name)
}

// localIgnoreWarning says when Git would commit one of the personal
// paths, or returns "" when each is ignored or dir is not in a
// repository. It checks only the paths given.
func localIgnoreWarning(dir string, paths ...string) string {
	if _, err := gitquery.Toplevel(dir); err != nil {
		return ""
	}
	var open []string
	for _, path := range paths {
		if !gitquery.Ignored(dir, path) {
			open = append(open, path)
		}
	}
	if len(open) == 0 {
		return ""
	}
	return fmt.Sprintf("Git would commit %s. Add it to .gitignore, or run `git rm --cached` on it if Git already tracks it", strings.Join(open, " and "))
}

// personalPaths lists the personal files the project at dir holds: its
// local specs folder and its local config.
func personalPaths(dir string) []string {
	var found []string
	for path, shown := range map[string]string{defaultProjectUser: defaultProjectUser + "/", config.LocalOverrideFileName: config.LocalOverrideFileName} {
		if _, err := os.Stat(filepath.Join(dir, path)); err == nil {
			found = append(found, shown)
		}
	}
	slices.Sort(found)
	return found
}
