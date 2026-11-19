package cli

import (
	"path/filepath"
	"strings"
)

func pathBelow(base, path string) (string, bool) {
	rel, err := filepath.Rel(base, path)
	return rel, err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func realAbs(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}
