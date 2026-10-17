package factory

import (
	"encoding/json"
	"fmt"
	"os"

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

// GitignoreHints keeps the machine-local settings file out of Git whether
// or not it holds the handoff grant, since users add their own keys to it.
func (Adapter) GitignoreHints(*config.Config) []string {
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
	// With no grant this sync, the file is not written, so the release of
	// an earlier output takes out only what sync added there.
	if err != nil || handoff == "" || sess.IsUnmanaged(path) {
		return err
	}
	existing, ok := existingAllowWrite(path)
	if !ok {
		emit.NoteProject(fmt.Sprintf("%s: %s stays as configured; to save shared handoffs, add %q to sandbox.filesystem.allowWrite, keeping its other entries", target, path, handoff))
		return nil
	}
	planned := emit.WithoutUserItems(path, allowWritePath, existing, []any{handoff})
	merged, claims := emit.MergeOwnedLists(path, allowWritePath[:2], map[string][]any{"allowWrite": existing}, map[string][]any{"allowWrite": planned})
	value := emit.CarriedJSONValue(merged["allowWrite"])
	if len(claims["allowWrite"]) > 0 {
		value = emit.ClaimedJSONItems(merged["allowWrite"], claims["allowWrite"])
	}
	keys := map[string]any{"sandbox": map[string]any{"filesystem": emit.NestedJSONObject(map[string]any{"allowWrite": value})}}
	return sess.MergeJSONFileNested(path, keys, []string{"sandbox"}, dryRun)
}

// existingAllowWrite returns the allowWrite list on disk, and false when
// a value on its path is not the object or list sync can merge into.
func existingAllowWrite(path string) ([]any, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, true
	}
	data, _ = emit.StripJSONC(data)
	var doc map[string]any
	if json.Unmarshal(data, &doc) != nil {
		return nil, false
	}
	for _, key := range allowWritePath[:2] {
		value, found := doc[key]
		if !found {
			return nil, true
		}
		if doc, _ = value.(map[string]any); doc == nil {
			return nil, false
		}
	}
	value, found := doc["allowWrite"]
	if !found {
		return nil, true
	}
	list, ok := value.([]any)
	return list, ok
}
