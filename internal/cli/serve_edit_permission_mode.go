package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/overview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// permissionModeEdit sets or clears permissions.default-mode in the
// settings spec the permission lists edit, so the mode and the rules stay
// in one file.
func (h *editHandler) permissionModeEdit(where editAudience, shown string, addr fieldAddress, raw json.RawMessage) (string, func([]byte) ([]byte, error), error) {
	key := addr.key
	if !h.global {
		return "", nil, errors.New("permissions.default-mode applies through global sync only; set it in global settings (agnostic-ai ui --global)")
	}
	if addr.target != "" {
		return "", nil, errors.New("the default permission mode has no per-tool value; send target all")
	}
	var mode *string
	if !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		var v string
		if json.Unmarshal(raw, &v) != nil || !slices.Contains(spec.PermissionModes, v) {
			return "", nil, fmt.Errorf("%s must be one of: %s", key, strings.Join(spec.PermissionModes, ", "))
		}
		mode = &v
	}
	file, scaffold, _, err := h.permissionSource(where, shown, key)
	if err != nil {
		return "", nil, err
	}
	if mode != nil {
		if err := h.checkPermissionModeShadow(file, key); err != nil {
			return "", nil, err
		}
	}
	return file, func(data []byte) ([]byte, error) {
		if len(data) == 0 {
			data = []byte(scaffold)
		}
		if mode == nil {
			if current, err := fieldValueText(data, addr); err != nil {
				return nil, err
			} else if current == nil {
				return nil, fmt.Errorf("%s: there is no %s to clear", overviewDisplay(h.root, file), key)
			}
			out, err := removeYAMLPath(data, key)
			if err != nil {
				return nil, err
			}
			return permissionEditRest(out, data, true), nil
		}
		return setPermissionValue(data, key, *mode)
	}, nil
}

// shadowedFieldError refuses a save that a later settings spec overrides,
// since sync would keep writing that spec's value.
type shadowedFieldError struct{ error }

// checkPermissionModeShadow refuses a mode save into file when a settings
// spec that loads later sets the mode for any tool that applies it, since
// global sync writes the last value for each tool.
func (h *editHandler) checkPermissionModeShadow(file, key string) error {
	scope, err := loadCheckScope(h.global)
	if err != nil {
		return err
	}
	settings := scope.bundle.Settings
	at := slices.IndexFunc(settings, func(e spec.Entry) bool {
		return resolvedAbs(e.Path) == resolvedAbs(file) ||
			e.Base != nil && resolvedAbs(e.Base.Path) == resolvedAbs(file) ||
			!overview.LocalLayer(e) && resolvedAbs(filepath.Join(h.root, overview.SettingsOverlayPath(h.global, e.Name))) == resolvedAbs(file)
	})
	if at < 0 {
		return nil
	}
	for _, target := range scope.targets {
		if globalTargets[target].settings.permissionMode == "" || !settings[at].EmitsTo(target) {
			continue
		}
		if j := overview.SettingsOverride(settings, at, key, []string{target}, h.global); j >= 0 {
			return shadowedFieldError{fmt.Errorf("%s sets %s for %s and loads after %s, so sync keeps its value; edit that spec", overview.SpecPath(h.root, settings[j]), key, target, overview.SpecPath(h.root, settings[at]))}
		}
	}
	return nil
}
