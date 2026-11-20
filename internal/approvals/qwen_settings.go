package approvals

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/preview"
)

// QwenSettingsFiles lists the Qwen Code settings files in precedence order.
// Unless set explicitly, system defaults live beside the system settings file.
func QwenSettingsFiles(userDir, cwd, goos string, getenv func(string) string) ([]string, error) {
	const settingsEnv = "QWEN_CODE_SYSTEM_SETTINGS_PATH"
	const defaultsEnv = "QWEN_CODE_SYSTEM_DEFAULTS_PATH"
	settings, defaults := getenv(settingsEnv), getenv(defaultsEnv)
	if settings != "" && !filepath.IsAbs(settings) {
		return nil, fmt.Errorf("%s is not an absolute path", settingsEnv)
	}
	if defaults != "" && !filepath.IsAbs(defaults) {
		return nil, fmt.Errorf("%s is not an absolute path", defaultsEnv)
	}
	if settings == "" {
		settings = map[string]string{
			"linux":  "/etc/qwen-code/settings.json",
			"darwin": "/Library/Application Support/QwenCode/settings.json",
		}[goos]
	}
	if defaults == "" && settings != "" {
		defaults = filepath.Join(filepath.Dir(settings), "system-defaults.json")
	}
	var files []string
	if defaults != "" {
		files = append(files, defaults)
	}
	files = append(files, filepath.Join(userDir, "settings.json"))
	if filepath.IsAbs(cwd) {
		var project []string
		for dir := filepath.Clean(cwd); ; dir = filepath.Dir(dir) {
			project = append(project, filepath.Join(dir, ".qwen", "settings.json"))
			if filepath.Dir(dir) == dir {
				break
			}
		}
		slices.Reverse(project)
		files = append(files, project...)
	}
	if settings != "" {
		files = append(files, settings)
	}
	return files, nil
}

// JSONSettings is what the hook needs from a tool's JSON settings files.
type JSONSettings struct {
	// Ask is set when any file lists a permissions.ask rule.
	Ask bool
	// HooksOff is set when any file sets disableAllHooks.
	HooksOff bool
	// ApprovalMode is tools.approvalMode from the last file that sets it.
	ApprovalMode string
	// DefaultPermissionMode is general.defaultPermissionMode from the
	// last file that sets it.
	DefaultPermissionMode string
}

// ReadJSONSettings reads files, comments allowed, in precedence order. A
// file that exists but does not read or parse is an error.
func ReadJSONSettings(files []string) (JSONSettings, error) {
	var out JSONSettings
	for _, path := range files {
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return JSONSettings{}, fmt.Errorf("read %s: %w", path, err)
		}
		data, _ = preview.StripJSONC(data)
		var file struct {
			DisableAllHooks bool `json:"disableAllHooks"`
			Permissions     struct {
				Ask []string `json:"ask"`
			} `json:"permissions"`
			Tools struct {
				ApprovalMode string `json:"approvalMode"`
			} `json:"tools"`
			General struct {
				DefaultPermissionMode string `json:"defaultPermissionMode"`
			} `json:"general"`
		}
		if err := json.Unmarshal(data, &file); err != nil {
			return JSONSettings{}, fmt.Errorf("parse %s: %w", path, err)
		}
		out.Ask = out.Ask || len(file.Permissions.Ask) > 0
		out.HooksOff = out.HooksOff || file.DisableAllHooks
		if file.Tools.ApprovalMode != "" {
			out.ApprovalMode = file.Tools.ApprovalMode
		}
		if file.General.DefaultPermissionMode != "" {
			out.DefaultPermissionMode = file.General.DefaultPermissionMode
		}
	}
	return out, nil
}
