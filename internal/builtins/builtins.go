package builtins

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

//go:embed data
var sourceFS embed.FS

var names = []string{"handoff"}

type builtinFile struct {
	path string
	body []byte
}

func Names() []string {
	return slices.Clone(names)
}

func Hash(name string) string {
	files, err := load(name)
	if err != nil {
		return ""
	}
	return contentHash(files)
}

func load(name string) ([]builtinFile, error) {
	if !slices.Contains(names, name) {
		return nil, fmt.Errorf("load builtin %q: unknown name (valid names: %s)", name, strings.Join(names, ", "))
	}
	root := "data/" + name
	var files []builtinFile
	err := fs.WalkDir(sourceFS, root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if entry.IsDir() {
			return nil
		}
		body, err := sourceFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		files = append(files, builtinFile{path: strings.TrimPrefix(path, root+"/"), body: body})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load builtin %q: %w", name, err)
	}
	return files, nil
}

func contentHash(files []builtinFile) string {
	hash := sha256.New()
	for _, file := range files {
		hash.Write([]byte(file.path))
		hash.Write([]byte{0})
		sum := sha256.Sum256(file.body)
		hash.Write(sum[:])
	}
	return hex.EncodeToString(hash.Sum(nil))
}
