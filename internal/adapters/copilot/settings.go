package copilot

import (
	"sort"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// disabledMcpServersKey is the repository-level key Copilot CLI reads
// for servers it should configure but not start. The repository-settings
// table on
// docs.github.com/en/copilot/reference/copilot-cli-reference/cli-config-dir-reference
// rows it as "`disabledMcpServers` | `string[]` | Union—repository can
// add entries, never remove | MCP servers configured but not started",
// under "Only the keys listed in the following table are supported at
// the repository level."
const disabledMcpServersKey = "disabledMcpServers"

// emitSettings merges the portable default model and the names of any
// MCP servers marked `disabled: true` into Copilot CLI's repository
// settings. MergeJSONFile preserves native sibling keys such as
// respectGitignore while making the Settings spec authoritative for
// model, and the disabled list authoritative for its own key.
//
// The disabled names come from the MCP specs, not from a settings
// spec: `disabled` is a per-server field, and this is the only
// file-based route Copilot publishes for it (target-audit 2026-09-19,
// #888).
// permissionsMDMOnlyReason explains why a portable permission policy
// reaches nothing in `.github/copilot/settings.json`. Copilot's
// repository-settings table is closed: "Only the keys listed in the
// following table are supported at the repository level. Any other
// keys, including keys that are valid in the user configuration file,
// are silently ignored". Its fourteen keys name no `permissions`
// (docs.github.com/en/copilot/reference/copilot-cli-reference/cli-config-dir-reference).
//
// Copilot does publish a deny/ask/allow rule grammar close to this
// project's own (`Bash(...)`, `Read(...)`, `Edit(...)`, `Domain(...)`),
// but only under MDM managed settings, at device paths such as
// `/etc/github-copilot/managed-settings.json`. Those sit outside any
// repository, so a project-tier tool cannot write them. The user-tier
// `~/.copilot/permissions-config.json` is not a fallback either: the
// vendor states it "doesn't support deny rules, 'ask' rules, default
// modes, URL rules, tool filtering, or repository-local shared policy"
// (target-audit 2026-09-19, #917).
const permissionsMDMOnlyReason = "Copilot's deny/ask/allow rule grammar exists only in device-level MDM managed settings; the repository-settings table for .github/copilot/settings.json names no permissions key"

func emitSettings(sess *emit.Session, settings, mcps []spec.Entry, dryRun bool) error {
	emit.NoteFieldNoOp(target, spec.KindSettings, "permissions",
		emit.SpecsWithPermissions(settings), permissionsMDMOnlyReason)
	keys := map[string]any{}
	if model := emit.LastSettingsModel(settings); model != "" {
		keys["model"] = model
	}
	if names := disabledMCPNames(mcps); len(names) > 0 {
		keys[disabledMcpServersKey] = names
	}
	// The `x-copilot` block on a settings spec carries the repository
	// keys this adapter does not model, `respectGitignore` among the
	// fourteen that table lists. Without it they are unreachable (#949).
	emit.MergeSettingsCustomKeys(keys, settings, target)
	if len(keys) == 0 {
		return nil
	}
	return sess.MergeJSONFile(defaultSettingsFile, keys, dryRun)
}

// disabledMCPNames returns the names of every MCP spec carrying
// `disabled: true`, sorted so two syncs of the same specs produce the
// same file.
func disabledMCPNames(mcps []spec.Entry) []string {
	var out []string
	for _, entry := range mcps {
		if entry.Name == "" {
			continue
		}
		if disabled, _ := entry.Meta["disabled"].(bool); disabled {
			out = append(out, entry.Name)
		}
	}
	sort.Strings(out)
	return out
}
