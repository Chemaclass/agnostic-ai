package cli

import (
	"os"
	"path/filepath"
)

func (g globalTarget) overrideRoot(home string) string {
	root := os.Getenv(g.rootEnv)
	if g.rootEnv != "KIRO_HOME" {
		return root
	}
	if !filepath.IsAbs(root) {
		return ""
	}
	root = filepath.Clean(root)
	if filepath.Dir(root) == root || withinDir(home, root) {
		return ""
	}
	return root
}
