package cli

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// headerProbeBytes bounds how much of a tracked file the ledger-less scan
// reads. The header sits on the first lines, below frontmatter at most.
const headerProbeBytes = 8 << 10

// ledgerMissing reports whether no `.sync-state` exists. An empty or
// unreadable ledger is still a record of what sync wrote, so only a
// missing one sends the leftover scan to git-tracked files (#1334).
func ledgerMissing(root string) bool {
	_, err := os.Lstat(stateFilePath(root))
	return errors.Is(err, fs.ErrNotExist)
}

// unledgeredCandidates lists the git-tracked files that sit where a
// configured target writes, for the leftover scan when no ledger names
// them. ok is false when git cannot list the tree.
func unledgeredCandidates(cfg *config.Config, emitted map[string]bool) (candidates []string, ok bool) {
	tracked, ok := trackedFiles(".")
	if !ok {
		return nil, false
	}
	loc := newOutputLocations(cfg, emitted)
	for _, p := range tracked {
		if loc.holds(filepath.ToSlash(p)) {
			candidates = append(candidates, p)
		}
	}
	return candidates, true
}

// ownedWithoutLedger reports whether path opens with the provenance header
// where the emitters write it. With no ledger there is no recorded sum, so
// this header is the only proof sync wrote the file. Only a bounded prefix
// is read.
func ownedWithoutLedger(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	head, err := io.ReadAll(io.LimitReader(f, headerProbeBytes))
	return err == nil && header.Leads(path, string(head))
}

// outputLocations are the places the configured targets write: tool
// directories and files, and the documents a scoped rule or review puts
// inside a project directory.
type outputLocations struct {
	dirs   []string
	files  map[string]bool
	scoped []string
	// nested caches whether a directory holds its own agnostic-ai project.
	nested map[string]bool
}

func newOutputLocations(cfg *config.Config, emitted map[string]bool) *outputLocations {
	l := &outputLocations{files: map[string]bool{}, nested: map[string]bool{}}
	for p := range emitted {
		// Every default tool dir is a dot-dir; a first segment without the
		// dot is a scope, a project directory holding the user's files.
		if top, _, nested := strings.Cut(filepath.ToSlash(p), "/"); nested && strings.HasPrefix(top, ".") {
			l.addDir(top)
		}
	}
	for _, t := range cfg.Targets {
		for _, a := range adapters.NativeArtifactsFor(t, cfg) {
			loc := strings.TrimPrefix(filepath.ToSlash(a.Location), "./")
			if strings.HasSuffix(loc, "/") {
				l.addDir(loc)
				continue
			}
			l.files[loc] = true
			l.addDir(path.Dir(loc))
		}
		l.scoped = append(l.scoped, adapters.ScopedDocuments(cfg, t)...)
	}
	dirs := configuredOutputDirs(cfg)
	for _, d := range append(dirs.roots, dirs.leaves...) {
		l.addDir(d)
	}
	return l
}

func (l *outputLocations) addDir(dir string) {
	dir = strings.Trim(dir, "/")
	if dir == "" || dir == "." || dir == defaultBaseDir {
		return
	}
	for _, d := range l.dirs {
		if d == dir {
			return
		}
	}
	l.dirs = append(l.dirs, dir)
}

// holds reports whether a configured target could have written p, a
// slash-separated path from the project root. Sources and fixture trees
// are never outputs: a Go `testdata` tree or a nested project's own tree
// copies real outputs byte for byte, so their paths are left out.
func (l *outputLocations) holds(p string) bool {
	if strings.HasPrefix(p, defaultBaseDir+"/") || p == config.ConfigFileName || p == config.LegacyConfigFileName {
		return false
	}
	for _, seg := range strings.Split(path.Dir(p), "/") {
		if seg == "testdata" {
			return false
		}
	}
	if l.inNestedProject(p) {
		return false
	}
	if !strings.Contains(p, "/") && strings.HasPrefix(p, ".") || l.files[p] {
		return true
	}
	for _, d := range l.dirs {
		if strings.HasPrefix(p, d+"/") {
			return true
		}
	}
	for _, doc := range l.scoped {
		if p == doc || strings.HasSuffix(p, "/"+doc) {
			return true
		}
	}
	return false
}

// inNestedProject reports whether a directory between the project root
// and p holds its own agnostic-ai config or source dir.
func (l *outputLocations) inNestedProject(p string) bool {
	for dir := path.Dir(p); dir != "." && dir != "/"; dir = path.Dir(dir) {
		nested, seen := l.nested[dir]
		if !seen {
			nested = fileExists(filepath.Join(filepath.FromSlash(dir), config.ConfigFileName)) ||
				fileExists(filepath.Join(filepath.FromSlash(dir), config.LegacyConfigFileName)) ||
				fileExists(filepath.Join(filepath.FromSlash(dir), defaultBaseDir))
			l.nested[dir] = nested
		}
		if nested {
			return true
		}
	}
	return false
}
