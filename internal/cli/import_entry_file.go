package cli

import (
	"io/fs"
	"os"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

// importAllSkippedEntryFiles is non-nil while `import all` runs, and
// holds the entry files it already skipped so each is noted once.
// Sequential use only, like importRunSources.
var importAllSkippedEntryFiles map[string]bool

// readEntryFile reads a project file an importer copies into the sources.
// Under `import all`, which runs on detection alone, a file that is not a
// regular file inside root reads as fs.ErrNotExist, noted once. A named
// import follows links anywhere.
//
// The project-local instructions block sync appends to entry points is
// dropped here, before any importer mirrors or slices the file, so the
// git-ignored `.agnostic-ai/local/AGNOSTIC_AI.md` text never lands in
// the committed sources.
//
// An entry point sync wrote from the current unfenced AGNOSTIC_AI.md
// alone reads as fs.ErrNotExist too: it holds nothing the sources lack.
// For a fenced source, mirrorMainFile keeps it and reports it unchanged.
func readEntryFile(root, path string) ([]byte, error) {
	data, err := readProjectEntryFile(root, path)
	if err != nil {
		return nil, err
	}
	if synced, fenced := syncedSharedBody(root, path, string(data)); synced && !fenced {
		return nil, fs.ErrNotExist
	}
	return data, nil
}

// readProjectEntryFile is readEntryFile without the synced-body check.
// It serves a reader of a generated block the shared body leaves out,
// such as the code review section, whose source may be gone even when
// the body is not.
func readProjectEntryFile(root, path string) ([]byte, error) {
	if importAllSkippedEntryFiles == nil || regularFileInside(root, path) {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return []byte(adapters.StripLocalInstructions(string(data))), nil
	}
	if _, err := os.Lstat(path); err == nil && !importAllSkippedEntryFiles[path] {
		importAllSkippedEntryFiles[path] = true
		summaryf("  ! skipped %s: not a file inside the project; import that tool by name to follow the link\n", path)
	}
	return nil, fs.ErrNotExist
}
