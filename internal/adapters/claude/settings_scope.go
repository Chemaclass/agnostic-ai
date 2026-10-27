package claude

import (
	"fmt"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	userOrManagedReason = "Claude Code ignores it in project settings; set it in a home settings spec and run `sync --global`, or in managed settings"
	managedOnlyReason   = "Claude Code reads it from managed settings only"
)

// projectIgnoredKey is a settings.json key Claude Code ignores in a
// project's .claude/settings.json. A non-nil ignored names the one value
// it ignores there; nil means every value.
type projectIgnoredKey struct {
	path    string
	managed bool
	ignored any
}

// Scope column of the code.claude.com/docs/en/settings-reference index, fetched 2026-10-27.
var projectIgnoredKeys = []projectIgnoredKey{
	{path: "allowAllClaudeAiMcps", managed: true},
	{path: "allowClaudeInChromeWithManagedMcp", managed: true},
	{path: "allowedChannelPlugins", managed: true},
	{path: "allowedProviders", managed: true},
	{path: "allowManagedHooksOnly", managed: true},
	{path: "allowManagedMcpServersOnly", managed: true},
	{path: "allowManagedPermissionRulesOnly", managed: true},
	{path: "appendPlugins"},
	{path: "askUserQuestionTimeout"},
	{path: "autoContinueAtUsageLimit"},
	{path: "autoMode"},
	{path: "availableModelsMatch", managed: true},
	{path: "axScreenReader"},
	{path: "bashEditDiffEnabled"},
	{path: "blockedMarketplaces", managed: true},
	{path: "browserExternalPageTools", managed: true},
	{path: "channelsEnabled", managed: true},
	{path: "claudeMd", managed: true},
	{path: "deniedModels", managed: true},
	{path: "desktopSessionCleanupPeriodDays"},
	{path: "dialogExpiry"},
	{path: "disableBrowserExternalNavigation", managed: true},
	{path: "disableCommandPluginSources", managed: true},
	{path: "disableDesktopLocalSessions", managed: true},
	{path: "disableMobileSimulatorTools", managed: true},
	{path: "disableSideloadFlags", managed: true},
	{path: "feedbackDrafts"},
	{path: "footerLinksRegexes"},
	{path: "forceLoginGatewayUrl"},
	{path: "forceRemoteSettingsRefresh", managed: true},
	{path: "gatewayInternalNetworks", managed: true},
	{path: "managedMcpServers", managed: true},
	{path: "managedSourcesBehavior", managed: true},
	{path: "modelPicker"},
	{path: "modelPricing", managed: true},
	{path: "parentSettingsBehavior", managed: true},
	{path: "pluginConfigs"},
	{path: "pluginSuggestionMarketplaces", managed: true},
	{path: "pluginTrustMessage", managed: true},
	{path: "policyHelper", managed: true},
	{path: "prependPlugins"},
	{path: "processWrapper"},
	{path: "requiredMaximumVersion", managed: true},
	{path: "requiredMinimumVersion", managed: true},
	{path: "sandbox.allowAppleEvents"},
	{path: "sandbox.allowNestedUserNamespaces", ignored: true},
	{path: "sandbox.bwrapPath", managed: true},
	{path: "sandbox.credentials.allowPlaintextInject"},
	{path: "sandbox.credentials.awsPairs"},
	{path: "sandbox.credentials.sigv4"},
	{path: "sandbox.filesystem.allowManagedReadPathsOnly", managed: true},
	{path: "sandbox.filesystem.disabled"},
	{path: "sandbox.network.allowManagedDomainsOnly", managed: true},
	{path: "sandbox.network.strictAllowlist"},
	{path: "sandbox.network.tlsTerminate"},
	{path: "sandbox.ripgrep"},
	{path: "sandbox.socatPath", managed: true},
	{path: "skipAutoPermissionPrompt"},
	{path: "skipDangerousModePermissionPrompt", ignored: true},
	{path: "spellcheck"},
	{path: "sshConfigs"},
	{path: "sshHostAllowlist", managed: true},
	{path: "sshHostCommands"},
	{path: "strictKnownMarketplaces", managed: true},
	{path: "strictPluginOnlyCustomization", managed: true},
	{path: "syncClaudeAiPlugins", ignored: false},
	{path: "syncClaudeAiSkills", ignored: false},
	{path: "useAutoModeDuringPlan", ignored: false},
	{path: "vimInsertModeRemaps"},
	{path: "wslInheritsWindowsSettings", managed: true},
}

type projectIgnoredSetting struct {
	field  string
	count  int
	reason string
}

// projectIgnoredSettings counts the settings specs whose x-claude block
// sets each key in projectIgnoredKeys. No portable field maps to one.
func projectIgnoredSettings(entries []spec.Entry) []projectIgnoredSetting {
	var out []projectIgnoredSetting
	for _, key := range projectIgnoredKeys {
		count := 0
		for _, entry := range entries {
			custom, _ := emit.CustomTargetMeta(entry.Meta, target)
			if v, ok := settingAt(custom, key.path); ok && (key.ignored == nil || v == key.ignored) {
				count++
			}
		}
		if count == 0 {
			continue
		}
		field := "x-" + target + "." + key.path
		if key.ignored != nil {
			field += fmt.Sprintf(": %v", key.ignored)
		}
		reason := userOrManagedReason
		if key.managed {
			reason = managedOnlyReason
		}
		out = append(out, projectIgnoredSetting{field: field, count: count, reason: reason})
	}
	return out
}

// noteProjectIgnoredSettings keeps each key in the file and names it in a note.
func noteProjectIgnoredSettings(entries []spec.Entry) {
	for _, s := range projectIgnoredSettings(entries) {
		emit.NoteFieldNoOp(target, spec.KindSettings, s.field, s.count, s.reason)
	}
}

func settingAt(m map[string]any, path string) (any, bool) {
	var v any = m
	for _, part := range strings.Split(path, ".") {
		obj, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		if v, ok = obj[part]; !ok {
			return nil, false
		}
	}
	return v, true
}
