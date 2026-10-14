package cli

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// importPreviewRoots holds the roots of the real project while a dry run
// works in a copy, which has no .git to find them from.
var importPreviewRoots *adapters.GrantedRoots

// importSyncRoots returns the directories sync grants write access to in
// the project at root, which an import leaves out of the specs.
func importSyncRoots(cfg *config.Config, root string) adapters.GrantedRoots {
	if importPreviewRoots != nil {
		return *importPreviewRoots
	}
	return adapters.SyncGrantedRoots(cfg, root)
}

// holdPreviewRoots finds the roots of the project at root before a dry run
// moves into its copy. The returned func lets them go.
func holdPreviewRoots(root string) func() {
	cfg, err := config.Load(root)
	if err != nil {
		return func() {}
	}
	roots := adapters.SyncGrantedRoots(cfg, root)
	importPreviewRoots = &roots
	return func() { importPreviewRoots = nil }
}
