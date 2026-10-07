package emit

import (
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

// MemoryIndexPaths returns the memory indexes a target that lists its
// context files loads, personal first, or nil when the memory built-in
// is off.
func MemoryIndexPaths(cfg *config.Config) []string {
	if cfg == nil || !slices.Contains(cfg.Builtins, MemoryBuiltin) {
		return nil
	}
	return []string{PersonalMemoryIndexPath, ProjectMemoryIndexPath}
}

// Sentinel markers delimiting the memory import block inside an
// entry-point file. Import strips the block (StripGeneratedAppendices)
// so it never flows back into AGNOSTIC_AI.md.
const (
	MemoryStartMarker = "<!-- agnostic-ai:memory:start -->"
	MemoryEndMarker   = "<!-- agnostic-ai:memory:end -->"
)

// RenderMemoryBlock returns the sentinel-marked block that imports the
// project and personal memory indexes into the entry-point file at
// entryPath, relative or absolute. Sync runs from the project root. Each
// import is relative to that file, as `@path` lines resolve, or absolute
// when no relative path exists (another Windows volume).
func RenderMemoryBlock(entryPath string) string {
	return RenderMemoryImports(entryPath, []string{ProjectMemoryIndexPath, PersonalMemoryIndexPath})
}

// RenderMemoryImports is RenderMemoryBlock for the given indexes only.
func RenderMemoryImports(entryPath string, indexes []string) string {
	var lines []string
	for _, index := range indexes {
		// Claude Code ends an import path at the first unescaped space.
		lines = append(lines, "@"+strings.ReplaceAll(filepath.ToSlash(memoryIndexRef(entryPath, index)), " ", `\ `))
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
