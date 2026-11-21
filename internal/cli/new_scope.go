package cli

import (
	"fmt"
	"os"
	"path/filepath"
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
	if err := refuseGlobalHome(".", globalHomeSpecsRemedy); err != nil {
		return newPlace{}, err
	}
	cfg, err := config.Load(".")
	if err != nil {
		return newPlace{}, err
	}
	shared, err := newSpecPath(cfg, kind, name)
	if err != nil {
		return newPlace{}, err
	}
	if scope != specScopeLocal {
		return newPlace{scope: scope, path: shared, cfg: cfg}, nil
	}
	path, err := newSpecPathIn(defaultProjectUser, defaultLayerSources(), kind, name)
	if err != nil {
		return newPlace{}, err
	}
	place := newPlace{scope: scope, path: path, cfg: cfg}
	if _, err := os.Stat(shared); err == nil {
		place.override = true
	}
	// A shared skill kept as a folder is overridden by the same folder.
	if kind == string(spec.KindSkill) {
		if _, err := os.Stat(filepath.Join(strings.TrimSuffix(shared, ".md"), "SKILL.md")); err == nil {
			place.override = true
			place.path = filepath.Join(strings.TrimSuffix(path, ".md"), "SKILL.md")
		}
	}
	return place, nil
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

// localIgnoreWarning says when Git would commit the personal layer, or
// returns "" when it is ignored or dir is not in a repository.
func localIgnoreWarning(dir string) string {
	if _, err := gitquery.Toplevel(dir); err != nil {
		return ""
	}
	var open []string
	for _, path := range []string{defaultProjectUser + "/", config.LocalOverrideFileName} {
		if !gitquery.Ignored(dir, path) {
			open = append(open, path)
		}
	}
	if len(open) == 0 {
		return ""
	}
	return fmt.Sprintf("Git does not ignore %s; add to .gitignore so personal specs stay out of commits", strings.Join(open, " or "))
}

// hasPersonalLayer reports whether the project at dir holds personal
// specs or a personal config.
func hasPersonalLayer(dir string) bool {
	for _, path := range []string{defaultProjectUser, config.LocalOverrideFileName} {
		if _, err := os.Stat(filepath.Join(dir, path)); err == nil {
			return true
		}
	}
	return false
}
