package spec

import "path/filepath"

// HookAssetDir keeps an inherited command tied to its bundled script source.
func (e Entry) HookAssetDir(command, target string) string {
	if override, ok := e.Meta["x-"+target].(map[string]any); ok {
		for _, key := range []string{"command", "commandWindows"} {
			if hookCommandContains(override[key], command) {
				return e.hookAssetDir("x-" + target + "." + key)
			}
		}
	}
	for _, key := range []string{"command", "commandWindows"} {
		if hookCommandContains(e.Meta[key], command) {
			return e.hookAssetDir(key)
		}
	}
	return ""
}

func (e Entry) hookAssetDir(field string) string {
	if dir, ok := e.HookAssetDirs[field]; ok {
		return dir
	}
	if e.Layer == "builtin" && e.Path != "" {
		return filepath.Join(filepath.Dir(filepath.Dir(e.Path)), "scripts")
	}
	return ""
}

func hookCommandContains(raw any, command string) bool {
	switch value := raw.(type) {
	case string:
		return value == command
	case []string:
		for _, item := range value {
			if item == command {
				return true
			}
		}
	case []any:
		for _, item := range value {
			if item == command {
				return true
			}
		}
	}
	return false
}

func mergeHookAssetDirs(base, over Entry, meta map[string]any) map[string]string {
	out := map[string]string{}
	for _, key := range []string{"command", "commandWindows"} {
		if _, exists := meta[key]; exists {
			from := base
			if _, exists := over.Meta[key]; exists {
				from = over
			}
			out[key] = from.hookAssetDir(key)
		}
	}
	for field, raw := range meta {
		values, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		overValues, _ := over.Meta[field].(map[string]any)
		for _, key := range []string{"command", "commandWindows"} {
			if _, exists := values[key]; !exists {
				continue
			}
			from := base
			if _, exists := overValues[key]; exists {
				from = over
			}
			out[field+"."+key] = from.hookAssetDir(field + "." + key)
		}
	}
	return out
}
