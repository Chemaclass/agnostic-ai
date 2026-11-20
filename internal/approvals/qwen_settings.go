package approvals

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/preview"
)

// QwenSettingsFiles lists the Qwen Code settings files whose
// permissions.ask rules may apply to a session in cwd: the system
// defaults and system files (or the paths their environment variables
// name), the user file in userDir, and .qwen/settings.json in cwd and
// every folder above it (docs/users/configuration/settings.md).
func QwenSettingsFiles(userDir, cwd, goos string, getenv func(string) string) []string {
	system := map[string]string{"linux": "/etc/qwen-code", "darwin": "/Library/Application Support/QwenCode"}[goos]
	var files []string
	for _, f := range []struct{ env, name string }{{"QWEN_CODE_SYSTEM_DEFAULTS_PATH", "system-defaults.json"}, {"QWEN_CODE_SYSTEM_SETTINGS_PATH", "settings.json"}} {
		if path := getenv(f.env); path != "" {
			files = append(files, path)
		} else if system != "" {
			files = append(files, filepath.Join(system, f.name))
		}
	}
	files = append(files, filepath.Join(userDir, "settings.json"))
	if filepath.IsAbs(cwd) {
		for dir := filepath.Clean(cwd); ; dir = filepath.Dir(dir) {
			files = append(files, filepath.Join(dir, ".qwen", "settings.json"))
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}
	return files
}

// SettingsHaveAskRules reports whether any of the JSON settings files,
// comments allowed, lists a permissions.ask rule. A hook allow may
// answer such a rule's prompt and the hook does not match the tool's
// own rule grammar, so any ask rule turns the hook off. A file that
// exists but does not read or parse is an error.
func SettingsHaveAskRules(files []string) (bool, error) {
	for _, path := range files {
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, fmt.Errorf("read %s: %w", path, err)
		}
		data, _ = preview.StripJSONC(data)
		var settings struct {
			Permissions struct {
				Ask []string `json:"ask"`
			} `json:"permissions"`
		}
		if err := json.Unmarshal(data, &settings); err != nil {
			return false, fmt.Errorf("parse %s: %w", path, err)
		}
		if len(settings.Permissions.Ask) > 0 {
			return true, nil
		}
	}
	return false, nil
}
