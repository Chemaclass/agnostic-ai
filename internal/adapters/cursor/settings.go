package cursor

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// cliConfigFile is the project CLI config. "Only permissions can be
// configured at the project level" (cursor.com/docs/cli/reference/configuration).
const cliConfigFile = ".cursor/cli.json"

// OwnedPermissionsFile records the rules sync added to cli.json, so a
// removed rule or protected path leaves while a hand-written rule stays.
const OwnedPermissionsFile = ".agnostic-ai-permissions.json"

const (
	modelNoOpReason      = "Cursor reads a model only from the user CLI config; a project .cursor/cli.json takes permissions only"
	customNoOpReason     = "a project .cursor/cli.json takes permissions only, and sync writes those from portable permissions and protected paths"
	protectAskNoOpReason = "Cursor CLI permissions have no ask list, so a decision: ask block stays advisory; the CLI already prompts before a write no allow rule covers, and decision: deny blocks it"
)

// ProtectedPaths reports that Cursor enforces deny protected paths with
// CLI permission rules.
func (Adapter) ProtectedPaths() (enforcement, reason string) { return "permission", "" }

// emitCLIConfig writes the translated portable permissions and the deny
// protected paths into .cursor/cli.json, and notes the settings fields
// Cursor has no project key for.
func emitCLIConfig(sess *emit.Session, cfg *config.Config, settings []spec.Entry, dryRun bool) error {
	noteSettingsNoOps(settings)
	groups, err := spec.ProtectedPaths(settings)
	if err != nil {
		return err
	}
	portable, dropped, asks, droppedDeny := cliPermissions(settings)
	emit.NoteFieldNoOp(target, spec.KindSettings, "permissions", dropped, permissionsUntranslatedReason)
	if len(droppedDeny) > 0 {
		emit.NoteProject("deny rule not enforced on cursor: " + strings.Join(droppedDeny, ", ") + " (no Cursor CLI rule blocks exactly that; block it another way)")
	}
	emit.NoteFieldNoOp(target, spec.KindSettings, "permissions.ask", asks, permissionsAskReason)
	var protected []string
	protectAsks := 0
	for _, group := range groups {
		if group.Decision != spec.ProtectDeny {
			protectAsks++
			continue
		}
		for _, path := range group.Paths {
			for _, rule := range protectWriteRules(path) {
				if !slices.Contains(protected, rule) {
					protected = append(protected, rule)
				}
			}
		}
	}
	emit.NoteFieldNoOp(target, spec.KindSettings, "protected", protectAsks, protectAskNoOpReason)
	memory, err := memoryWriteRules(sess, cfg, dryRun)
	if err != nil {
		return err
	}
	owned, err := emit.ReadOwnedRules(filepath.Join(filepath.Dir(cliConfigFile), OwnedPermissionsFile))
	if err != nil || (len(portable) == 0 && len(protected) == 0 && len(memory) == 0 && !owned.Exists()) {
		return err
	}
	onDisk := map[string]any{}
	for _, list := range cliPermissionLists {
		if rules := sess.ExistingNestedStrings(cliConfigFile, "permissions", list, dryRun); rules != nil {
			onDisk[list] = toAny(rules)
		}
	}
	base := owned.Strip(onDisk)
	generated := []map[string]any{{"allow": toAny(portable["allow"]), "deny": toAny(portable["deny"])}, {"deny": toAny(protected)}, {"allow": toAny(memory)}}
	final := map[string]any{}
	for _, list := range cliPermissionLists {
		merged, _ := base[list].([]any)
		for _, layer := range generated {
			for _, rule := range emit.StringSlice(layer[list]) {
				if !slices.Contains(merged, any(rule)) {
					merged = append(merged, rule)
				}
			}
		}
		if len(merged) == 0 && onDisk[list] == nil {
			continue
		}
		if merged == nil {
			merged = []any{}
		}
		final[list] = merged
	}
	empty := true
	for _, list := range cliPermissionLists {
		if rules, _ := final[list].([]any); len(rules) > 0 {
			empty = false
		}
	}
	if empty {
		// With no rule left, the files sync wrote leave with its claims.
		return nil
	}
	// Cursor CLI rejects a permissions object that lacks either list.
	for _, list := range cliPermissionLists {
		if final[list] == nil {
			final[list] = []any{}
		}
	}
	if err := owned.Record(sess, final, base, generated, dryRun); err != nil {
		return err
	}
	// The lists hold the user's rules too, so sync claims only its own.
	// The record is the whole truth: a list with none of sync's rules
	// left releases any rule an earlier sync claimed there.
	owns := emit.OwnedRulesOf(final, base, generated)
	permissions := make(map[string]any, len(final))
	for list, rules := range final {
		permissions[list] = emit.CarriedJSONValue(rules)
		if len(owns[list]) > 0 {
			permissions[list] = emit.ClaimedJSONItems(rules, owns[list])
		} else if len(emit.StringSlice(rules)) == 0 && (onDisk[list] == nil || emit.ClaimsWholeValue(cliConfigFile, "permissions", list)) {
			// An empty list sync adds so Cursor loads the file stays sync's.
			permissions[list] = rules
		}
	}
	return sess.MergeJSONFileNested(cliConfigFile, map[string]any{"permissions": permissions}, []string{"permissions"}, dryRun)
}

// memoryWriteRules allows writes to the repo store of personal memory
// when it lies outside the checkout. Cursor asks before it writes outside
// the workspace, and a headless run rejects the write.
func memoryWriteRules(sess *emit.Session, cfg *config.Config, dryRun bool) ([]string, error) {
	// cli.json holds permissions alone, so only a committed settings kind
	// keeps the absolute path out of it.
	if cfg == nil || !slices.Contains(cfg.Builtins, emit.MemoryBuiltin) || !emit.PersonalMemoryLeavesCheckoutIn(cfg, cliConfigFile, []string{"settings"}, target) {
		return nil, nil
	}
	dir, err := emit.PersonalMemoryDir(cfg, ".")
	if err != nil {
		return nil, err
	}
	if err := sess.CreateRepoMemoryStore(cfg, dir, dryRun); err != nil {
		return nil, err
	}
	return []string{"Write(" + filepath.ToSlash(dir) + "/**)"}, nil
}

// cliPermissionLists are the two lists Cursor CLI permissions take.
var cliPermissionLists = []string{"allow", "deny"}

func toAny(rules []string) []any {
	out := make([]any, len(rules))
	for i, rule := range rules {
		out[i] = rule
	}
	return out
}

// protectWriteRules spells a normalized protected path as Cursor CLI
// Write rules. A relative path is "scoped to the current workspace"
// while a leading `/` is an absolute path, so the rule has none. A
// protected path also covers every file under it, which a glob does
// not, so a second rule adds `/**`.
func protectWriteRules(path string) []string {
	rules := []string{"Write(" + path + ")"}
	if !strings.HasSuffix(path, "**") {
		rules = append(rules, "Write("+path+"/**)")
	}
	return rules
}

func noteSettingsNoOps(settings []spec.Entry) {
	models, custom := 0, 0
	for _, entry := range settings {
		if emit.SettingsModel([]spec.Entry{entry}, target) != "" {
			models++
		}
		if _, ok := entry.Meta["x-"+target]; ok {
			custom++
		}
	}
	emit.NoteFieldNoOp(target, spec.KindSettings, "model", models, modelNoOpReason)
	emit.NoteFieldNoOp(target, spec.KindSettings, "x-"+target, custom, customNoOpReason)
}
