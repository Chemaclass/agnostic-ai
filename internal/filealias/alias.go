package filealias

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Cache struct {
	info    map[string]os.FileInfo
	entries map[string][]os.DirEntry
}

func (c *Cache) Match(path string, info os.FileInfo, paths []string) (string, error) {
	if c.info == nil {
		c.info = make(map[string]os.FileInfo)
		c.entries = make(map[string][]os.DirEntry)
	}
	if info == nil {
		info = c.pathInfo(path)
	}
	if info == nil {
		return "", nil
	}
	for _, output := range paths {
		if !strings.EqualFold(filepath.Base(path), filepath.Base(output)) {
			continue
		}
		current := c.pathInfo(output)
		if current != nil && os.SameFile(info, current) {
			same, err := c.sameEntry(path, output)
			if err != nil {
				return "", err
			}
			if same {
				return output, nil
			}
		}
	}
	return "", nil
}

func (c *Cache) pathInfo(path string) os.FileInfo {
	info, ok := c.info[path]
	if !ok {
		// Compare links themselves so distinct links to one target stay distinct.
		info, _ = os.Lstat(path)
		c.info[path] = info
	}
	return info
}

func (c *Cache) sameEntry(path, output string) (bool, error) {
	dir, outputDir := filepath.Dir(path), filepath.Dir(output)
	parent, outputParent := c.pathInfo(dir), c.pathInfo(outputDir)
	if parent == nil || outputParent == nil || !os.SameFile(parent, outputParent) {
		return false, nil
	}
	name, outputName := filepath.Base(path), filepath.Base(output)
	if name == outputName {
		return true, nil
	}
	children, ok := c.entries[dir]
	if !ok {
		var err error
		children, err = os.ReadDir(dir)
		if err != nil {
			return false, fmt.Errorf("%s: %w", dir, err)
		}
		c.entries[dir] = children
	}
	var foundPath, foundOutput bool
	for _, child := range children {
		foundPath = foundPath || child.Name() == name
		foundOutput = foundOutput || child.Name() == outputName
	}
	// Two recorded spellings are separate directory entries even when hard-linked.
	return !foundPath || !foundOutput, nil
}
