package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (f userSettingsFile) platformPath(goos string) string {
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

func (f userSettingsFile) platformRootError(goos string) error {
	var name string
	switch path := f.platformPath(goos); {
	case strings.HasPrefix(path, userPathLocalAppData):
		name = "LOCALAPPDATA"
	case strings.HasPrefix(path, userPathXDG):
		name = "XDG_CONFIG_HOME"
	}
	names := f.absEnv
	if name != "" {
		names = append(names, name)
	}
	for _, name := range names {
		if root := os.Getenv(name); root != "" && !filepath.IsAbs(root) {
			return fmt.Errorf("%s=%q must be an absolute path", name, root)
		}
	}
	return nil
}
