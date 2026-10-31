package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (f globalSettingsFile) platformPath(goos string) string {
	switch goos {
	case "linux":
		if f.linuxPath != "" {
			return f.linuxPath
		}
	case "windows":
		if f.windowsPath != "" {
			return f.windowsPath
		}
	}
	return f.path
}

func (f globalSettingsFile) platformRootError(goos string) error {
	var name string
	switch path := f.platformPath(goos); {
	case strings.HasPrefix(path, globalPathLocalAppData):
		name = "LOCALAPPDATA"
	case strings.HasPrefix(path, globalPathXDG):
		name = "XDG_CONFIG_HOME"
	}
	if name != "" {
		if root := os.Getenv(name); root != "" && !filepath.IsAbs(root) {
			return fmt.Errorf("%s=%q must be an absolute path", name, root)
		}
	}
	return nil
}
