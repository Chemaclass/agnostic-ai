package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/overview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const permissionCreatedSpec = fieldCreatedMarker + "name: permissions\ndescription: Shared permission rules.\n"

func (h *editHandler) permissionSource(where editAudience, shown, key string) (string, string, []string, error) {
	scope, err := loadCheckScope(h.global)
	if err != nil {
		return "", "", nil, err
	}
	edit := overview.PermissionFieldEdit(scope.bundle.Settings, scope.cfg, h.root, h.global)
	if edit.DisabledReason != "" {
		return "", "", nil, fmt.Errorf("permissions: %s", edit.DisabledReason)
	}
	if shown == "" {
		if edit.Path == "" {
			return "", "", nil, fmt.Errorf("several editable settings specs exist; choose a path")
		}
		shown = edit.Path
	}
	var chosen spec.Entry
	for _, e := range scope.bundle.Settings {
		if overview.PermissionEditCandidate(e) && slices.Contains(edit.Paths, overview.SettingsTeamPath(h.root, e)) && h.matchesShownSpec(e, shown) {
			chosen = e
			break
		}
	}
	team, local := h.builtinsConfigFiles()
	paths := []string{team, local}
	for _, e := range scope.bundle.Settings {
		for current := &e; current != nil; current = current.Base {
			paths = append(paths, current.Path)
		}
	}
	if chosen.Path != "" {
		file, scaffold, err := h.scopedSettingsFile(chosen, key, where)
		if err != nil {
			return "", "", nil, err
		}
		paths = append(paths, filepath.Join(h.root, overview.SettingsOverlayPath(h.global, chosen.Name)))
		return file, scaffold, paths, nil
	}
	if len(scope.bundle.Settings) != 0 || shown != edit.Path {
		return "", "", nil, fmt.Errorf("%s is not an editable settings spec of this configuration", shown)
	}
	file := edit.Path
	if !filepath.IsAbs(file) {
		file = filepath.Join(h.root, filepath.FromSlash(file))
	}
	scaffold := permissionCreatedSpec
	if where == audiencePersonal {
		file = filepath.Join(h.root, overview.SettingsOverlayPath(h.global, "permissions"))
		scaffold = settingsOverlayScaffold("permissions")
		if err := h.checkPersonalTarget(file); err != nil {
			return "", "", nil, err
		}
	}
	if _, err := os.Lstat(file); err == nil {
		return "", "", nil, fmt.Errorf("%s exists but is not a loaded settings spec", overviewDisplay(h.root, file))
	} else if !os.IsNotExist(err) {
		return "", "", nil, fmt.Errorf("%s: %w", file, err)
	}
	paths = append(paths, filepath.Join(h.root, overview.SettingsOverlayPath(h.global, "permissions")))
	return file, scaffold, paths, nil
}

func permissionEditRest(data, original []byte, rules []string) []byte {
	if rules != nil || !strings.HasPrefix(string(original), fieldCreatedMarker) {
		return data
	}
	if string(data) == permissionCreatedSpec {
		return nil
	}
	var name string
	_, mapping, err := yamlDocument(data)
	if err == nil && len(mapping.Content) == 2 && mapping.Content[0].Value == "name" {
		name = mapping.Content[1].Value
	}
	if name != "" && string(data) == settingsOverlayScaffold(name) {
		return nil
	}
	return data
}
