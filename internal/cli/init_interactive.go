package cli

// targetChoice is one selectable AI CLI target shown to the user and
// written to agnostic.config.yaml.
type targetChoice struct {
	Name string // canonical name written to the config
	Desc string // short human description shown in prompts
}

// allTargets is the canonical target list, in display + emit order.
// Single source of truth for renderConfig and the interactive prompt.
var allTargets = []targetChoice{
	{Name: "claude", Desc: "Claude Code (Anthropic)"},
	{Name: "codex", Desc: "Codex CLI (OpenAI)"},
	{Name: "gemini", Desc: "Gemini CLI (Google)"},
	{Name: "cursor", Desc: "Cursor (cursor.com)"},
	{Name: "copilot", Desc: "GitHub Copilot"},
	{Name: "aider", Desc: "Aider (aider.chat)"},
	{Name: "cline", Desc: "Cline (VSCode extension)"},
	{Name: "windsurf", Desc: "Windsurf (Codeium)"},
	{Name: "continue", Desc: "Continue (continue.dev)"},
	{Name: "amp", Desc: "Amp (Sourcegraph)"},
	{Name: "zed", Desc: "Zed editor"},
	{Name: "warp", Desc: "Warp terminal"},
	{Name: "opencode", Desc: "OpenCode"},
}

// allTargetNames returns just the canonical names of allTargets.
func allTargetNames() []string {
	out := make([]string, len(allTargets))
	for i, t := range allTargets {
		out[i] = t.Name
	}
	return out
}
