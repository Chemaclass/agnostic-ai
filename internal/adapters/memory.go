package adapters

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// RenderMemoryBlock returns the block importing the shared memory index
// into the entry-point file at entryPath (re-exported from the emit
// layer).
func RenderMemoryBlock(entryPath, personalIndex string) string {
	return emit.RenderMemoryBlock(entryPath, personalIndex)
}

// MemoryBlockImportsPersonal reports whether the memory block in body
// imports a personal index (re-exported from the emit layer).
func MemoryBlockImportsPersonal(body string) bool {
	return emit.MemoryBlockImportsPersonal(body)
}

// PersonalMemoryDir returns the personal store of the project at root
// (re-exported from the emit layer).
func PersonalMemoryDir(cfg *config.Config, root string) (string, error) {
	return emit.PersonalMemoryDir(cfg, root)
}

// GrantedRoots names the directories sync grants write access to outside
// the checkout: stores of personal memory and the shared handoff store.
// Each is a path on one machine, so an import leaves it out of the specs.
type GrantedRoots struct {
	// Dirs are the stores of this project, as sync writes them.
	Dirs []string
	// Stores are the folders that hold every project's repo memory store.
	Stores []string
}

// SyncGrantedRoots returns the roots of the project at root. It names the
// repo memory store whatever memory.personal says now, since sync may have
// written it before the mode changed.
func SyncGrantedRoots(root string) GrantedRoots {
	var granted GrantedRoots
	add := func(list *[]string, dir string) {
		*list = append(*list, filepath.ToSlash(dir))
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			*list = append(*list, filepath.ToSlash(resolved))
		}
	}
	repo := &config.Config{Memory: config.MemoryConfig{Personal: config.PersonalMemoryRepo}}
	if dir, err := emit.PersonalMemoryDir(repo, root); err == nil {
		add(&granted.Dirs, dir)
	}
	if stores, err := emit.RepoMemoryStores(); err == nil {
		add(&granted.Stores, stores)
	}
	if dir := emit.HandoffStoreOf(root); dir != "" {
		add(&granted.Dirs, dir)
	}
	return granted
}

// Has reports whether dir is a root sync grants: a store of this
// project, the shared handoff store of this project, or any repo memory store.
func (g GrantedRoots) Has(dir string) bool {
	dir = filepath.ToSlash(filepath.Clean(dir))
	if slices.Contains(g.Dirs, dir) {
		return true
	}
	for _, stores := range g.Stores {
		if strings.HasPrefix(dir, stores+"/") {
			return true
		}
	}
	return false
}

type nativePersonalMemory interface {
	NativePersonalMemoryDir(cfg *config.Config, root string) (string, error)
}

func NativePersonalMemoryDirFor(name string, cfg *config.Config, root string) (string, error) {
	adapter, ok := registry[name].(nativePersonalMemory)
	if !ok {
		return "", nil
	}
	return adapter.NativePersonalMemoryDir(cfg, root)
}

// PersonalMemoryDirFor returns the personal store the project files of
// targets may name (re-exported from the emit layer).
func PersonalMemoryDirFor(cfg *config.Config, path string, targets ...string) (string, error) {
	return emit.PersonalMemoryDirFor(cfg, path, targets...)
}

// RepoMemoryFiler is an adapter whose project files can hold the repo
// store of personal memory, an absolute path on one machine.
type RepoMemoryFiler interface {
	// RepoMemoryFiles names those files, from the shared config alone.
	RepoMemoryFiles(cfg *config.Config) []string
}

// RepoMemoryFiles returns the files of target that can hold the repo
// store of personal memory, which the managed block always ignores.
func RepoMemoryFiles(cfg *config.Config, target string) []string {
	adapter, err := Resolve(target)
	if err != nil {
		return nil
	}
	filer, ok := adapter.(RepoMemoryFiler)
	if !ok {
		return nil
	}
	return filer.RepoMemoryFiles(cfg)
}

// NativeMemoryStorer is an adapter whose tool keeps its own memory store
// on this machine, as a MEMORY.md index beside one file per fact.
type NativeMemoryStorer interface {
	// NativeMemoryDir returns the store the tool uses for the project at
	// root when no setting moves it.
	NativeMemoryDir(root string) (string, error)
}

// NativeMemoryDir returns the native memory store of target for the
// project at root, and false when the target keeps none.
func NativeMemoryDir(target, root string) (string, bool, error) {
	adapter, err := Resolve(target)
	if err != nil {
		return "", false, nil
	}
	storer, ok := adapter.(NativeMemoryStorer)
	if !ok {
		return "", false, nil
	}
	dir, err := storer.NativeMemoryDir(root)
	return dir, true, err
}

// MemorySeed is a copy of a tool's earlier facts into the empty personal
// store, queued by an adapter (re-exported from the emit layer).
type MemorySeed = emit.MemorySeed

func RequestMemorySeed(seed MemorySeed) {
	emit.RequestMemorySeed(seed)
}

// TakeMemorySeeds returns the seeds adapters queued and clears the queue.
func TakeMemorySeeds() []MemorySeed { return emit.TakeMemorySeeds() }

// MemoryFactFiles returns the fact files in a memory store, by name
// (re-exported from the emit layer).
func MemoryFactFiles(dir string) ([]string, error) { return emit.MemoryFactFiles(dir) }

// ExcludeFromGit adds pattern to the repository's info/exclude
// (re-exported from the emit layer).
func ExcludeFromGit(pattern string) error { return emit.ExcludeFromGit(pattern) }

// GitExcludePath finds the info/exclude file ExcludeFromGit writes
// (re-exported from the emit layer).
func GitExcludePath() (string, bool) { return emit.GitExcludePath() }

// WithGitExclusion returns info/exclude content holding pattern
// (re-exported from the emit layer).
func WithGitExclusion(data []byte, pattern string) ([]byte, bool) {
	return emit.WithGitExclusion(data, pattern)
}

// AppendMemoryBlock appends the memory block to body, replacing any
// earlier one (re-exported from the emit layer).
func AppendMemoryBlock(body, block string) string {
	return emit.AppendMemoryBlock(body, block)
}

// ModelTiersLine lists the tiers that carry a description, "" when none
// does (re-exported from the emit layer).
func ModelTiersLine(tiers map[string]config.ModelTier) string {
	return emit.ModelTiersLine(tiers)
}

// RenderModelTiers renders the marked model tiers block (re-exported from
// the emit layer).
func RenderModelTiers(tiers map[string]config.ModelTier) string {
	return emit.RenderModelTiers(tiers)
}

// StripModelTiers removes the marked model tiers block from body
// (re-exported from the emit layer).
func StripModelTiers(body string) string {
	return emit.StripModelTiers(body)
}

// AppendModelTiers appends the model tiers block to body, replacing any
// earlier one (re-exported from the emit layer).
func AppendModelTiers(body, block string) string {
	return emit.AppendModelTiers(body, block)
}

// ProjectMemoryIndexPath is the project-relative index of the shared
// memory store (re-exported from the emit layer).
const ProjectMemoryIndexPath = emit.ProjectMemoryIndexPath

// PersonalMemoryIndexPath is the project-relative index of the personal
// memory store (re-exported from the emit layer).
const PersonalMemoryIndexPath = emit.PersonalMemoryIndexPath
