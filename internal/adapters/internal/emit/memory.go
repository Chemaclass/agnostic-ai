package emit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// ProjectMemoryIndexPath is the project-relative index of the shared
// memory store the `memory` built-in maintains. It is fixed: tools read
// it at runtime, so it never follows `sources:` paths.
const ProjectMemoryIndexPath = ".agnostic-ai/memory/MEMORY.md"

// PersonalMemoryIndexPath is the index of the personal store, under the
// git-ignored project-user layer.
const PersonalMemoryIndexPath = ".agnostic-ai/local/memory/MEMORY.md"

// MemoryBuiltin names the built-in that owns the shared memory store.
const MemoryBuiltin = "memory"

// MemoryIndexPaths returns the memory indexes target loads when it
// lists its context files, personal first, or nil when the memory
// built-in is off.
func MemoryIndexPaths(cfg *config.Config, path, target string) ([]string, error) {
	if cfg == nil || !slices.Contains(cfg.Builtins, MemoryBuiltin) {
		return nil, nil
	}
	dir, err := PersonalMemoryDirFor(cfg, path, target)
	if err != nil {
		return nil, err
	}
	return []string{filepath.ToSlash(filepath.Join(dir, "MEMORY.md")), ProjectMemoryIndexPath}, nil
}

// PersonalMemoryDir returns the personal store of the project at root:
// .agnostic-ai/local/memory under root, or with memory.personal: repo,
// $AGNOSTIC_AI_HOME/local/memory/<repo-slug>, one store for every
// worktree of the repository.
func PersonalMemoryDir(cfg *config.Config, root string) (string, error) {
	if !cfg.RepoPersonalMemory() {
		return filepath.Join(root, filepath.Dir(filepath.FromSlash(PersonalMemoryIndexPath))), nil
	}
	out, err := exec.Command("git", "-C", root, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return "", fmt.Errorf("memory.personal: repo needs a Git repository at %s: %w", root, err)
	}
	stores, err := repoMemoryStores()
	if err != nil {
		return "", fmt.Errorf("memory.personal: repo: %w", err)
	}
	return filepath.Join(stores, RepoSlug(strings.TrimSpace(string(out)))), nil
}

// CreateRepoMemoryStore creates the repo store dir, private to the user,
// so a tool setting that names it never points at a missing folder. The
// checkout store, a dry run, and a capture write nothing.
func (s *Session) CreateRepoMemoryStore(cfg *config.Config, dir string, dryRun bool) error {
	if !cfg.RepoPersonalMemory() || !filepath.IsAbs(dir) || dryRun || s.IsCapturing() {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("%s: %w", dir, err)
	}
	return nil
}

// repoMemoryStores returns the folder that holds every repo store, with
// symlinks resolved: Codex refuses a writable root whose path holds one,
// as when ~/.agnostic-ai links to a clone elsewhere.
func repoMemoryStores() (string, error) {
	home, err := repoMemoryHome()
	if err != nil {
		return "", err
	}
	// A home that does not exist yet resolves through its existing
	// parent, so the path stays the same once sync creates it.
	if abs, err := filepath.Abs(home); err == nil {
		home = realPath(abs)
	}
	return filepath.Join(home, "local", "memory"), nil
}

func repoMemoryHome() (string, error) {
	if home := os.Getenv("AGNOSTIC_AI_HOME"); home != "" {
		return home, nil
	}
	user, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(user, ".agnostic-ai"), nil
}

// WithoutStalePersonalIndexes drops from the instructions list of the
// file at path each personal memory index that current does not name:
// the checkout index, one in a repo store, or one an earlier sync
// claimed there. It is the entry an earlier sync added before
// memory.personal, the gitignore setup, or the home folder changed.
func WithoutStalePersonalIndexes(path string, list, current []string) []string {
	// An earlier sync may have written the store through the symlink.
	var prefixes []string
	if stores, err := repoMemoryStores(); err == nil {
		prefixes = append(prefixes, filepath.ToSlash(stores)+"/")
	}
	if home, err := repoMemoryHome(); err == nil {
		prefixes = append(prefixes, filepath.ToSlash(filepath.Join(home, "local", "memory"))+"/")
	}
	inStores := func(entry string) bool {
		return slices.ContainsFunc(prefixes, func(prefix string) bool { return strings.HasPrefix(entry, prefix) })
	}
	// A store under a home folder sync no longer uses matches no prefix.
	claimed := PriorClaimedItems(path, []string{"instructions"})
	out := make([]string, 0, len(list))
	for _, entry := range list {
		index := strings.HasSuffix(entry, "/MEMORY.md")
		personal := entry == PersonalMemoryIndexPath ||
			index && (inStores(entry) || slices.Contains(claimed, ContentSum(entry)))
		if personal && !slices.Contains(current, entry) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// PersonalMemoryDirFor returns the personal store the project files of
// targets may name. The repo store is an absolute path, so it lands only
// in files the managed .gitignore block keeps out of Git; otherwise the
// files keep the checkout store, as a project without repo mode has.
func PersonalMemoryDirFor(cfg *config.Config, path string, targets ...string) (string, error) {
	if !PersonalMemoryLeavesCheckout(cfg, path, targets...) {
		return PersonalMemoryDir(nil, ".")
	}
	return PersonalMemoryDir(cfg, ".")
}

// PersonalMemoryLeavesCheckout reports whether the project files of
// targets name the repo store rather than the checkout one. Any
// gitignore.commit kind for a target keeps its files on the checkout
// store, since one of them may hold the path.
func PersonalMemoryLeavesCheckout(cfg *config.Config, path string, targets ...string) bool {
	return personalMemoryLeavesCheckout(cfg, path, nil, targets)
}

// PersonalMemoryLeavesCheckoutIn is PersonalMemoryLeavesCheckout for a
// file that only the given gitignore.commit kinds write, so a committed
// kind that never reaches it leaves the repo store in place.
func PersonalMemoryLeavesCheckoutIn(cfg *config.Config, path string, kinds []string, targets ...string) bool {
	return personalMemoryLeavesCheckout(cfg, path, kinds, targets)
}

func personalMemoryLeavesCheckout(cfg *config.Config, path string, kinds, targets []string) bool {
	if !cfg.RepoPersonalMemory() || !cfg.Gitignore.Enabled || filepath.IsAbs(path) {
		return false
	}
	if cfg.Gitignore.Path != "" && filepath.Clean(cfg.Gitignore.Path) != ".gitignore" {
		return false
	}
	root, err := os.Getwd()
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || cfg.Gitignore.AllowsPath(rel) {
		return false
	}
	for _, t := range targets {
		committed := cfg.Gitignore.CommitKinds(t)
		if kinds == nil && len(committed) > 0 {
			return false
		}
		for _, kind := range kinds {
			if slices.Contains(committed, kind) {
				return false
			}
		}
	}
	return true
}

// RepoSlug names the repository whose Git common dir is commonDir: its
// folder name plus a hash of the path, so every worktree of one
// repository gets one slug and two clones of one name get two.
func RepoSlug(commonDir string) string {
	dir := realPath(filepath.Clean(commonDir))
	name := filepath.Base(dir)
	if name == ".git" {
		name = filepath.Base(filepath.Dir(dir))
	}
	name = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '-'
	}, strings.TrimSuffix(name, ".git"))
	sum := sha256.Sum256([]byte(filepath.ToSlash(dir)))
	return strings.Trim(name, ".-") + "-" + hex.EncodeToString(sum[:4])
}

// Sentinel markers delimiting the memory import block inside an
// entry-point file. Import strips the block (StripGeneratedAppendices)
// so it never flows back into AGNOSTIC_AI.md.
const (
	MemoryStartMarker = "<!-- agnostic-ai:memory:start -->"
	MemoryEndMarker   = "<!-- agnostic-ai:memory:end -->"
)

// RenderMemoryBlock returns the sentinel-marked block that imports the
// project memory index into the entry-point file at
// entryPath, relative or absolute, with personalIndex. Sync runs from
// the project root. A project-relative import is relative to that file,
// as `@path` lines resolve, or absolute when no relative path exists
// (another Windows volume). An absolute personalIndex stays absolute.
func RenderMemoryBlock(entryPath, personalIndex string) string {
	var lines []string
	for _, index := range []string{ProjectMemoryIndexPath, personalIndex} {
		ref := index
		if !filepath.IsAbs(index) {
			ref = memoryIndexRef(entryPath, index)
		}
		// Claude Code ends an import path at the first unescaped space.
		lines = append(lines, "@"+strings.ReplaceAll(filepath.ToSlash(ref), " ", `\ `))
	}
	return MemoryStartMarker + "\n\n## Shared memory\n\n" + strings.Join(lines, "\n") + "\n\n" + MemoryEndMarker + "\n"
}

func memoryIndexRef(entryPath, indexPath string) string {
	index, err := filepath.Abs(indexPath)
	if err != nil {
		return indexPath
	}
	dir, err := filepath.Abs(filepath.Dir(entryPath))
	if err != nil {
		return index
	}
	if ref, err := filepath.Rel(realPath(dir), realPath(index)); err == nil {
		return ref
	}
	return index
}

// realPath resolves symlinks in the longest existing prefix of the
// absolute path p, so two spellings of one directory (macOS /var and
// /private/var) compare equal even before p itself exists.
func realPath(p string) string {
	rest := ""
	for {
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(resolved, rest)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return filepath.Join(p, rest)
		}
		rest = filepath.Join(filepath.Base(p), rest)
		p = parent
	}
}

// AppendMemoryBlock returns body with block appended after one blank
// line, replacing any earlier memory block. Returns body unchanged when
// block is empty.
func AppendMemoryBlock(body, block string) string {
	if block == "" {
		return body
	}
	body = strings.TrimRight(StripMemoryBlock(body), "\n")
	if body == "" {
		return block
	}
	return body + "\n\n" + block
}

// StripMemoryBlock removes the sentinel-marked memory block (markers
// included) from body. Returns body unchanged when no block is present.
func StripMemoryBlock(body string) string {
	return stripMarkedBlock(body, MemoryStartMarker, MemoryEndMarker)
}
