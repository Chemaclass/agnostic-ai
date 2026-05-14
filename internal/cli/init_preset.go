package cli

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// presetFS holds stack-flavored starter spec packs. `init --preset go`
// (or `--preset ts-react`, `--preset python`) seeds the project with the
// preset's specs, in addition to whatever `--demo` would write. Each
// preset lives at `initdata/presets/<name>/<kind>/...`.
//
//go:embed all:initdata/presets
var presetFS embed.FS

const presetRoot = "initdata/presets"

// availablePresets returns the sorted list of preset names embedded in
// the binary. Drives both tab completion and the unknown-preset error
// message.
func availablePresets() []string {
	entries, err := presetFS.ReadDir(presetRoot)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

func validatePresetName(name string) error {
	for _, p := range availablePresets() {
		if p == name {
			return nil
		}
	}
	return fmt.Errorf("unknown preset %q. Available: %s", name, strings.Join(availablePresets(), ", "))
}

// writePresetFiles mirrors every file under initdata/presets/<name>
// into baseDir, preserving the kind subfolder. Existing files are left
// untouched, so layering --preset on top of --demo or an existing tree
// never clobbers user content.
func writePresetFiles(baseDir, name string) error {
	root := presetRoot + "/" + name
	return fs.WalkDir(presetFS, root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(baseDir, filepath.FromSlash(rel))
		if _, err := os.Stat(dst); err == nil {
			return nil
		}
		data, err := presetFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", dst, err)
		}
		return nil
	})
}
