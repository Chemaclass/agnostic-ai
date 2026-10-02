package config

import "path/filepath"

func ResolveSourcePath(root, source string) string {
	if filepath.IsAbs(source) {
		return filepath.Clean(source)
	}
	return filepath.Join(root, source)
}
