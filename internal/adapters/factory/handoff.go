package factory

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// localSettingsFile is the one project file Droid reads for machine-local
// overrides (docs.factory.com/droid-cli/settings), so it can hold the
// absolute path of the main checkout's handoff store.
const localSettingsFile = ".factory/settings.local.json"

// allowWritePath is where the sandbox lists writable paths beyond the
// working folder (docs.factory.com/autonomy-and-safety/sandbox).
var allowWritePath = []string{"sandbox", "filesystem", "allowWrite"}

// RepoMemoryFiles names the local settings file while it can hold the
// handoff store, so the managed ignore block keeps it out of Git.
func (Adapter) RepoMemoryFiles(cfg *config.Config) []string {
	if !emit.HandoffConfigFile(cfg, localSettingsFile, target) {
		return nil
	}
	return []string{localSettingsFile}
}

// emitHandoffAccess lists the main checkout's handoff store as writable,
// so the sandbox lets a linked worktree save shared handoffs. It only
// adds a path: whether the sandbox is on stays the user's choice.
func emitHandoffAccess(sess *emit.Session, cfg *config.Config, dryRun bool) error {
	path := localSettingsFile
	if preserved, err := sess.PreserveHandoffConfig(cfg, path, target); preserved || err != nil {
		return err
	}
	handoff, err := sess.HandoffAccessRoot(cfg, path, target, "sandbox.filesystem.allowWrite", dryRun)
	if err != nil || sess.IsUnmanaged(path) {
		return err
	}
	if handoff == "" && len(emit.PriorClaimedItems(path, allowWritePath)) == 0 {
		return nil
	}
	filesystem, _ := sess.ExistingJSONObject(path, "sandbox", dryRun)["filesystem"].(map[string]any)
	existing, _ := filesystem["allowWrite"].([]any)
	var planned []any
	if handoff != "" {
		planned = emit.WithoutUserItems(path, allowWritePath, existing, []any{handoff})
	}
	merged, claims := emit.MergeOwnedLists(path, allowWritePath[:2], map[string][]any{"allowWrite": existing}, map[string][]any{"allowWrite": planned})
	list := merged["allowWrite"]
	value := emit.CarriedJSONValue(list)
	if len(claims["allowWrite"]) > 0 {
		value = emit.ClaimedJSONItems(list, claims["allowWrite"])
	} else if len(list) == 0 {
		value = emit.RemoveJSONKey
	}
	keys := map[string]any{"sandbox": map[string]any{"filesystem": emit.NestedJSONObject(map[string]any{"allowWrite": value})}}
	return sess.MergeJSONFileNested(path, keys, []string{"sandbox"}, dryRun)
}
