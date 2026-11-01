package cli

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/gitexclude"
	"github.com/chemaclass/agnostic-ai/internal/gitquery"
	"github.com/chemaclass/agnostic-ai/internal/overview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// editAudience says which files an edit writes: the committed shared
// files, or the personal, git-ignored ones that overlay them.
type editAudience string

const (
	audienceShared   editAudience = "shared"
	audiencePersonal editAudience = "personal"
)

// legacyScopes are the names requests sent before audience. They stay
// accepted until the next minor release.
var legacyScopes = map[string]editAudience{"team": audienceShared, "local": audiencePersonal}

var (
	errEditAudience = errors.New(`audience must be "shared" or "personal"`)
	errEditScope    = errors.New(`scope must be "team" or "local"; send audience "shared" or "personal" instead`)
	errAudienceDiff = errors.New("audience and scope disagree; send audience alone")
)

type audienceFields struct {
	Audience string `json:"audience"`
	Scope    string `json:"scope"`
}

func (f audienceFields) sent() bool {
	return f.Audience != "" || f.Scope != ""
}

func (f audienceFields) audience() (editAudience, error) {
	audience := editAudience(f.Audience)
	if f.Audience != "" && audience != audienceShared && audience != audiencePersonal {
		return "", errEditAudience
	}
	if f.Scope == "" {
		if f.Audience == "" {
			return "", errEditAudience
		}
		return audience, nil
	}
	legacy, ok := legacyScopes[f.Scope]
	switch {
	case !ok:
		return "", errEditScope
	case f.Audience != "" && legacy != audience:
		return "", errAudienceDiff
	}
	return legacy, nil
}

func (a editAudience) legacyScope() string {
	for scope, audience := range legacyScopes {
		if audience == a {
			return scope
		}
	}
	return ""
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
func (h *editHandler) builtinsEdit(where editAudience, req builtinRequest) (string, func([]byte) ([]byte, error), error) {
	team, local := h.builtinsConfigFiles()
	localData, err := os.ReadFile(local)
	localMissing := os.IsNotExist(err)
	if err != nil && !localMissing {
		return "", nil, fmt.Errorf("%s: %w", local, err)
	}
	if where == audienceShared {
		if req.reset {
			return "", nil, fmt.Errorf("%s: reset applies to your personal list; send audience personal", overviewDisplay(h.root, local))
		}
		if personal, err := personalBuiltins(localData); err != nil || personal != nil {
			return "", nil, cmp.Or(err, fmt.Errorf("%s sets builtins for you; reset it before changing the shared list", overviewDisplay(h.root, local)))
		}
		return team, func(data []byte) ([]byte, error) { return setBuiltin(data, req.name, req.enabled) }, nil
	}
	if err := h.checkPersonalTarget(local); err != nil {
		return "", nil, err
	}
	teamList, err := h.teamBuiltins()
	if err != nil {
		return "", nil, err
	}
	inherited := ""
	if !h.global {
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
				seed := ""
				if localMissing {
					seed = inherited
				}
				if data, err = seedPersonalBuiltins(data, teamList, seed); err != nil {
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
		if !req.reset && !sameBuiltins(*personal, teamList) {
			return out, nil
		}
		if out, err = removeYAMLPath(out, "builtins"); err != nil {
			return nil, err
		}
		return personalConfigRest(out, inherited), nil
	}, nil
}

// configList is the list a config sets under key, merges included, or
// nil when it sets none. A null list counts as set and empty.
func configList(data []byte, key string) (*[]string, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	raw, set := doc[key]
	if !set {
		return nil, nil
	}
	list := []string{}
	if raw != nil {
		items, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("%s is not a list", key)
		}
		for _, item := range items {
			name, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s contains a non-string", key)
			}
			list = append(list, name)
		}
	}
	return &list, nil
}

// personalBuiltins is the builtins list a config sets, or nil when it
// sets none.
func personalBuiltins(data []byte) (*[]string, error) { return configList(data, "builtins") }

// seedPersonalBuiltins starts a personal list from the team list. A new
// personal config in a linked worktree also keeps the memory.personal it
// inherited from the main checkout, which any local config would end.
func seedPersonalBuiltins(data []byte, teamList []string, inherited string) ([]byte, error) {
	return seedPersonalList(data, "builtins", teamList, inherited)
}

func seedPersonalList(data []byte, key string, teamList []string, inherited string) ([]byte, error) {
	if len(bytes.TrimSpace(data)) == 0 && inherited != "" {
		data = []byte("memory:\n  personal: " + inherited + "\n")
	}
	_, mapping, err := yamlDocument(data)
	if err != nil {
		return nil, err
	}
	return editYAMLPair(data, mapping, key, slices.Clone(teamList))
}

// personalConfigRest is the personal config once its builtins list is
// gone, or nil when deleting the file keeps the same effective config.
// Any personal config stops a linked worktree inheriting memory.personal
// from the main checkout, so an empty one keeps the checkout mode in
// writing instead of being deleted.
func personalConfigRest(out []byte, inherited string) []byte {
	if len(bytes.TrimSpace(out)) == 0 {
		if inherited == "" || inherited == config.PersonalMemoryCheckout {
			return nil
		}
		return []byte("memory:\n  personal: " + config.PersonalMemoryCheckout + "\n")
	}
	var doc map[string]any
	if err := yaml.Unmarshal(out, &doc); err == nil && inherited != "" && reflect.DeepEqual(doc, map[string]any{"memory": map[string]any{"personal": inherited}}) {
		return nil
	}
	return out
}

func inheritedPersonalMemory(root string) (string, error) {
	if git, ok := config.ProjectGitFor(root); ok {
		return git.Personal, nil
	}
	git, _, err := config.ReadProjectGit(root)
	return git.Personal, err
}

// teamBuiltins is the committed builtins list; a global source without a
// config turns none on.
func (h *editHandler) teamBuiltins() ([]string, error) {
	if team, _ := h.builtinsConfigFiles(); h.global {
		if _, err := os.Stat(team); os.IsNotExist(err) {
			return nil, nil
		}
	}
	return config.TeamBuiltins(h.root)
}

// overviewBuiltins reports the team builtins list and whether the
// personal config replaces it, for the overview's local badges.
func overviewBuiltins(root string, global bool) ([]string, bool) {
	h := &editHandler{root: root, global: global}
	_, local := h.builtinsConfigFiles()
	list, err := h.teamBuiltins()
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
func (h *editHandler) scopedSettingsFile(e spec.Entry, key string, where editAudience) (string, string, error) {
	local := overview.LocalLayer(e)
	if where == audiencePersonal {
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
		return "", "", fmt.Errorf("%s is a personal spec with no shared value; switch to Personal to edit it", overview.SpecPath(h.root, e))
	}
	if overview.LocalSets(e, key) {
		return "", "", fieldSourceError{fmt.Errorf("%s sets %s for you; reset it to edit the shared value", overview.SpecPath(h.root, e), key)}
	}
	if e.Base.Layer == layerNameBuiltin || strings.HasPrefix(e.Base.Layer, "pack:") {
		return "", "", fmt.Errorf("the shared value comes from %s, which updates replace", overview.SpecPath(h.root, *e.Base))
	}
	return e.Base.Path, "", nil
}

// checkPersonalTarget refuses a personal write unless file resolves,
// through every link, inside the personal folder (or is the personal
// config itself), outside every team source, and is not tracked by Git.
func (h *editHandler) checkPersonalTarget(file string) error {
	localRoot, _ := h.personalRoot(file)
	team, err := h.teamSourcePaths()
	if err != nil {
		return err
	}
	target := resolvedAbs(file)
	refuse := func(why string) error {
		return fmt.Errorf("%s %s; personal files must stay in %s", overviewDisplay(h.root, file), why, overviewDisplay(h.root, localRoot))
	}
	if !withinDir(target, resolvedAbs(localRoot)) {
		return refuse("resolves outside the personal folder")
	}
	for _, path := range team {
		if withinDir(target, resolvedAbs(path)) {
			return refuse("resolves to the team source " + overviewDisplay(h.root, path))
		}
	}
	if gitTracks(target) {
		return refuse("is tracked by Git")
	}
	return nil
}

// personalRoot is the personal folder a personal write of file must stay
// in, or the personal config when file is that config.
func (h *editHandler) personalRoot(file string) (root string, folder bool) {
	if _, personalConfig := h.builtinsConfigFiles(); filepath.Clean(file) == filepath.Clean(personalConfig) && !h.global {
		return personalConfig, false
	}
	return filepath.Join(h.root, filepath.Dir(filepath.Dir(overview.SettingsOverlayPath(h.global, "x")))), true
}

// keepPersonalOutOfGit makes sure Git ignores file before a personal write
// creates it. When no rule ignores it yet, the personal folder, or the
// personal config, goes into the repository's info/exclude, which a
// linked worktree shares with its main checkout. Outside a work tree
// nothing can stage the file.
var personalGitMu sync.Mutex

func (h *editHandler) preparePersonalOutOfGit(file string) (func(bool) error, error) {
	personalGitMu.Lock()
	layout, layoutErr := gitquery.LayoutAtFresh(h.root)
	ex, ok := personalExcludeLayout(layout, layoutErr)
	if !ok {
		if err := h.keepPersonalOutOfGit(file); err != nil {
			personalGitMu.Unlock()
			return nil, err
		}
		return func(bool) error { personalGitMu.Unlock(); return nil }, nil
	}
	guard, err := gitexclude.Acquire(ex.path)
	if err != nil {
		personalGitMu.Unlock()
		return nil, err
	}
	unlock := func() { guard.Release(); personalGitMu.Unlock() }
	before, err := snapshotEditFile(ex.path)
	if err != nil {
		unlock()
		return nil, err
	}
	keepErr := h.keepPersonalOutOfGitLocked(file, guard)
	after, err := snapshotEditFile(ex.path)
	if err != nil {
		unlock()
		return nil, err
	}
	finish := func(rollback bool) error {
		defer unlock()
		if !rollback || (before.info == nil) == (after.info == nil) && bytes.Equal(before.data, after.data) {
			return nil
		}
		return fmt.Errorf("%s: kept the new Git exclusion to protect personal files", ex.path)
	}
	if keepErr != nil {
		if undoErr := finish(true); undoErr != nil {
			return nil, fmt.Errorf("%s; restore Git exclusion: %w", keepErr, undoErr)
		}
		return nil, keepErr
	}
	return finish, nil
}

func (h *editHandler) keepPersonalOutOfGit(file string) error {
	return h.keepPersonalOutOfGitLocked(file, nil)
}

func (h *editHandler) keepPersonalOutOfGitLocked(file string, guard *gitexclude.Guard) error {
	target := resolvedAbs(file)
	dir := existingAncestor(filepath.Dir(target))
	layout, err := gitquery.LayoutAtFresh(dir)
	if err != nil {
		if outsideGit(dir) {
			return nil
		}
		return fmt.Errorf("%s: could not check Git before saving this personal file: %w", overviewDisplay(h.root, file), err)
	}
	if !layout.HasToplevel || gitIgnores(dir, target) {
		return nil
	}
	display := overviewDisplay(h.root, file)
	personal, folder := h.personalRoot(file)
	rel, err := filepath.Rel(h.root, personal)
	rootLayout, layoutErr := gitquery.LayoutAtFresh(h.root)
	ex, ok := personalExcludeLayout(rootLayout, layoutErr)
	if err != nil || !ok {
		return fmt.Errorf("%s: Git would track this personal file; ignore %s in .gitignore", display, overviewDisplay(h.root, personal))
	}
	pattern := "/" + ex.prefix + filepath.ToSlash(rel)
	if folder {
		pattern += "/"
	}
	edit := func(content string) string {
		out, _ := adapters.WithGitExclusion([]byte(content), pattern)
		return string(out)
	}
	var updateErr error
	if guard != nil {
		updateErr = guard.Update(ex.path, edit)
	} else {
		updateErr = gitexclude.Update(ex.path, edit)
	}
	if err := updateErr; err != nil {
		return fmt.Errorf("%s: could not keep this personal file out of Git: %w", display, err)
	}
	if !gitIgnores(dir, target) {
		return fmt.Errorf("%s: Git would still track this personal file; ignore %s in .gitignore", display, overviewDisplay(h.root, personal))
	}
	return nil
}

func outsideGit(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--is-inside-work-tree")
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.HasPrefix(string(out), "fatal: not a git repository") {
		return false
	}
	for path := dir; ; path = filepath.Dir(path) {
		if _, err := os.Lstat(filepath.Join(path, ".git")); !os.IsNotExist(err) {
			return false
		}
		if filepath.Dir(path) == path {
			return true
		}
	}
}

// teamSourcePaths are the committed config files and spec folders an
// edit must never reach through a personal path.
func (h *editHandler) teamSourcePaths() ([]string, error) {
	sources, base := globalSources, h.root
	paths := []string{filepath.Join(h.root, config.ConfigFileName), filepath.Join(h.root, config.LegacyConfigFileName)}
	if !h.global {
		cfg, err := config.Load(h.root)
		if err != nil {
			return nil, err
		}
		sources = cfg.Sources
	}
	for _, dir := range []string{sources.Agents, sources.Skills, sources.Rules, sources.Hooks, sources.MCPs, sources.Commands, sources.Settings, sources.Reviews, sources.Environments, sources.Ignore} {
		if dir != "" {
			paths = append(paths, config.ResolveSourcePath(base, dir))
		}
	}
	return paths, nil
}

func resolvedAbs(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return resolveExisting(abs)
}

// gitTracks reports whether the Git repository around path tracks it, in
// any letter case: on a case-insensitive filesystem a deleted tracked file
// comes back under another case, which a plain pathspec misses.
func gitTracks(path string) bool {
	dir := filepath.Dir(path)
	for {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			break
		}
		if parent := filepath.Dir(dir); parent != dir {
			dir = parent
		} else {
			return false
		}
	}
	return exec.Command("git", "-C", dir, "ls-files", "--error-unmatch", "--", ":(icase,literal)"+path).Run() == nil
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
