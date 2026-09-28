package cli

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// importTree names the directories below a project root that import
// walks leave out: the ones git ignores, and the ones holding their own
// `.git` entry (a nested clone, submodule, or worktree). Neither holds
// the project's own configuration, and a walk into an agent worktree
// imports every spec a second time. The root itself is never left out.
type importTree struct {
	root    string          // absolute
	ignored map[string]bool // root-relative slash paths git ignores
	// nested lists the repositories a preview copy dropped the `.git`
	// entry of, so the copy leaves out what the project leaves out.
	nested map[string]bool
}

// importTreeGitTimeout bounds the git call that lists ignored paths. It
// runs once per import, so it gets far more room than a sync status call.
const importTreeGitTimeout = 30 * time.Second

// importRunTree is the tree of the project the running import reads,
// loaded once so every walk shares one git call. Nil outside a run.
// Sequential use only, like importSandbox.
var importRunTree *importTree

// loadImportTree asks git which paths under root it ignores. Outside a
// repository, or without git, nothing counts as ignored and only nested
// `.git` entries are left out.
func loadImportTree(root string) importTree {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	tree := importTree{root: abs, ignored: map[string]bool{}}
	out, ok := runGitWithin(abs, importTreeGitTimeout,
		"ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--directory")
	if !ok {
		return tree
	}
	for p := range strings.SplitSeq(out, "\x00") {
		p = strings.TrimSuffix(p, "/")
		if p != "" && p != "." {
			tree.ignored[p] = true
		}
	}
	return tree
}

// withImportTree runs fn with root's tree loaded for every walk it makes,
// unless a caller (an import preview) already set one.
func withImportTree(root string, fn func() error) error {
	if importRunTree == nil {
		tree := loadImportTree(root)
		importRunTree = &tree
		defer func() { importRunTree = nil }()
	}
	return fn()
}

// importTreeFor returns the running import's tree when it covers root,
// and loads one otherwise.
func importTreeFor(root string) importTree {
	if importRunTree != nil {
		if abs, err := filepath.Abs(root); err == nil && abs == importRunTree.root {
			return *importRunTree
		}
	}
	return loadImportTree(root)
}

// skipsDir reports whether a walk should leave out the directory at rel,
// a root-relative slash path. A walk prunes top-down, so rel's parents
// are known to be kept.
func (t importTree) skipsDir(rel string) bool {
	if rel == "." || rel == "" {
		return false
	}
	if t.ignored[rel] || t.nested[rel] {
		return true
	}
	_, err := os.Lstat(filepath.Join(t.root, filepath.FromSlash(rel), ".git"))
	return err == nil
}

// onNativePath reports whether the directory rel is nativeDir, or a
// leading part of it, under any directory the walk kept: `.cursor` and
// `.cursor/skills` in `pkg/.cursor/skills`. A walk keeps these even when
// git ignores them, because sync writes there and a project that ignores
// its generated output still imports it.
func onNativePath(rel, nativeDir string) bool {
	parts := strings.Split(nativeDir, "/")
	for i := 1; i <= len(parts); i++ {
		lead := strings.Join(parts[:i], "/")
		if rel == lead || strings.HasSuffix(rel, "/"+lead) {
			return true
		}
	}
	return false
}
