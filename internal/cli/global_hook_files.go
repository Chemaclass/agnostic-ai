package cli

import (
	"io/fs"
	"maps"
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// addGlobalHookFiles places the hooks of a target that reads one file per
// hook from a user directory, plus the shared scripts they run. Each file
// is owned whole, like an agent file.
func addGlobalHookFiles(home, source, target string, g globalTarget, b spec.Bundle, onUnsupported string, add func(string, []byte, fs.FileMode) error) error {
	dir, scriptsDir := g.path(home, g.hookFiles), g.path(home, g.hookScripts)
	hooks := adapters.TargetHooks(target, b.HooksFor(target))
	adapters.NotePortableHookGaps(target, b.Hooks)
	if err := adapters.ReportHookProjectRoot(target, hooks, onUnsupported, true); err != nil {
		return err
	}
	files, scripts, err := adapters.UserHookFiles(target, hooks, filepath.Join(source, "scripts"), scriptsDir)
	if err != nil {
		return err
	}
	for _, script := range scripts {
		if err := add(script.Path, script.Body, script.Mode); err != nil {
			return err
		}
	}
	for _, name := range slices.Sorted(maps.Keys(files)) {
		if err := add(filepath.Join(dir, name), []byte(files[name]), 0o644); err != nil {
			return err
		}
	}
	return nil
}
