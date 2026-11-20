package cli

import (
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// landingLocation names where target writes the listed specs, for a kind
// its adapter declares no folder for: their shared folder, or each file.
// It renders the target again per spec, so it reads only the specs the
// list shows.
func landingLocation(cfg *config.Config, b spec.Bundle, target string, entries []spec.Entry) string {
	view := *cfg
	view.OnUnsupported = "silent"
	adapters.SetWarner(io.Discard)
	defer adapters.SetWarner(os.Stderr)
	defer adapters.SkipNotes()()

	var paths []string
	for _, e := range entries[:min(len(entries), 3)] {
		found, err := specContributions(target, e, b, bundleWithout(b, e), &view)
		if err != nil {
			return ""
		}
		for _, c := range found {
			paths = append(paths, filepath.ToSlash(c.Path))
		}
		if file, ok := entryPointRuleFile(&view, target, e, singleEntryBundle(e)); ok {
			paths = append(paths, filepath.ToSlash(file.Path))
		}
	}
	return summarizeLocations(paths)
}

func summarizeLocations(paths []string) string {
	paths = slices.Compact(slices.Sorted(slices.Values(paths)))
	if len(paths) < 2 {
		return strings.Join(paths, "")
	}
	for dir := path.Dir(paths[0]); dir != "." && dir != "/"; dir = path.Dir(dir) {
		if !slices.ContainsFunc(paths, func(p string) bool { return !strings.HasPrefix(p, dir+"/") }) {
			return dir + "/"
		}
	}
	return strings.Join(paths, ", ")
}
