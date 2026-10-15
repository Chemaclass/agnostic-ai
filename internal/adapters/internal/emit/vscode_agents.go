package emit

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// VSCodeSettingsFile is VS Code's workspace settings file.
const VSCodeSettingsFile = ".vscode/settings.json"

// VSCodeAgentsMdKey turns VS Code's reading of the root AGENTS.md on or
// off. It defaults to true, has no scope restriction (so a workspace may
// set it), and also gates the nested AGENTS.md files of
// chat.useNestedAgentsMdFiles (microsoft/vscode chat.shared.contribution.ts
// and the Copilot extension's agentInstructionsLocator.ts).
const VSCodeAgentsMdKey = "chat.useAgentsMdFile"

// CopilotSharesRootAgents reports whether Copilot's own instructions
// file and a synced root AGENTS.md both exist. They carry the same
// body, and VS Code chat reads both.
func CopilotSharesRootAgents(cfg *config.Config) bool {
	if cfg == nil || !slices.Contains(cfg.Targets, "copilot") || EntryPointPath(cfg, "copilot") == "AGENTS.md" || cfg.IsUnmanaged("AGENTS.md") {
		return false
	}
	for _, t := range cfg.Targets {
		if t != "copilot" && EntryPointPath(cfg, t) == "AGENTS.md" {
			return true
		}
	}
	return false
}

// SyncSetsVSCodeAgentsMd reports whether sync turns VS Code's AGENTS.md
// reading off: Copilot shares the root AGENTS.md, and the key is unset
// or holds the false an earlier sync wrote. A value the user set wins.
func SyncSetsVSCodeAgentsMd(cfg *config.Config) bool {
	if !CopilotSharesRootAgents(cfg) || cfg.IsUnmanaged(VSCodeSettingsFile) {
		return false
	}
	raw, found := vscodeAgentsMdValue()
	return !found || unchangedSince(priorMergedKeys(VSCodeSettingsFile), []string{VSCodeAgentsMdKey}, raw)
}

// VSCodeReadsRootAgents reports whether VS Code chat still loads the
// root AGENTS.md next to Copilot's instructions file after sync.
func VSCodeReadsRootAgents(cfg *config.Config) bool {
	if !CopilotSharesRootAgents(cfg) || SyncSetsVSCodeAgentsMd(cfg) {
		return false
	}
	raw, found := vscodeAgentsMdValue()
	return !found || !bytes.Equal(bytes.TrimSpace(raw), []byte("false"))
}

func vscodeAgentsMdValue() (json.RawMessage, bool) {
	data, err := os.ReadFile(VSCodeSettingsFile)
	if err != nil {
		return nil, false
	}
	stripped, _ := StripJSONC(data)
	var doc map[string]json.RawMessage
	if json.Unmarshal(stripped, &doc) != nil {
		return nil, false
	}
	raw, found := doc[VSCodeAgentsMdKey]
	return raw, found
}
