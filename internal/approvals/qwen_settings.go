package approvals

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

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
	// MovedEnv is set when a file's env block sets a variable that
	// changes where the hook reads its policy.
	MovedEnv bool
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
			Env map[string]any `json:"env"`
		}
		if err := json.Unmarshal(data, &file); err != nil {
			return JSONSettings{}, fmt.Errorf("parse %s: %w", path, err)
		}
		out.Ask = out.Ask || len(file.Permissions.Ask) > 0
		out.HooksOff = out.HooksOff || file.DisableAllHooks
		for _, key := range policyEnv {
			if _, set := file.Env[key]; set {
				out.MovedEnv = true
			}
		}
		if file.Tools.ApprovalMode != "" {
			out.ApprovalMode = file.Tools.ApprovalMode
		}
		if file.General.DefaultPermissionMode != "" {
			out.DefaultPermissionMode = file.General.DefaultPermissionMode
		}
	}
	return out, nil
}

// QwenDotEnvMovesPolicy reports whether a .env or .qwen/.env file in cwd
// or a folder above it sets a variable that changes where the hook
// reads its policy. Qwen Code loads those files into its own
// environment, which its hooks inherit, so a repository could point the
// hook at a policy of its own. A file that exists but does not read is
// an error.
func QwenDotEnvMovesPolicy(cwd string) (bool, error) {
	if !filepath.IsAbs(cwd) {
		return false, fmt.Errorf("%s is not an absolute path", cwd)
	}
	for dir := filepath.Clean(cwd); ; dir = filepath.Dir(dir) {
		for _, file := range []string{filepath.Join(dir, ".env"), filepath.Join(dir, ".qwen", ".env")} {
			data, err := os.ReadFile(file)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return false, fmt.Errorf("read %s: %w", file, err)
			}
			if dotEnvSetsPolicy(string(data)) {
				return true, nil
			}
		}
		if filepath.Dir(dir) == dir {
			return false, nil
		}
	}
}

// dotEnvLine is the line grammar of the dotenv package Qwen Code
// loads .env files with: a key, then = or a colon and a blank.
// Its \s is JavaScript's, which also takes the vertical tab and every
// Unicode space.
var dotEnvLine = regexp.MustCompile(`(?m)^[\s\v\p{Z}\x{FEFF}]*(?:export[\s\v\p{Z}\x{FEFF}]+)?([\w.-]+)(?:[\s\v\p{Z}\x{FEFF}]*=|:[\s\v\p{Z}\x{FEFF}])`)

func dotEnvSetsPolicy(text string) bool {
	text = strings.NewReplacer("\r\n", "\n", "\r", "\n", "\u2028", "\n", "\u2029", "\n").Replace(text)
	for _, m := range dotEnvLine.FindAllStringSubmatch(text, -1) {
		if slices.ContainsFunc(policyEnv, func(key string) bool { return strings.EqualFold(key, m[1]) }) {
			return true
		}
	}
	return false
}
