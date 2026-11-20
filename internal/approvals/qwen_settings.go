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

// QwenSettingsFiles lists the Qwen Code settings files that may apply to
// a session in cwd, lowest precedence first: the system defaults file,
// the user file in userDir, .qwen/settings.json in every folder from the
// root down to cwd, and the system file. The two system files move to
// the paths their environment variables name
// (docs/users/configuration/settings.md).
func QwenSettingsFiles(userDir, cwd, goos string, getenv func(string) string) []string {
	system := map[string]string{"linux": "/etc/qwen-code", "darwin": "/Library/Application Support/QwenCode"}[goos]
	systemFile := func(env, name string) []string {
		if path := getenv(env); path != "" {
			return []string{path}
		}
		if system != "" {
			return []string{filepath.Join(system, name)}
		}
		return nil
	}
	files := systemFile("QWEN_CODE_SYSTEM_DEFAULTS_PATH", "system-defaults.json")
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
	return append(files, systemFile("QWEN_CODE_SYSTEM_SETTINGS_PATH", "settings.json")...)
}

// QwenSettings is what the hook needs from Qwen Code's settings files.
type QwenSettings struct {
	// Ask is set when any file lists a permissions.ask rule.
	Ask bool
	// HooksOff is set when any file sets disableAllHooks.
	HooksOff bool
	// ApprovalMode is tools.approvalMode from the last file that sets it.
	ApprovalMode string
}

// ReadQwenSettings reads files, comments allowed, in precedence order. A
// file that exists but does not read or parse is an error.
func ReadQwenSettings(files []string) (QwenSettings, error) {
	var out QwenSettings
	for _, path := range files {
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return QwenSettings{}, fmt.Errorf("read %s: %w", path, err)
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
		}
		if err := json.Unmarshal(data, &file); err != nil {
			return QwenSettings{}, fmt.Errorf("parse %s: %w", path, err)
		}
		out.Ask = out.Ask || len(file.Permissions.Ask) > 0
		out.HooksOff = out.HooksOff || file.DisableAllHooks
		if file.Tools.ApprovalMode != "" {
			out.ApprovalMode = file.Tools.ApprovalMode
		}
	}
	return out, nil
}
