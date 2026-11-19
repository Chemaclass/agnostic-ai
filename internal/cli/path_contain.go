package cli

import (
	"path/filepath"
	"strings"
)

func pathBelow(base, path string) (string, bool) {
	rel, err := filepath.Rel(base, path)
	return rel, err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// pathUnder joins name onto dir, or reports false when name is not a
// local path that stays under dir, such as one holding "..".
func pathUnder(dir, name string) (string, bool) {
	if !filepath.IsLocal(name) {
		return "", false
	}
	return filepath.Join(dir, name), true
}

func realAbs(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}
