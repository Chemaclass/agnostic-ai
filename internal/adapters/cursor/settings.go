package cursor

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// cliConfigFile is the project CLI config. "Only permissions can be
// configured at the project level" (cursor.com/docs/cli/reference/configuration).
const cliConfigFile = ".cursor/cli.json"

// ProtectedRulesFile records the Write rules sync added to cli.json, so
// a removed protected path loses its rule while a hand-written one stays.
const ProtectedRulesFile = ".agnostic-ai-protected.json"

const (
	modelNoOpReason       = "Cursor reads a model only from the user CLI config; a project .cursor/cli.json takes permissions only"
	permissionsNoOpReason = "sync does not translate portable permission rules into Cursor CLI permissions yet"
	customNoOpReason      = "a project .cursor/cli.json takes permissions only, and sync writes those from protected paths"
	protectAskNoOpReason  = "Cursor CLI permissions have no ask list, so a decision: ask block stays advisory; the CLI already prompts before a write no allow rule covers, and decision: deny blocks it"
)

// ProtectedPaths reports that Cursor enforces deny protected paths with
// CLI permission rules.
func (Adapter) ProtectedPaths() (enforcement, reason string) { return "permission", "" }

// emitCLIConfig writes the deny protected paths into .cursor/cli.json and
// notes the settings fields Cursor has no project key for.
func emitCLIConfig(sess *emit.Session, settings []spec.Entry, dryRun bool) error {
	noteSettingsNoOps(settings)
	groups, err := spec.ProtectedPaths(settings)
	if err != nil {
		return err
	}
	var deny []string
	asks := 0
	for _, group := range groups {
		if group.Decision != spec.ProtectDeny {
			asks++
			continue
		}
		for _, path := range group.Paths {
			for _, rule := range protectWriteRules(path) {
				if !slices.Contains(deny, rule) {
					deny = append(deny, rule)
				}
			}
		}
	}
	emit.NoteFieldNoOp(target, spec.KindSettings, "protected", asks, protectAskNoOpReason)
	rules := map[string][]string{}
	if len(deny) > 0 {
		rules["deny"] = deny
	}
	owned, err := emit.ReadOwnedRules(filepath.Join(filepath.Dir(cliConfigFile), ProtectedRulesFile), rules)
	if err != nil || !owned.Active() {
		return err
	}
	var existing []any
	for _, rule := range sess.ExistingNestedStrings(cliConfigFile, "permissions", "deny", dryRun) {
		existing = append(existing, rule)
	}
	base := owned.Strip(map[string]any{"deny": existing})
	merged, _ := base["deny"].([]any)
	for _, rule := range deny {
		if !slices.Contains(merged, any(rule)) {
			merged = append(merged, rule)
		}
	}
	if merged == nil {
		merged = []any{}
	}
	if err := owned.Record(sess, base, dryRun); err != nil {
		return err
	}
	return sess.MergeJSONFileNested(cliConfigFile, map[string]any{"permissions": map[string]any{"deny": merged}}, []string{"permissions"}, dryRun)
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
	emit.NoteFieldNoOp(target, spec.KindSettings, "permissions", emit.SpecsWithPermissions(settings), permissionsNoOpReason)
	emit.NoteFieldNoOp(target, spec.KindSettings, "x-"+target, custom, customNoOpReason)
}
