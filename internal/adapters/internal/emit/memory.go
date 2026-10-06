package emit

import (
	"path/filepath"
	"strings"
)

// ProjectMemoryIndexPath is the project-relative index of the shared
// memory store the `memory` built-in maintains. It is fixed: tools read
// it at runtime, so it never follows `sources:` paths.
const ProjectMemoryIndexPath = ".agnostic-ai/memory/MEMORY.md"

// Sentinel markers delimiting the memory import block inside an
// entry-point file. Import strips the block (StripGeneratedAppendices)
// so it never flows back into AGNOSTIC_AI.md.
const (
	MemoryStartMarker = "<!-- agnostic-ai:memory:start -->"
	MemoryEndMarker   = "<!-- agnostic-ai:memory:end -->"
)

// RenderMemoryBlock returns the sentinel-marked block that imports the
// shared memory index into the entry-point file at entryPath. The import
// is relative to that file, as `@path` lines resolve.
func RenderMemoryBlock(entryPath string) string {
	ref, err := filepath.Rel(filepath.Dir(entryPath), ProjectMemoryIndexPath)
	if err != nil {
		ref = ProjectMemoryIndexPath
	}
	return MemoryStartMarker + "\n\n## Shared memory\n\n@" + filepath.ToSlash(ref) + "\n\n" + MemoryEndMarker + "\n"
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
