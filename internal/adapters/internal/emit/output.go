package emit

import "github.com/chemaclass/agnostic-ai/internal/config"

// OutputFile returns cfg.Outputs[target].File when set, otherwise fallback.
func OutputFile(cfg *config.Config, target, fallback string) string {
	if o, ok := cfg.Outputs[target]; ok && o.File != "" {
		return o.File
	}
	return fallback
}

// OutputDir returns cfg.Outputs[target].Dir when set, otherwise fallback.
func OutputDir(cfg *config.Config, target, fallback string) string {
	if o, ok := cfg.Outputs[target]; ok && o.Dir != "" {
		return o.Dir
	}
	return fallback
}

// OutputRulesDir returns cfg.Outputs[target].RulesDir when set, otherwise fallback.
func OutputRulesDir(cfg *config.Config, target, fallback string) string {
	if o, ok := cfg.Outputs[target]; ok && o.RulesDir != "" {
		return o.RulesDir
	}
	return fallback
}

// OutputRulesFile returns cfg.Outputs[target].RulesFile when set, otherwise fallback.
func OutputRulesFile(cfg *config.Config, target, fallback string) string {
	if o, ok := cfg.Outputs[target]; ok && o.RulesFile != "" {
		return o.RulesFile
	}
	return fallback
}

// OutputMCPFile returns cfg.Outputs[target].MCPFile when set, otherwise fallback.
func OutputMCPFile(cfg *config.Config, target, fallback string) string {
	if o, ok := cfg.Outputs[target]; ok && o.MCPFile != "" {
		return o.MCPFile
	}
	return fallback
}

// OutputAgentsDir returns cfg.Outputs[target].AgentsDir when set, otherwise fallback.
func OutputAgentsDir(cfg *config.Config, target, fallback string) string {
	if o, ok := cfg.Outputs[target]; ok && o.AgentsDir != "" {
		return o.AgentsDir
	}
	return fallback
}

// OutputSkillsDir returns cfg.Outputs[target].SkillsDir when set, otherwise fallback.
func OutputSkillsDir(cfg *config.Config, target, fallback string) string {
	if o, ok := cfg.Outputs[target]; ok && o.SkillsDir != "" {
		return o.SkillsDir
	}
	return fallback
}

// OutputInstructionsDir returns cfg.Outputs[target].InstructionsDir when set, otherwise fallback.
func OutputInstructionsDir(cfg *config.Config, target, fallback string) string {
	if o, ok := cfg.Outputs[target]; ok && o.InstructionsDir != "" {
		return o.InstructionsDir
	}
	return fallback
}

// OutputCommandsDir returns cfg.Outputs[target].CommandsDir when set, otherwise fallback.
func OutputCommandsDir(cfg *config.Config, target, fallback string) string {
	if o, ok := cfg.Outputs[target]; ok && o.CommandsDir != "" {
		return o.CommandsDir
	}
	return fallback
}

// OutputMCPDir returns cfg.Outputs[target].MCPDir when set, otherwise fallback.
// Used by adapters that emit one MCP server file per entry into a directory
// (Continue: `.continue/mcpServers/`) rather than a single combined MCP file.
func OutputMCPDir(cfg *config.Config, target, fallback string) string {
	if o, ok := cfg.Outputs[target]; ok && o.MCPDir != "" {
		return o.MCPDir
	}
	return fallback
}

// EmitSkillsAsCommands reports whether the named target opts skills into
// slash-command emission. Off by default.
func EmitSkillsAsCommands(cfg *config.Config, target string) bool {
	if o, ok := cfg.Outputs[target]; ok {
		return o.EmitSkillsAsCommands
	}
	return false
}
