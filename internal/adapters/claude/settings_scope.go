package claude

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	userOrManagedReason = "Claude Code ignores it in project settings; set it in a home settings spec and run `sync --global`, or in managed settings"
	userOnlyReason      = "Claude Code reads it from user settings only; set it in a home settings spec and run `sync --global`"
	managedOnlyReason   = "Claude Code reads it from managed settings only"
)

type settingScope int

const (
	userOrManaged settingScope = iota
	userOnly
	managedOnly
)

func (s settingScope) reason() string {
	switch s {
	case userOnly:
		return userOnlyReason
	case managedOnly:
		return managedOnlyReason
	}
	return userOrManagedReason
}

// projectIgnoredKey is a settings.json key Claude Code ignores in a
// project's .claude/settings.json. A non-nil ignored names the one value
// it ignores there; nil means every value.
type projectIgnoredKey struct {
	path    string
	scope   settingScope
	ignored any
}

// Scope column of the code.claude.com/docs/en/settings-reference index, fetched 2026-10-27.
var projectIgnoredKeys = []projectIgnoredKey{
	{path: "allowAllClaudeAiMcps", scope: managedOnly},
	{path: "allowClaudeInChromeWithManagedMcp", scope: managedOnly},
	{path: "allowedChannelPlugins", scope: managedOnly},
	{path: "allowedProviders", scope: managedOnly},
	{path: "allowManagedHooksOnly", scope: managedOnly},
	{path: "allowManagedMcpServersOnly", scope: managedOnly},
	{path: "allowManagedPermissionRulesOnly", scope: managedOnly},
	{path: "appendPlugins"},
	{path: "askUserQuestionTimeout"},
	{path: "autoContinueAtUsageLimit"},
	{path: "autoMode"},
	{path: "availableModelsMatch", scope: managedOnly},
	{path: "axScreenReader"},
	{path: "bashEditDiffEnabled"},
	{path: "blockedMarketplaces", scope: managedOnly},
	{path: "browserExternalPageTools", scope: managedOnly},
	{path: "channelsEnabled", scope: managedOnly},
	{path: "claudeMd", scope: managedOnly},
	{path: "deniedModels", scope: managedOnly},
	{path: "desktopSessionCleanupPeriodDays"},
	{path: "dialogExpiry"},
	{path: "disableBrowserExternalNavigation", scope: managedOnly},
	{path: "disableCommandPluginSources", scope: managedOnly},
	{path: "disableDesktopLocalSessions", scope: managedOnly},
	{path: "disableMobileSimulatorTools", scope: managedOnly},
	{path: "disableSideloadFlags", scope: managedOnly},
	{path: "feedbackDrafts"},
	{path: "footerLinksRegexes"},
	{path: "forceLoginGatewayUrl"},
	{path: "forceRemoteSettingsRefresh", scope: managedOnly},
	{path: "gatewayInternalNetworks", scope: managedOnly},
	{path: "managedMcpServers", scope: managedOnly},
	{path: "managedSourcesBehavior", scope: managedOnly},
	{path: "modelPicker"},
	{path: "modelPricing", scope: managedOnly},
	{path: "parentSettingsBehavior", scope: managedOnly},
	{path: "pluginConfigs"},
	{path: "pluginSuggestionMarketplaces", scope: managedOnly},
	{path: "pluginTrustMessage", scope: managedOnly},
	{path: "policyHelper", scope: managedOnly},
	{path: "prependPlugins"},
	{path: "processWrapper"},
	{path: "requiredMaximumVersion", scope: managedOnly},
	{path: "requiredMinimumVersion", scope: managedOnly},
	{path: "sandbox.allowAppleEvents"},
	{path: "sandbox.allowNestedUserNamespaces", ignored: true},
	{path: "sandbox.bwrapPath", scope: managedOnly},
	{path: "sandbox.credentials.allowPlaintextInject"},
	{path: "sandbox.credentials.awsPairs"},
	{path: "sandbox.credentials.sigv4"},
	{path: "sandbox.filesystem.allowManagedReadPathsOnly", scope: managedOnly},
	{path: "sandbox.filesystem.disabled"},
	{path: "sandbox.network.allowManagedDomainsOnly", scope: managedOnly},
	{path: "sandbox.network.strictAllowlist"},
	{path: "sandbox.network.tlsTerminate"},
	{path: "sandbox.ripgrep"},
	{path: "sandbox.socatPath", scope: managedOnly},
	{path: "skipAutoPermissionPrompt"},
	{path: "skipDangerousModePermissionPrompt", ignored: true},
	{path: "spellcheck"},
	{path: "sshConfigs"},
	{path: "sshHostAllowlist", scope: managedOnly},
	{path: "sshHostCommands"},
	{path: "strictKnownMarketplaces", scope: managedOnly},
	{path: "strictPluginOnlyCustomization", scope: managedOnly},
	{path: "syncClaudeAiPlugins", ignored: false},
	{path: "syncClaudeAiSkills", ignored: false},
	{path: "useAutoModeDuringPlan", ignored: false},
	{path: "vimInsertModeRemaps"},
	{path: "worktree.location", scope: userOnly},
	{path: "wslInheritsWindowsSettings", scope: managedOnly},
}

type projectIgnoredSetting struct {
	field  string
	count  int
	reason string
}

// projectIgnoredSettings names each key in projectIgnoredKeys that the
// merged x-claude block custom writes, counting the specs whose own value
// is the one written. No portable field maps to one.
func projectIgnoredSettings(entries []spec.Entry, custom map[string]any) []projectIgnoredSetting {
	var out []projectIgnoredSetting
	for _, key := range projectIgnoredKeys {
		written, ok := settingAt(custom, key.path)
		if !ok || (key.ignored != nil && written != key.ignored) {
			continue
		}
		count := 0
		for _, entry := range entries {
			own, _ := emit.CustomTargetMeta(entry.Meta, target)
			if v, ok := settingAt(own, key.path); ok && reflect.DeepEqual(v, written) {
				count++
			}
		}
		field := "x-" + target + "." + key.path
		if key.ignored != nil {
			field += fmt.Sprintf(": %v", key.ignored)
		}
		out = append(out, projectIgnoredSetting{field: field, count: count, reason: key.scope.reason()})
	}
	return out
}

// noteProjectIgnoredSettings keeps each key in the file and names it in a note.
func noteProjectIgnoredSettings(entries []spec.Entry, custom map[string]any) {
	for _, s := range projectIgnoredSettings(entries, custom) {
		emit.NoteFieldNoOp(target, spec.KindSettings, s.field, s.count, s.reason)
	}
}

// UserIgnoredSetting returns the managed-only key that a dotted user
// settings key sits at or under, with the reason Claude Code ignores it.
func UserIgnoredSetting(key string) (field, reason string, ok bool) {
	for _, k := range projectIgnoredKeys {
		if k.scope == managedOnly && (key == k.path || strings.HasPrefix(key, k.path+".")) {
			return k.path, managedOnlyReason, true
		}
	}
	return "", "", false
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
