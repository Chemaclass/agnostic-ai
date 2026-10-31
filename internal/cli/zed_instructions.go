package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

func zedInstructionsRoot(home, goos string) (string, error) {
	switch goos {
	case "darwin":
		return filepath.Join(home, ".config", "zed"), nil
	case "linux", "freebsd":
		if root, set := os.LookupEnv("FLATPAK_XDG_CONFIG_HOME"); set {
			if !filepath.IsAbs(root) {
				return "", fmt.Errorf("FLATPAK_XDG_CONFIG_HOME=%q must be an absolute path", root)
			}
			return filepath.Join(root, "zed"), nil
		}
	}
	return "", nil
}
