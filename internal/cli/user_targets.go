package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/augment"
	"github.com/chemaclass/agnostic-ai/internal/adapters/claude"
	"github.com/chemaclass/agnostic-ai/internal/adapters/codebuddy"
	"github.com/chemaclass/agnostic-ai/internal/adapters/codex"
	"github.com/chemaclass/agnostic-ai/internal/adapters/copilot"
	"github.com/chemaclass/agnostic-ai/internal/adapters/gemini"
	"github.com/chemaclass/agnostic-ai/internal/adapters/grok"
	"github.com/chemaclass/agnostic-ai/internal/adapters/openhands"
	"github.com/chemaclass/agnostic-ai/internal/adapters/pi"
	"github.com/chemaclass/agnostic-ai/internal/adapters/qwen"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Path prefixes used in the userTargets table. A path carries its own
// root because several tools split surfaces across two roots: Kilo Code
// reads instructions from ~/.config/kilo/ but skills from ~/.kilo/, and
// Goose reads hints from ~/.config/goose/ but skills from ~/.agents/.
const (
	userPathHome         = "home:"
	userPathXDG          = "xdg:"
	userPathAppData      = "appdata:"
	userPathLocalAppData = "localappdata:"
)

const (
	hookTargetSettingsEnv = "settings-env"
	hookTargetExport      = "export"
	hookTargetHandlerEnv  = "handler-env"
	hookTargetSessionEnv  = "session-env"
)

// userTarget declares one tool's user-level paths and discovery settings.
// Default paths use userPathHome or userPathXDG.
//
// An empty field means sync --scope user has no supported user-level surface
// of that kind.
type userTarget struct {
	// instructions is the always-on context file the tool loads with no
	// wiring. Global rules inline into it.
	instructions        string
	instructionsWindows string
	instructionsRoot    func(home, goos string) (string, error)
	// rules is a per-rule directory, used only for a target whose
	// vendor documents no user-level instructions file at all.
	rules string
	// skills is the skills directory.
	skills string
	// agents is the native agent directory.
	agents string
	// rootEnv names the vendor's configuration root variable. When set,
	// its value replaces root for every surface under root, so all of a
	// target's files move together.
	rootEnv string
	// root is the default location rootEnv replaces.
	root string
	// rootSubdir is appended to the rootEnv value, for a vendor whose
	// variable names the parent of its configuration directory.
	rootSubdir string
	// agentEfforts is the user settings file holding per-agent effort
	// for a target whose portable effort is not mapped into agent files.
	agentEfforts string
	// agentsWindows is relative to APPDATA when the vendor uses it on Windows.
	agentsWindows string
	// hooks is the hooks file.
	hooks string
	// hooksDir is a native directory of standalone hook definition files.
	hooksDir string
	// hookScriptsDir keeps command scripts outside the native definitions directory.
	hookScriptsDir string
	// hooksFormat selects the native hooks schema: "claude", "cursor",
	// "copilot", or "augment".
	hooksFormat string
	// hookTarget is how a hook learns which target ran it, the same way
	// the project adapter tells it: the settings `env` (claude), an
	// `export` prefix with the declared command kept for Windows
	// (codex), each handler's `env` (gemini, qoder), or a sessionStart
	// hook's `env` output (cursor). See the hookTarget* constants.
	hookTarget string
	// hookTimeout converts a spec's timeout in seconds to the unit the
	// target reads, for a target that reads another one. Nil keeps it.
	hookTimeout func(meta map[string]any) (any, bool)
	// hookArgs marks a Claude-shaped hooks file whose command handler
	// takes `args`, the exec form (claude, qoder). The augment format
	// writes `args` on its own.
	hookArgs bool
	// hookFoldArgs marks a target with no `args` field (codex, cursor,
	// gemini): the args fold into the command, each quoted for a POSIX
	// shell, since a bare interpreter would read the hook's JSON payload
	// as its program.
	hookFoldArgs bool
	// bridge marks a target that does not auto-load its user-level
	// instructions file, so the body is injected through a managed
	// session-start hook instead.
	bridge bool
	// bridgeEvent is the session-start event name in the target's own
	// hook vocabulary.
	bridgeEvent string
	// bridgeKey is the JSON field the target reads injected context
	// from in that hook's stdout.
	bridgeKey string
	// settings is the user settings file the portable settings fields
	// write to, keyed per native key. Its zero value means none.
	settings userSettingsFile
	// mcp is the user file holding MCP servers. Its zero value means
	// none.
	mcp userMCPFile
}

// userTargets maps target name to its user-level surfaces, as
// documented by each vendor (agents verified 2026-09-22). Only targets
// listed here are accepted by sync --scope user.
//
// Absent by verdict: aider (a home instructions file reaches it only
// through a `read:` entry in ~/.aider.conf.yml, never automatically),
// continue (its one home surface is the `rules:` list inside the
// config.yaml Continue itself rewrites), and jules (nothing documented
// at user scope at all).
var userTargets = map[string]userTarget{
	"codebuddy": {
		instructions: userPathHome + ".codebuddy/CODEBUDDY.md",
		agents:       userPathHome + ".codebuddy/agents",
		skills:       userPathHome + ".codebuddy/skills",
		hooks:        userPathHome + ".codebuddy/settings.json", hooksFormat: "claude", hookFoldArgs: true,
		mcp:      userMCPFile{path: userPathHome + ".codebuddy/.mcp.json", format: "json", key: "mcpServers", legacy: []string{userPathHome + ".codebuddy/mcp.json", userPathHome + ".codebuddy.json"}, private: true},
		settings: userSettingsFile{path: userPathHome + ".codebuddy/settings.json", format: "json", model: "model", effort: "reasoningEffort", effortLevel: codebuddy.Adapter{}.SettingsEffortLevels, permissionMode: "permissions.defaultMode", permissionLists: true, retention: "cleanupPeriodDays", reserved: map[string]string{"hooks": "sync --scope user writes hooks from hook specs", "permissions": "use permissions.default-mode and permission lists; sync writes individual permission entries"}},
	},
	"grok": {
		rootEnv: "GROK_HOME", root: userPathHome + ".grok",
		instructions: userPathHome + ".grok/AGENTS.md", skills: userPathHome + ".grok/skills", agents: userPathHome + ".grok/agents",
		hooksDir: userPathHome + ".grok/hooks", hookScriptsDir: userPathHome + ".grok/hook-scripts",
		mcp: userMCPFile{path: userPathHome + ".grok/config.toml", format: "toml", key: "mcp_servers", private: true, validate: grok.ValidateMCPs},
		settings: userSettingsFile{path: userPathHome + ".grok/config.toml", format: grokTOMLFormat, private: true, model: "models.default",
			voice: &userVoiceKeys{
				mode: "ui.voice_capture_mode", language: "ui.voice_stt_language",
				modes: map[string]string{"hold": "hold", "tap": "toggle"},
				notes: map[string]string{"enabled": "Grok has no dictation on/off key; /voice always works. Set x-grok.ui.voice_keybind_enabled to turn the shortcut off"},
			},
			reserved: map[string]string{"mcp_servers": "sync --scope user writes MCP servers from MCP specs", "permission": "sync --scope user does not write Grok user permission rules", "hooks": "sync --scope user writes hook definitions from hook specs", "compat": "set compatibility policy by hand", "trust": "set trust by hand", "models": "use the portable model field"}},
	},
	"kimi": {
		rootEnv: "KIMI_CODE_HOME", root: userPathHome + ".kimi-code",
		instructions: userPathHome + ".kimi-code/AGENTS.md",
		agents:       userPathHome + ".kimi-code/agents",
		skills:       userPathHome + ".kimi-code/skills",
		hooks:        userPathHome + ".kimi-code/config.toml",
		hooksFormat:  "kimi-toml", hookFoldArgs: true,
		mcp: userMCPFile{path: userPathHome + ".kimi-code/mcp.json", format: "json", key: "mcpServers", private: true},
		settings: userSettingsFile{
			path: userPathHome + ".kimi-code/config.toml", format: "toml", model: "default_model", private: true,
			allowed: map[string]string{"default_model": "string", "default_plan_mode": "bool", "telemetry": "bool"},
		},
	},
	"vibe": {
		rootEnv:      "VIBE_HOME",
		root:         userPathHome + ".vibe",
		instructions: userPathHome + ".vibe/AGENTS.md",
		agents:       userPathHome + ".vibe/agents",
		skills:       userPathHome + ".vibe/skills",
		hooks:        userPathHome + ".vibe/hooks.toml",
		hooksFormat:  "vibe-toml",
		hookFoldArgs: true,
		mcp:          userMCPFile{path: userPathHome + ".vibe/config.toml", format: "toml-array", key: "mcp_servers", private: true},
		settings: userSettingsFile{
			path: userPathHome + ".vibe/config.toml", format: "toml", model: "active_model", private: true,
			voice: &userVoiceKeys{enabled: "voice_mode_enabled", notes: map[string]string{
				"mode": "Vibe has no voice capture mode setting", "language": "Vibe has no dictation language setting",
			}},
			reserved: map[string]string{
				"mcp_servers":     "sync --scope user writes MCP entries from specs",
				"trusted_folders": "trust remains with the user",
				"default_agent":   "select the primary agent in Vibe; approvals review sets it to smart-approve",
			},
		},
	},
	"pi": {
		rootEnv: "PI_CODING_AGENT_DIR", root: userPathHome + ".pi/agent",
		instructions: userPathHome + ".pi/agent/AGENTS.md",
		skills:       userPathHome + ".pi/agent/skills",
		mcp:          userMCPFile{path: userPathHome + ".pi/agent/mcp.json", format: "json", key: "mcpServers"},
		settings: userSettingsFile{path: userPathHome + ".pi/agent/settings.json", format: "json", model: "defaultModel", effort: "defaultThinkingLevel", effortLevel: pi.Adapter{}.SettingsEffortLevels,
			reserved: map[string]string{"defaultProjectTrust": "Pi project trust stays with the user"},
		},
	},
	"claude": {
		rootEnv:      "CLAUDE_CONFIG_DIR",
		root:         userPathHome + ".claude",
		instructions: userPathHome + ".claude/CLAUDE.md",
		agents:       userPathHome + ".claude/agents",
		skills:       userPathHome + ".claude/skills",
		hooks:        userPathHome + ".claude/settings.json",
		hooksFormat:  "claude",
		hookTarget:   hookTargetSettingsEnv,
		hookArgs:     true,
		// Claude Code rewrites ~/.claude.json itself, so sync edits only
		// its own mcpServers entries, as text, and leaves every other key.
		mcp: userMCPFile{path: userPathHome + ".claude.json", format: "json", key: "mcpServers", rootFile: ".claude.json", private: true},
		settings: userSettingsFile{
			path: userPathHome + ".claude/settings.json", format: "json",
			model: "model", effort: "effortLevel", effortLevel: claude.Adapter{}.SettingsEffortLevels,
			ignored: claude.UserIgnoredSetting,
			effortGap: userSurfaceGap{
				surface: "the effort of Opus 5.5 and later models",
				reason:  "user-level effortLevel reaches Opus 5, Fable 5.1, and earlier models only; newer models need a level saved per model with /effort",
			},
			permissionMode:  "permissions.defaultMode",
			permissionLists: true,
			retention:       "cleanupPeriodDays",
			voice: &userVoiceKeys{
				enabled: "voice.enabled", mode: "voice.mode",
				notes: map[string]string{"language": "Claude reads the dictation language from its response language; set x-claude.language to change both"},
			},
			reserved: map[string]string{
				"hooks":       "sync --scope user writes hooks from hook specs",
				"permissions": "use permissions.default-mode for the user default mode; sync --scope user writes only the allow, ask, and deny rule lists under permissions",
			},
		},
	},
	"cursor": {
		agents:       userPathHome + ".cursor/agents",
		instructions: userPathHome + ".cursor/AGENTS.md",
		skills:       userPathHome + ".cursor/skills",
		hooks:        userPathHome + ".cursor/hooks.json",
		mcp:          userMCPFile{path: userPathHome + ".cursor/mcp.json", format: "json", key: "mcpServers"},
		hooksFormat:  "cursor",
		hookTarget:   hookTargetSessionEnv,
		// cursor-agent rewrites cli-config.json itself, so sync owns only
		// approvalMode there.
		settings: userSettingsFile{
			path: userPathHome + ".cursor/cli-config.json", format: "json", resolve: cursorCLIConfigPath, toolOwned: true,
			absEnv:        []string{"CURSOR_CONFIG_DIR", "XDG_CONFIG_HOME"},
			allowed:       map[string]string{"approvalMode": "enum:allowlist|unrestricted|auto-review"},
			allowedReason: "sync --scope user writes only approvalMode to cursor-agent's cli-config.json",
		},
		hookFoldArgs: true,
		bridge:       true,
		bridgeEvent:  "sessionStart",
		bridgeKey:    "additional_context",
	},
	"codex": {
		rootEnv:      "CODEX_HOME",
		root:         userPathHome + ".codex",
		agents:       userPathHome + ".codex/agents",
		instructions: userPathHome + ".codex/AGENTS.md",
		skills:       userPathHome + ".agents/skills",
		hooks:        userPathHome + ".codex/hooks.json",
		hooksFormat:  "claude",
		hookTarget:   hookTargetExport,
		hookFoldArgs: true,
		mcp:          userMCPFile{path: userPathHome + ".codex/config.toml", format: "toml", key: "mcp_servers"},
		settings: userSettingsFile{
			path: userPathHome + ".codex/config.toml", format: "toml",
			model: "model", effort: "model_reasoning_effort", effortLevel: codex.Adapter{}.SettingsEffortLevels,
		},
	},
	"qwen": {
		rootEnv: "QWEN_HOME", root: userPathHome + ".qwen",
		instructions: userPathHome + ".qwen/QWEN.md", agents: userPathHome + ".qwen/agents", skills: userPathHome + ".qwen/skills",
		hooks: userPathHome + ".qwen/settings.json", hooksFormat: "claude", hookTarget: hookTargetHandlerEnv, hookFoldArgs: true, hookTimeout: qwen.HookTimeout,
		mcp: userMCPFile{path: userPathHome + ".qwen/settings.json", format: "json", key: "mcpServers"},
		settings: userSettingsFile{path: userPathHome + ".qwen/settings.json", format: "json", model: "model.name", effort: "model.reasoningEffort", effortLevel: qwen.Adapter{}.SettingsEffortLevels, permissionLists: true, itemLists: []string{"mcp.excluded"},
			voice:    &userVoiceKeys{enabled: "general.voice.enabled", mode: "general.voice.mode", language: "general.voice.language"},
			reserved: map[string]string{"hooks": "sync --scope user writes hooks from hook specs", "mcpServers": "sync --scope user writes MCP servers from MCP specs"}},
	},
	"gemini": {
		rootEnv:      "GEMINI_CLI_HOME",
		root:         userPathHome + ".gemini",
		rootSubdir:   ".gemini",
		agents:       userPathHome + ".gemini/agents",
		instructions: userPathHome + ".gemini/GEMINI.md",
		skills:       userPathHome + ".gemini/skills",
		hooks:        userPathHome + ".gemini/settings.json",
		hooksFormat:  "claude",
		hookTarget:   hookTargetHandlerEnv,
		hookFoldArgs: true,
		hookTimeout:  gemini.HookTimeout,
		mcp:          userMCPFile{path: userPathHome + ".gemini/settings.json", format: "json", key: "mcpServers"},
		settings: userSettingsFile{
			path: userPathHome + ".gemini/settings.json", format: "json",
			retention: "general.sessionRetention.maxAge",
			voice: &userVoiceKeys{
				enabled: "experimental.voiceMode", mode: "experimental.voice.activationMode",
				modes: map[string]string{"hold": "push-to-talk", "tap": "toggle"},
				notes: map[string]string{"language": "Gemini has no dictation language setting"},
			},
			// Gemini sets thinking per model under modelConfigs, with no
			// default effort key, so a portable effort raises a note.
			model: "model.name",
			reserved: map[string]string{
				"hooks":      "sync --scope user writes hooks from hook specs",
				"mcpServers": "sync --scope user writes MCP servers from MCP specs",
			},
		},
	},
	"qoder": {
		rootEnv:      "QODER_CONFIG_DIR",
		root:         userPathHome + ".qoder",
		agents:       userPathHome + ".qoder/agents",
		instructions: userPathHome + ".qoder/AGENTS.md",
		skills:       userPathHome + ".qoder/skills",
		hooks:        userPathHome + ".qoder/settings.json",
		hooksFormat:  "claude",
		hookTarget:   hookTargetHandlerEnv,
		hookArgs:     true,
		mcp:          userMCPFile{path: userPathHome + ".qoder/settings.json", format: "json", key: "mcpServers"},
		settings: userSettingsFile{
			path: userPathHome + ".qoder/settings.json", format: "json",
			retention: "general.sessionRetention.maxAge",
			model:     "model.name", effort: "model.reasoningEffort", effortLevel: qoderUserEffortLevels,
			reserved: map[string]string{
				"hooks":       "sync --scope user writes hooks from hook specs",
				"mcpServers":  "sync --scope user writes MCP servers from MCP specs",
				"permissions": "sync --scope user does not write user-level permissions",
			},
		},
	},
	"copilot": {
		hooks:        userPathHome + ".copilot/hooks/agnostic-ai.json",
		hooksFormat:  "copilot",
		rootEnv:      "COPILOT_HOME",
		root:         userPathHome + ".copilot",
		agents:       userPathHome + ".copilot/agents",
		agentEfforts: userPathHome + ".copilot/settings.json",
		instructions: userPathHome + ".copilot/copilot-instructions.md",
		skills:       userPathHome + ".copilot/skills",
		mcp:          userMCPFile{path: userPathHome + ".copilot/mcp-config.json", format: "json", key: "mcpServers"},
		settings: userSettingsFile{
			path: userPathHome + ".copilot/settings.json", format: "json",
			model: "model", effort: "effortLevel", effortLevel: copilot.Adapter{}.SettingsEffortLevels,
			reserved: map[string]string{"subagents": "sync --scope user writes per-agent effort from agent specs"},
		},
	},
	"cline": {
		hooksDir:       userPathHome + ".cline/hooks",
		hookScriptsDir: userPathHome + ".cline/hook-scripts",
		rootEnv:        "CLINE_DIR",
		root:           userPathHome + ".cline",
		agents:         userPathHome + ".cline/agents",
		instructions:   userPathHome + ".agents/AGENTS.md",
		skills:         userPathHome + ".cline/skills",
	},
	"windsurf": {
		agents:        userPathHome + ".config/devin/agents",
		agentsWindows: "devin/agents",
		instructions:  userPathXDG + "devin/AGENTS.md",
		skills:        userPathHome + ".agents/skills",
		// Devin also reads hooks and permissions here; sync owns only the
		// permission mode.
		settings: userSettingsFile{
			path: userPathHome + ".config/devin/config.json", windowsPath: userPathAppData + "devin/config.json", format: "json", toolOwned: true,
			allowed:       map[string]string{"permission_mode": "enum:normal|accept-edits|smart|dangerous"},
			allowedReason: "sync --scope user writes only permission_mode to Devin's config.json",
		},
	},
	"amp": {
		instructions: userPathXDG + "amp/AGENTS.md",
		skills:       userPathHome + ".agents/skills",
	},
	"zed": {
		instructionsRoot:    zedInstructionsRoot,
		instructions:        userPathXDG + "zed/AGENTS.md",
		instructionsWindows: "Zed/AGENTS.md",
		skills:              userPathHome + ".agents/skills",
	},
	"warp": {
		mcp:          userMCPFile{path: userPathHome + ".warp/.mcp.json", format: "json", key: "mcpServers"},
		instructions: userPathHome + ".agents/AGENTS.md",
		skills:       userPathHome + ".agents/skills",
		settings: userSettingsFile{
			path:        userPathHome + ".warp/settings.toml",
			linuxPath:   userPathXDG + "warp-terminal/settings.toml",
			windowsPath: userPathLocalAppData + "warp/Warp/config/settings.toml",
			format:      tableTOMLFormat, private: true,
			voice: &userVoiceKeys{
				enabled: "agents.voice.voice_input_enabled", language: "agents.voice.voice_input_language",
				notes:         map[string]string{"mode": "Warp has no voice capture mode setting", "language": "Warp takes a lowercase two-letter language code or an empty string for auto-detect; other voice languages are not written"},
				languageValid: warpVoiceLanguage,
			},
			allowed:       map[string]string{"agents.voice.voice_input_enabled": "bool", "agents.voice.voice_input_language": "voice-language"},
			allowedReason: "sync --scope user writes only Warp's voice enabled and language keys; keep other settings in Warp",
			reserved:      map[string]string{"mcp_servers": "sync --scope user writes MCP servers from MCP specs", "permissions": "sync --scope user does not write Warp user permission rules"},
		},
	},
	"opencode": {
		agents:       userPathXDG + "opencode/agents",
		instructions: userPathXDG + "opencode/AGENTS.md",
		skills:       userPathXDG + "opencode/skills",
	},
	"antigravity": {
		mcp:          userMCPFile{path: userPathHome + ".gemini/config/mcp_config.json", format: "json", key: "mcpServers"},
		agents:       userPathHome + ".gemini/config/agents",
		instructions: userPathHome + ".gemini/GEMINI.md",
		// antigravity.google/docs/skills?tab=ide rows the global scope
		// as "`~/.gemini/config/skills/<skill-folder>/` | Global (all
		// workspaces; legacy `~/.gemini/antigravity/skills/` is also
		// supported)", and the Antigravity app section names the config
		// path with no legacy alternative at all. The legacy tree still
		// loads in the IDE, but the Antigravity app on the same
		// machine reads only the config path
		// (target-audit 2026-09-19, #896).
		skills: userPathHome + ".gemini/config/skills",
	},
	"junie": {
		rootEnv:      "JUNIE_HOME",
		root:         userPathHome + ".junie",
		agents:       userPathHome + ".junie/agents",
		instructions: userPathHome + ".junie/AGENTS.md",
		skills:       userPathHome + ".junie/skills",
	},
	"kiro": {
		settings: userSettingsFile{
			path: userPathHome + ".kiro/settings/cli.json", format: flatJSONFormat,
			voice: &userVoiceKeys{language: "voice.language", notes: map[string]string{
				"enabled": "Kiro CLI has no dictation on/off setting; start voice input with /voice or hold Space",
				"mode":    "Kiro CLI has no voice capture mode setting",
			}},
			reserved: map[string]string{
				"hooks":      "sync --scope user writes hooks from hook specs",
				"mcpServers": "sync --scope user writes MCP servers from MCP specs",
			},
		},
		rootEnv:        "KIRO_HOME",
		root:           userPathHome + ".kiro",
		agents:         userPathHome + ".kiro/agents",
		instructions:   userPathHome + ".kiro/steering/AGENTS.md",
		skills:         userPathHome + ".kiro/skills",
		hooksDir:       userPathHome + ".kiro/hooks",
		hookScriptsDir: userPathHome + ".kiro/scripts",
	},
	"crush": {
		instructions: userPathXDG + "crush/CRUSH.md",
		skills:       userPathXDG + "crush/skills",
	},
	"factory": {
		agents:       userPathHome + ".factory/droids",
		instructions: userPathHome + ".factory/AGENTS.md",
		skills:       userPathHome + ".factory/skills",
	},
	"kilo": {
		agents:       userPathXDG + "kilo/agents",
		instructions: userPathXDG + "kilo/AGENTS.md",
		skills:       userPathHome + ".kilo/skills",
	},
	"goose": {
		agents:       userPathHome + ".agents/agents",
		instructions: userPathXDG + "goose/.goosehints",
		skills:       userPathHome + ".agents/skills",
		// Goose loads each directory under ~/.agents/plugins as a plugin
		// and reads its hooks/hooks.json (Paths::plugins_dir, v1.59.0).
		hooksDir:       userPathHome + ".agents/plugins/agnostic-ai-global/hooks",
		hookScriptsDir: userPathHome + ".agents/plugins/agnostic-ai-global/hooks",
	},
	"openhands": {
		// OpenHands-CLI locations.py reads OPENHANDS_PERSISTENCE_DIR, else
		// ~/.openhands, for mcp.json; the .agents trees do not move.
		rootEnv: "OPENHANDS_PERSISTENCE_DIR",
		root:    userPathHome + ".openhands",
		agents:  userPathHome + ".agents/agents",
		skills:  userPathHome + ".agents/skills",
		// Only the unmaintained OpenHands-CLI reads mcp.json. Agent Canvas,
		// Cloud, and the SDK keep MCP servers in their settings store
		// (docs.openhands.dev/openhands/usage/settings/mcp-settings, #2576).
		mcp: userMCPFile{
			path: userPathHome + ".openhands/mcp.json", format: "json", key: "mcpServers",
			retired: openhands.MCPSettingsHint,
		},
	},
	"trae": {
		agents: userPathHome + ".trae-cn/agents",
		skills: userPathHome + ".trae/skills",
	},
	"augment": {
		agents: userPathHome + ".augment/agents",
		rules:  userPathHome + ".augment/rules",
		skills: userPathHome + ".augment/skills",
		// ~/.augment/settings.json is the user tier of the same file,
		// read by the CLI, VS Code, and IntelliJ (docs.augmentcode.com/
		// cli/hooks, #1253).
		hooks:       userPathHome + ".augment/settings.json",
		hooksFormat: "augment",
		mcp:         userMCPFile{path: userPathHome + ".augment/settings.json", format: "json", key: "mcpServers"},
		settings: userSettingsFile{
			path: userPathHome + ".augment/settings.json", format: "json",
			model: "defaultModel", effort: "defaultThinkingLevel", effortLevel: augment.Adapter{}.SettingsEffortLevels,
			reserved: map[string]string{
				"hooks":           "sync --scope user writes hooks from hook specs",
				"mcpServers":      "sync --scope user writes MCP servers from MCP specs",
				"toolPermissions": "sync --scope user does not write user-level permissions",
			},
		},
	},
}

// qoderUserEffortLevels are the model.reasoningEffort values Qoder CLI
// documents for its user settings (docs.qoder.com/cli/settings-reference).
func qoderUserEffortLevels() []string {
	return []string{"disabled", "off", "none", "low", "medium", "high", "xhigh", "max"}
}

// userTargetNames returns every target sync --scope user supports, sorted.
func userTargetNames() []string {
	out := make([]string, 0, len(userTargets))
	for name := range userTargets {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// userKindSupport maps each kind the global layers load to the targets
// with a user-level surface for it.
func userKindSupport() kindSupport {
	support := kindSupport{spec.KindAgent: {}, spec.KindSkill: {}, spec.KindRule: {}, spec.KindHook: {}, spec.KindSettings: {}, spec.KindMCP: {}}
	for name, g := range userTargets {
		for kind, surface := range map[spec.Kind]bool{
			spec.KindAgent:    g.agents != "",
			spec.KindSkill:    g.skills != "",
			spec.KindRule:     g.instructions != "" || g.rules != "",
			spec.KindHook:     g.hooks != "" || g.hooksDir != "",
			spec.KindSettings: g.settings.path != "",
			spec.KindMCP:      g.mcp.path != "" && g.mcp.retired == "",
		} {
			if surface {
				support[kind][name] = struct{}{}
			}
		}
	}
	return support
}

// userHookTargets keeps the targets sync --scope user writes hooks for,
// sorted.
func userHookTargets(targets []string) []string {
	var out []string
	for _, name := range slices.Sorted(slices.Values(targets)) {
		if userTargets[name].hooks != "" || userTargets[name].hooksDir != "" {
			out = append(out, name)
		}
	}
	return out
}

// userLevelPath resolves one table path against the user home.
func userLevelPath(home, p string) string {
	if rest, ok := strings.CutPrefix(p, userPathAppData); ok {
		root := os.Getenv("APPDATA")
		if !filepath.IsAbs(root) {
			root = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(root, filepath.FromSlash(rest))
	}
	if rest, ok := strings.CutPrefix(p, userPathLocalAppData); ok {
		root := os.Getenv("LOCALAPPDATA")
		if !filepath.IsAbs(root) {
			root = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(root, filepath.FromSlash(rest))
	}
	if rest, ok := strings.CutPrefix(p, userPathXDG); ok {
		root := os.Getenv("XDG_CONFIG_HOME")
		if !filepath.IsAbs(root) {
			root = filepath.Join(home, ".config")
		}
		return filepath.Join(root, filepath.FromSlash(rest))
	}
	return filepath.Join(home, filepath.FromSlash(strings.TrimPrefix(p, userPathHome)))
}

// userRoots returns every directory a global path can resolve under:
// the home, and each configuration root variable that is set.
func userRoots(home string) []string {
	return userRootsForOS(home, runtime.GOOS)
}

func userRootsForOS(home, goos string) []string {
	roots := []string{filepath.Clean(home)}
	for _, env := range []string{"XDG_CONFIG_HOME", "APPDATA", "LOCALAPPDATA"} {
		if root := os.Getenv(env); filepath.IsAbs(root) {
			roots = append(roots, filepath.Clean(root))
		}
	}
	for _, name := range userTargetNames() {
		if resolve := userTargets[name].instructionsRoot; resolve != nil {
			if root, err := resolve(home, goos); err == nil && filepath.IsAbs(root) {
				roots = append(roots, root)
			}
		}
		if root := userTargets[name].overrideRoot(home); filepath.IsAbs(root) {
			roots = append(roots, filepath.Clean(root))
		}
	}
	return roots
}

// trees returns the target's managed directory surfaces, resolved.
// Recorded files under these trees are swept when their source goes away.
func (g userTarget) trees(home string) []string {
	var out []string
	if g.hooks != "" {
		out = append(out, filepath.Join(filepath.Dir(g.path(home, g.hooks)), "hooks"))
	}
	for _, p := range []string{g.skills, g.rules, g.hooksDir, g.hookScriptsDir} {
		if p != "" {
			out = append(out, g.path(home, p))
		}
	}
	if dir := g.agentsPath(home); dir != "" {
		out = append(out, dir)
	}
	return out
}

func (g userTarget) agentsPath(home string) string {
	if g.agents == "" {
		return ""
	}
	if runtime.GOOS == "windows" && g.agentsWindows != "" {
		return userLevelPath(home, userPathAppData+g.agentsWindows)
	}
	return g.path(home, g.agents)
}

// path resolves one of the target's table paths, honoring rootEnv.
func (g userTarget) path(home, p string) string {
	return g.pathForOS(home, p, runtime.GOOS)
}

func (g userTarget) pathForOS(home, p, goos string) string {
	if p != "" && p == g.instructions && g.instructionsRoot != nil {
		if root, err := g.instructionsRoot(home, goos); err == nil && root != "" {
			return filepath.Join(root, filepath.Base(filepath.FromSlash(p)))
		}
	}
	if goos == "windows" && p != "" && p == g.instructions && g.instructionsWindows != "" {
		p = userPathAppData + g.instructionsWindows
	}
	if p != "" && p == g.settings.path && g.settings.resolve != nil {
		return g.settings.resolve(home)
	}
	if p != "" && p == g.settings.path {
		p = g.settings.platformPath(goos)
	}
	if g.rootEnv != "" {
		if root := g.overrideRoot(home); root != "" {
			if rest, ok := strings.CutPrefix(p, g.root); ok && (rest == "" || rest[0] == '/') {
				return filepath.Join(root, filepath.FromSlash(g.rootSubdir), filepath.FromSlash(rest))
			}
		}
	}
	return userLevelPath(home, p)
}

// rootError rejects roots whose ownership would depend on the working directory.
func (g userTarget) rootError(target string) error {
	if err := g.instructionsRootError(runtime.GOOS); err != nil {
		return fmt.Errorf("%s: %w", target, err)
	}
	if err := g.settings.platformRootError(runtime.GOOS); err != nil {
		return fmt.Errorf("%s: %w", target, err)
	}
	if g.rootEnv == "" || g.rootEnv == "KIRO_HOME" {
		return nil
	}
	if root := os.Getenv(g.rootEnv); root != "" && !filepath.IsAbs(root) {
		return fmt.Errorf("%s: %s=%q must be an absolute path", target, g.rootEnv, root)
	}
	return nil
}

// filesForOS returns the target's single-file surfaces, resolved, including
// the bridge script when it has one. Ownership of these is by exact
// path, since each sits in a directory the tool also uses for its own
// unmanaged configuration.
func (g userTarget) filesForOS(home, goos string) []string {
	var out []string
	for _, p := range []string{g.instructions, g.hooks} {
		if p != "" {
			out = append(out, g.pathForOS(home, p, goos))
		}
	}
	if legacy := g.legacyInstructionsPath(home, goos); legacy != "" {
		out = append(out, legacy)
	}
	if g.bridge {
		out = append(out, userBridgePath(filepath.Dir(g.path(home, g.hooks))))
	}
	return out
}

func (g userTarget) legacyInstructionsPath(home, goos string) string {
	if g.instructionsRoot != nil {
		legacy := userLevelPath(home, g.instructions)
		if filepath.IsAbs(legacy) && legacy != g.pathForOS(home, g.instructions, goos) {
			return legacy
		}
	}
	if goos == "windows" && g.instructionsWindows != "" {
		return userLevelPath(home, g.instructions)
	}
	return ""
}

func (g userTarget) instructionsRootError(goos string) error {
	if g.instructionsRoot != nil {
		if _, err := g.instructionsRoot("", goos); err != nil {
			return err
		}
	}
	if goos == "windows" && g.instructionsWindows != "" {
		if root := os.Getenv("APPDATA"); root != "" && !filepath.IsAbs(root) {
			return fmt.Errorf("APPDATA=%q must be an absolute path", root)
		}
	}
	return nil
}

// cursorCLIConfigPath is where cursor-agent reads cli-config.json:
// CURSOR_CONFIG_DIR, else $XDG_CONFIG_HOME/cursor, else ~/.cursor.
func cursorCLIConfigPath(home string) string {
	if dir := strings.TrimSpace(os.Getenv("CURSOR_CONFIG_DIR")); dir != "" {
		return filepath.Join(dir, "cli-config.json")
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, "cursor", "cli-config.json")
	}
	return filepath.Join(home, ".cursor", "cli-config.json")
}
