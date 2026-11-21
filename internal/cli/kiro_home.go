package cli

import (
	"os"
	"path/filepath"
)

func (g userTarget) overrideRoot(home string) string {
	root := os.Getenv(g.rootEnv)
	if g.rootEnv != "KIRO_HOME" {
		return root
	}
	if !filepath.IsAbs(root) {
		return ""
	}
	root = filepath.Clean(root)
	if _, inside := pathBelow(root, home); filepath.Dir(root) == root || inside {
		return ""
	}
	return root
}
