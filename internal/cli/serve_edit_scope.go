package cli

import (
	"bytes"
	"cmp"
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

// builtinRequest switches one built-in, or with reset drops the personal
// builtins list so the team list applies again.
type builtinRequest struct {
	name    string
	enabled bool
	reset   bool
}

// builtinsEdit applies req to the team or the personal builtins list. A
// local edit copies the team list into the personal config first, since
// that list replaces the team one whole, and drops it again once it
// matches the team list.
func (h *editHandler) builtinsEdit(scope editScope, req builtinRequest) (string, func([]byte) ([]byte, error), error) {
	team, local := h.builtinsConfigFiles()
	localData, err := os.ReadFile(local)
	if err != nil && !os.IsNotExist(err) {
		return "", nil, fmt.Errorf("%s: %w", local, err)
	}
	if scope == scopeTeam {
		if req.reset {
			return "", nil, fmt.Errorf("%s: reset applies to your personal list; send scope local", overviewDisplay(h.root, local))
		}
		if personal, err := personalBuiltins(localData); err != nil || personal != nil {
			return "", nil, cmp.Or(err, fmt.Errorf("%s sets builtins for you; reset it before changing the team list", overviewDisplay(h.root, local)))
		}
		return team, func(data []byte) ([]byte, error) { return setBuiltin(data, req.name, req.enabled) }, nil
	}
	teamList, err := readBuiltinsList(team)
	if err != nil {
		return "", nil, err
	}
	inherited := ""
	if len(localData) == 0 && !h.global {
		if inherited, err = inheritedPersonalMemory(h.root); err != nil {
			return "", nil, err
		}
	}
	display := overviewDisplay(h.root, local)
	return local, func(data []byte) ([]byte, error) {
		personal, err := personalBuiltins(data)
		if err != nil {
			return nil, err
		}
		if personal != nil && !yamlHasKey(data, "builtins") {
			return nil, fmt.Errorf("%s sets builtins through a YAML merge; edit that file by hand", display)
		}
		var out []byte
		switch {
		case req.reset && personal == nil:
			return nil, fmt.Errorf("%s: there is no personal builtins list to reset", display)
		case req.reset:
			out = data
		case personal == nil && slices.Contains(teamList, req.name) == req.enabled:
			return data, nil
		default:
			if personal == nil {
				if data, err = seedPersonalBuiltins(data, teamList, inherited); err != nil {
					return nil, err
				}
			}
			if out, err = setBuiltin(data, req.name, req.enabled); err != nil {
				return nil, err
			}
			if personal, err = personalBuiltins(out); err != nil {
				return nil, err
			}
		}
		if !req.reset && !sameNameSet(*personal, teamList) {
			return out, nil
		}
		if out, err = removeYAMLPath(out, "builtins"); err != nil {
			return nil, err
		}
		if onlyPersonalMemory(out, inherited) {
			return nil, nil
		}
		return out, nil
	}, nil
}

// personalBuiltins is the builtins list a config sets, merges included,
// or nil when it sets none.
func personalBuiltins(data []byte) (*[]string, error) {
	var parsed struct {
		Builtins *[]string `yaml:"builtins"`
	}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	if parsed.Builtins == nil && yamlHasKey(data, "builtins") {
		parsed.Builtins = &[]string{}
	}
	return parsed.Builtins, nil
}

// seedPersonalBuiltins starts a personal list from the team list. A new
// personal config in a linked worktree also keeps the memory.personal it
// inherited from the main checkout, which any local config would end.
func seedPersonalBuiltins(data []byte, teamList []string, inherited string) ([]byte, error) {
	if len(bytes.TrimSpace(data)) == 0 && inherited != "" {
		data = []byte("memory:\n  personal: " + inherited + "\n")
	}
	_, mapping, err := yamlDocument(data)
	if err != nil {
		return nil, err
	}
	return editYAMLPair(data, mapping, "builtins", slices.Clone(teamList))
}

// onlyPersonalMemory reports whether data sets nothing but the
// memory.personal the checkout inherits anyway, so the file can go.
func onlyPersonalMemory(data []byte, inherited string) bool {
	if len(bytes.TrimSpace(data)) == 0 {
		return true
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return false
	}
	return inherited != "" && reflect.DeepEqual(doc, map[string]any{"memory": map[string]any{"personal": inherited}})
}

func inheritedPersonalMemory(root string) (string, error) {
	if git, ok := config.ProjectGitFor(root); ok {
		return git.Personal, nil
	}
	git, _, err := config.ReadProjectGit(root)
	return git.Personal, err
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
	if err != nil {
		return list, false
	}
	personal, err := personalBuiltins(data)
	return list, err == nil && personal != nil
}

// scopedSettingsFile is the file an edit of key in the loaded settings
// spec e writes, and the text that file starts from when it is new. A team
// edit writes the shared spec behind a local overlay; a local edit writes
// the overlay, which is created next to the other personal specs.
func (h *editHandler) scopedSettingsFile(e spec.Entry, key string, scope editScope) (string, string, error) {
	local := overview.LocalLayer(e)
	if scope == scopeLocal {
		file, scaffold := e.Path, fieldCreatedSpec
		switch {
		case local && e.Base != nil:
			scaffold = settingsOverlayScaffold(e.Name)
		case !local:
			file, scaffold = filepath.Join(h.root, overview.SettingsOverlayPath(h.global, e.Name)), settingsOverlayScaffold(e.Name)
			if _, err := os.Lstat(file); err == nil {
				return "", "", fmt.Errorf("%s exists but does not overlay the settings spec named %q", overviewDisplay(h.root, file), e.Name)
			} else if !os.IsNotExist(err) {
				return "", "", fmt.Errorf("%s: %w", file, err)
			}
		}
		if err := h.checkPersonalTarget(file); err != nil {
			return "", "", err
		}
		return file, scaffold, nil
	}
	if !local {
		return e.Path, "", nil
	}
	if e.Base == nil {
		return "", "", fmt.Errorf("%s is a personal spec with no team value; switch to Just me to edit it", overview.SpecPath(h.root, e))
	}
	if overview.LocalSets(e, key) {
		return "", "", fmt.Errorf("%s sets %s for you; reset it to edit the team value", overview.SpecPath(h.root, e), key)
	}
	if e.Base.Layer == layerNameBuiltin || strings.HasPrefix(e.Base.Layer, "pack:") {
		return "", "", fmt.Errorf("the team value comes from %s, which updates replace", overview.SpecPath(h.root, *e.Base))
	}
	return e.Base.Path, "", nil
}

// checkPersonalTarget refuses a personal file whose folders resolve,
// through links, outside the personal folder, or a personal folder that
// holds the team sources, so a Just me save never rewrites a team file.
func (h *editHandler) checkPersonalTarget(file string) error {
	localDir := filepath.Join(h.root, filepath.Dir(filepath.Dir(overview.SettingsOverlayPath(h.global, "x"))))
	teamDir := h.root
	if !h.global {
		teamDir = filepath.Join(h.root, defaultBaseDir)
	}
	abs := func(path string) string {
		if out, err := filepath.Abs(path); err == nil {
			return resolveExisting(out)
		}
		return path
	}
	realLocal, realTeam, realDir := abs(localDir), abs(teamDir), abs(filepath.Dir(file))
	if !withinDir(realDir, realLocal) || withinDir(realTeam, realLocal) {
		return fmt.Errorf("%s resolves outside %s; personal files must stay in that folder", overviewDisplay(h.root, file), overviewDisplay(h.root, localDir))
	}
	return nil
}

func withinDir(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func settingsOverlayScaffold(name string) string {
	out, err := yaml.Marshal(map[string]string{"name": name})
	if err != nil {
		panic(err)
	}
	return fieldCreatedMarker + string(out)
}

// matchesShownSpec reports whether shown names e, by its own path or by
// the shared spec it extends.
func (h *editHandler) matchesShownSpec(e spec.Entry, shown string) bool {
	return overview.SpecPath(h.root, e) == shown || overview.SettingsTeamPath(h.root, e) == shown
}

// removeYAMLPath deletes a dotted key and its value through a byte splice,
// and the parent keys it leaves empty, then checks that nothing else
// changed.
func removeYAMLPath(data []byte, key string) ([]byte, error) {
	_, parent, err := yamlDocument(data)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(key, ".")
	for _, part := range parts[:len(parts)-1] {
		_, value := yamlMapValue(parent, part)
		if value == nil {
			return data, nil
		}
		if value.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%s is not safely removable", key)
		}
		parent = value
	}
	at, value := yamlMapValue(parent, parts[len(parts)-1])
	if value == nil {
		return data, nil
	}
	if len(parts) > 1 && len(parent.Content) == 2 {
		return removeYAMLPath(data, strings.Join(parts[:len(parts)-1], "."))
	}
	if parent.Style&yaml.FlowStyle != 0 || parent.Anchor != "" || value.Anchor != "" {
		return nil, fmt.Errorf("%s is not safely removable", key)
	}
	start, err := yamlNodeOffset(data, parent.Content[at])
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
	m := before
	for _, part := range parts[:len(parts)-1] {
		m, _ = m[part].(map[string]any)
	}
	delete(m, parts[len(parts)-1])
	if len(before) == 0 && len(after) == 0 {
		return out, nil
	}
	if !reflect.DeepEqual(before, after) {
		return nil, errors.New("the edit changes other values")
	}
	return out, nil
}
