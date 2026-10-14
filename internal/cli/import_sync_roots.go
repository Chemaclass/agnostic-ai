package cli

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/codex"
)

// importPreviewRoots holds the roots of the real project while a dry run
// works in a copy, which has no .git to find them from.
var importPreviewRoots *adapters.GrantedRoots

// importSyncRoots returns the directories sync grants write access to in
// the project at root, which an import leaves out of the specs.
func importSyncRoots(root string) adapters.GrantedRoots {
	if importPreviewRoots != nil {
		return *importPreviewRoots
	}
	return adapters.SyncGrantedRoots(root)
}

// holdPreviewRoots finds the roots of the project at root before a dry run
// moves into its copy. The returned func lets them go.
func holdPreviewRoots(root string) func() {
	roots := adapters.SyncGrantedRoots(root)
	importPreviewRoots = &roots
	return func() { importPreviewRoots = nil }
}

// withoutGrantedRoots returns the Codex overlay data with the roots sync
// grants removed, so the guard that puts an overlay back when the tool
// files are unedited still lets an import clean it. Other files come back
// as they are.
func withoutGrantedRoots(path string, data []byte, granted adapters.GrantedRoots, root string) []byte {
	if filepath.Clean(path) != filepath.Clean(codexOverlayPath(root)) {
		return data
	}
	text, _, ok := codex.RemoveWritableRoots(string(data), granted.Has)
	if !ok {
		return data
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return []byte(text)
}
