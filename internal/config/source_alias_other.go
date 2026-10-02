//go:build !windows

package config

import "path/filepath"

func ResolveSourceAlias(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}
