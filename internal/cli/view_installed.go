package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// installedMu serializes installedState: rendering a target touches
// process-wide adapter state and may enter another directory.
var installedMu sync.Mutex

// installedState renders target in capture mode, as sync would write
// it, and compares each file with disk: a missing file is
// not-generated, a different one stale, an equal one in-sync. Overall,
// any stale file makes the target stale, no file on disk leaves it
// not-generated, and anything else is in-sync. root is the project root;
// the bundle's paths must read from it as sync reads them.
func installedState(target string, cfg *config.Config, b spec.Bundle, root string) (authority.InstalledState, error) {
	// Get, not Resolve: Resolve runs an agnostic-ai-<name> binary from
	// PATH, and any web page can trigger this route.
	adapter, ok := adapters.Get(target)
	if !ok {
		return authority.InstalledState{}, fmt.Errorf("%s is not a built-in target; external adapters are not run here", target)
	}
	installedMu.Lock()
	defer installedMu.Unlock()
	var state authority.InstalledState
	err := inDir(root, func() error {
		adapters.SetWarner(io.Discard)
		defer adapters.SetWarner(os.Stderr)
		defer adapters.SkipNotes()()
		defer holdStateFile(".")()
		captured, err := captureEmit(adapter, b, cfg)
		if err != nil {
			return fmt.Errorf("render %s: %w", target, err)
		}
		state, err = compareInstalled(captured)
		return err
	})
	if err != nil {
		return authority.InstalledState{}, err
	}
	state.CheckedAt = time.Now().UTC().Format(time.RFC3339)
	return state, nil
}

// compareInstalled compares captured files with disk in the working
// directory. A file captured twice counts with its last content.
func compareInstalled(captured []adapters.CapturedFile) (authority.InstalledState, error) {
	content := map[string]string{}
	var paths []string
	for _, f := range captured {
		p := filepath.ToSlash(f.Path)
		if _, seen := content[p]; !seen {
			paths = append(paths, p)
		}
		content[p] = f.Content
	}
	slices.Sort(paths)
	state := authority.InstalledState{Files: []authority.InstalledFile{}}
	present, missing, stale := 0, 0, false
	for _, p := range paths {
		file := authority.InstalledFile{Path: p, State: authority.InstalledInSync}
		disk, err := os.ReadFile(filepath.FromSlash(p))
		switch {
		case err != nil && notOnDisk(err):
			file.State = authority.InstalledNotGenerated
			missing++
		case err != nil:
			return authority.InstalledState{}, fmt.Errorf("read %s: %w", p, err)
		case string(disk) != content[p]:
			file.State = authority.InstalledStale
			stale = true
			present++
		default:
			present++
		}
		state.Files = append(state.Files, file)
	}
	// The whole is in sync only when every generated file is. A file
	// that sync would write but that is missing leaves the output
	// incomplete, so present files beside missing ones count as stale.
	switch {
	case stale, present > 0 && missing > 0:
		state.State = authority.InstalledStale
	case present == 0:
		state.State = authority.InstalledNotGenerated
	default:
		state.State = authority.InstalledInSync
	}
	return state, nil
}

// inDir runs fn with dir as the working directory.
func inDir(dir string, fn func() error) error {
	if dir == "" || filepath.Clean(dir) == "." {
		return fn()
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}
	if err := os.Chdir(dir); err != nil {
		return fmt.Errorf("enter %s: %w", dir, err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	return fn()
}
