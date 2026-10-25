package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/overview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// editScope says which files an edit writes: the committed team files,
// or the personal, git-ignored ones that overlay them.
type editScope string

const (
	scopeTeam  editScope = "team"
	scopeLocal editScope = "local"
)

var errEditScope = errors.New(`scope must be "team" or "local"`)

func parseEditScope(s string) (editScope, error) {
	if s := editScope(s); s == scopeTeam || s == scopeLocal {
		return s, nil
	}
	return "", errEditScope
}

// builtinsConfigFiles are the committed config and the personal one whose
// builtins list replaces it.
func (h *editHandler) builtinsConfigFiles() (team, local string) {
	if h.global {
		paths := globalConfigPaths(h.root)
		return paths[0], paths[1]
	}
	team = filepath.Join(h.root, config.ConfigFileName)
	if path, _, err := config.ResolveConfigPath(h.root); err == nil {
		team = path
	}
	return team, filepath.Join(h.root, config.LocalOverrideFileName)
}

// builtinsEdit turns one built-in on or off. A local edit copies the team
// list into the personal config first, since that list replaces the team
// one whole, and drops it again once it matches the team list.
func (h *editHandler) builtinsEdit(scope editScope, name string, enabled bool) (string, func([]byte) ([]byte, error), error) {
	team, local := h.builtinsConfigFiles()
	localData, err := os.ReadFile(local)
	if err != nil && !os.IsNotExist(err) {
		return "", nil, fmt.Errorf("%s: %w", local, err)
	}
	if scope == scopeTeam {
		if yamlHasKey(localData, "builtins") {
			return "", nil, fmt.Errorf("%s sets builtins for you; reset it before changing the team list", overviewDisplay(h.root, local))
		}
		return team, func(data []byte) ([]byte, error) { return setBuiltin(data, name, enabled) }, nil
	}
	teamList, err := readBuiltinsList(team)
	if err != nil {
		return "", nil, err
	}
	return local, func(data []byte) ([]byte, error) {
		if !yamlHasKey(data, "builtins") {
			if slices.Contains(teamList, name) == enabled {
				return data, nil
			}
			_, mapping, err := yamlDocument(data)
			if err != nil {
				return nil, err
			}
			if data, err = editYAMLPair(data, mapping, "builtins", append([]string{}, teamList...)); err != nil {
				return nil, err
			}
		}
		out, err := setBuiltin(data, name, enabled)
		if err != nil {
			return nil, err
		}
		var parsed struct {
			Builtins []string `yaml:"builtins"`
		}
		if err := yaml.Unmarshal(out, &parsed); err != nil {
			return nil, err
		}
		if sameNameSet(parsed.Builtins, teamList) {
			if out, err = removeYAMLPair(out, "builtins"); err != nil || len(bytes.TrimSpace(out)) == 0 {
				return nil, err
			}
		}
		return out, nil
	}, nil
}

func readBuiltinsList(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var parsed struct {
		Builtins []string `yaml:"builtins"`
	}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return parsed.Builtins, nil
}

func sameNameSet(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(slices.Compact(a), slices.Compact(b))
}

// overviewBuiltins reports the team builtins list and whether the
// personal config replaces it, for the overview's local badges.
func overviewBuiltins(root string, global bool) ([]string, bool) {
	h := &editHandler{root: root, global: global}
	team, local := h.builtinsConfigFiles()
	list, err := readBuiltinsList(team)
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(local)
	return list, err == nil && yamlHasKey(data, "builtins")
}

// scopedSettingsFile is the file an edit of key in the loaded settings
// spec e writes, and the text that file starts from when it is new. A team
// edit writes the shared spec behind a local overlay; a local edit writes
// the overlay, which is created next to the other personal specs.
func (h *editHandler) scopedSettingsFile(e spec.Entry, key string, scope editScope) (string, string, error) {
	local := overview.LocalLayer(e)
	if scope == scopeLocal {
		if local && e.Base != nil {
			return e.Path, settingsOverlayScaffold(e.Name), nil
		}
		if local {
			return e.Path, fieldCreatedSpec, nil
		}
		file := filepath.Join(h.root, overview.SettingsOverlayPath(h.global, e.Name))
		if _, err := os.Lstat(file); err == nil {
			return "", "", fmt.Errorf("%s exists but does not overlay the settings spec named %q", overviewDisplay(h.root, file), e.Name)
		} else if !os.IsNotExist(err) {
			return "", "", fmt.Errorf("%s: %w", file, err)
		}
		return file, settingsOverlayScaffold(e.Name), nil
	}
	if !local {
		return e.Path, "", nil
	}
	if e.Base == nil {
		return "", "", fmt.Errorf("%s is a personal spec with no team value; switch to Just me to edit it", overview.SpecPath(h.root, e))
	}
	if localSetsPath(e, key) {
		return "", "", fmt.Errorf("%s sets %s for you; reset it to edit the team value", overview.SpecPath(h.root, e), key)
	}
	if e.Base.Layer == layerNameBuiltin || strings.HasPrefix(e.Base.Layer, "pack:") {
		return "", "", fmt.Errorf("the team value comes from %s, which updates replace", overview.SpecPath(h.root, *e.Base))
	}
	return e.Base.Path, "", nil
}

// localSetsPath reports whether the personal file behind e sets the
// dotted key itself.
func localSetsPath(e spec.Entry, key string) bool {
	if !overview.LocalLayer(e) {
		return false
	}
	var current any = e.LocalMeta
	if e.Base == nil {
		current = e.Meta
	}
	for _, part := range strings.Split(key, ".") {
		m, ok := current.(map[string]any)
		if !ok {
			return false
		}
		if current, ok = m[part]; !ok {
			return false
		}
	}
	return true
}

func settingsOverlayScaffold(name string) string {
	return fieldCreatedMarker + "name: " + name + "\n"
}

// matchesShownSpec reports whether shown names e, by its own path or by
// the shared spec it extends.
func (h *editHandler) matchesShownSpec(e spec.Entry, shown string) bool {
	return overview.SpecPath(h.root, e) == shown || overview.SettingsTeamPath(h.root, e) == shown
}

// removeYAMLPair deletes a top-level key and its value through a byte
// splice, then checks that nothing else changed.
func removeYAMLPair(data []byte, key string) ([]byte, error) {
	_, mapping, err := yamlDocument(data)
	if err != nil {
		return nil, err
	}
	at, value := yamlMapValue(mapping, key)
	if value == nil {
		return data, nil
	}
	if mapping.Style&yaml.FlowStyle != 0 || value.Anchor != "" {
		return nil, fmt.Errorf("%s is not safely removable", key)
	}
	start, err := yamlNodeOffset(data, mapping.Content[at])
	if err != nil {
		return nil, err
	}
	lineStart := bytes.LastIndexByte(data[:start], '\n') + 1
	if len(bytes.TrimSpace(data[lineStart:start])) > 0 {
		return nil, fmt.Errorf("%s is not safely removable", key)
	}
	end, err := yamlCollectionEnd(data, value)
	if err != nil {
		return nil, fmt.Errorf("%s is not safely removable: %w", key, err)
	}
	for end < len(data) && data[end] != '\n' {
		end++
	}
	if end < len(data) {
		end++
	}
	out := spliceEditYAML(data, lineStart, end, "")
	var before, after map[string]any
	if err := yaml.Unmarshal(data, &before); err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(out, &after); err != nil {
		return nil, fmt.Errorf("the edit does not parse: %w", err)
	}
	delete(before, key)
	if len(before) == 0 && len(after) == 0 {
		return out, nil
	}
	if !reflect.DeepEqual(before, after) {
		return nil, errors.New("the edit changes other values")
	}
	return out, nil
}
