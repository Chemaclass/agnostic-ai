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

// OutputReviewFile returns cfg.Outputs[target].ReviewFile when set, otherwise fallback.
func OutputReviewFile(cfg *config.Config, target, fallback string) string {
	if o, ok := cfg.Outputs[target]; ok && o.ReviewFile != "" {
		return o.ReviewFile
	}
	return fallback
}

// OutputEnvironmentFile returns cfg.Outputs[target].EnvironmentFile when set, otherwise fallback.
func OutputEnvironmentFile(cfg *config.Config, target, fallback string) string {
	if o, ok := cfg.Outputs[target]; ok && o.EnvironmentFile != "" {
		return o.EnvironmentFile
	}
	return fallback
}

// OutputIgnoreFile returns cfg.Outputs[target].IgnoreFile when set, otherwise fallback.
func OutputIgnoreFile(cfg *config.Config, target, fallback string) string {
	if o, ok := cfg.Outputs[target]; ok && o.IgnoreFile != "" {
		return o.IgnoreFile
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

// OutputHooksFile returns cfg.Outputs[target].HooksFile when set, otherwise fallback.
func OutputHooksFile(cfg *config.Config, target, fallback string) string {
	if o, ok := cfg.Outputs[target]; ok && o.HooksFile != "" {
		return o.HooksFile
	}
	return fallback
}

// OutputHooksDir returns cfg.Outputs[target].HooksDir when set, otherwise
// fallback. Used by adapters that emit one hook definition file per hook
// into a directory (Kiro: `.kiro/hooks/<name>.json`) rather than a single
// combined hooks file.
func OutputHooksDir(cfg *config.Config, target, fallback string) string {
	if o, ok := cfg.Outputs[target]; ok && o.HooksDir != "" {
		return o.HooksDir
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

// OutputChatmodesDir returns cfg.Outputs[target].ChatmodesDir when set,
// otherwise fallback. Used by the Copilot adapter to opt agents into
// emission as Custom Chat Modes.
func OutputChatmodesDir(cfg *config.Config, target, fallback string) string {
	if o, ok := cfg.Outputs[target]; ok && o.ChatmodesDir != "" {
		return o.ChatmodesDir
	}
	return fallback
}

// OutputWorkflowsDir returns cfg.Outputs[target].WorkflowsDir when set,
// otherwise fallback. Used by Cline, Windsurf, and Warp adapters to
// opt agents into emission as workflows.
func OutputWorkflowsDir(cfg *config.Config, target, fallback string) string {
	if o, ok := cfg.Outputs[target]; ok && o.WorkflowsDir != "" {
		return o.WorkflowsDir
	}
	return fallback
}

// OutputAssistantsDir returns cfg.Outputs[target].AssistantsDir when
// set, otherwise fallback. Used by the Continue adapter to opt agents
// into emission as Continue assistants.
func OutputAssistantsDir(cfg *config.Config, target, fallback string) string {
	if o, ok := cfg.Outputs[target]; ok && o.AssistantsDir != "" {
		return o.AssistantsDir
	}
	return fallback
}

// OutputTasksFile returns cfg.Outputs[target].TasksFile when set,
// otherwise fallback. Used by the Zed adapter to opt hooks into
// emission as Zed tasks.
func OutputTasksFile(cfg *config.Config, target, fallback string) string {
	if o, ok := cfg.Outputs[target]; ok && o.TasksFile != "" {
		return o.TasksFile
	}
	return fallback
}

// OutputConfFile returns cfg.Outputs[target].ConfFile when set,
// otherwise fallback. Used by the Aider adapter to opt into writing
// .aider.conf.yml.
func OutputConfFile(cfg *config.Config, target, fallback string) string {
	if o, ok := cfg.Outputs[target]; ok && o.ConfFile != "" {
		return o.ConfFile
	}
	return fallback
}

// OutputModel returns cfg.Outputs[target].Model when set, otherwise fallback.
// Used by the Aider adapter to write the `model:` key into
// `.aider.conf.yml`.
func OutputModel(cfg *config.Config, target, fallback string) string {
	if o, ok := cfg.Outputs[target]; ok && o.Model != "" {
		return o.Model
	}
	return fallback
}

// OutputWeakModel returns cfg.Outputs[target].WeakModel when set,
// otherwise fallback. Used by the Aider adapter to write the
// `weak-model:` key into `.aider.conf.yml`.
func OutputWeakModel(cfg *config.Config, target, fallback string) string {
	if o, ok := cfg.Outputs[target]; ok && o.WeakModel != "" {
		return o.WeakModel
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
