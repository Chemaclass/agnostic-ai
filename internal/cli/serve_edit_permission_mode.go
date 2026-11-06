package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

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
