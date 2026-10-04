package cli

import (
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// specKeys lists every frontmatter key agnostic-ai or an adapter reads on
// agents, skills, rules, hooks, MCP servers, commands, reviews, and ignore
// files. It only decides what counts as a typo (see keyTypo), so a key
// read by a single target belongs here too. The spec-format page's field
// tables are checked against it in tests.
var specKeys = []string{
	// Shared.
	"name", "description", "scope", "target", "targets", "target-exclude", "targets-exclude",
	// Agents and skills.
	"tools", "model", "effort", "color", "readonly", "memory", "mcpServers", "permissionMode",
	"hooks", "disable-model-invocation", "argument-hint", "reasoningEffort", "temperature",
	"nickname_candidates", "permission", "agent", "subtask",
	"license", "workspaces",
	// Rules.
	"globs", "paths", "alwaysApply",
	// Hooks.
	"event", "matcher", "command", "args", "type", "timeout", "disabled", "async", "asyncRewake",
	"shell", "if", "commandWindows", "additionalContextLimit", "continueOnBlock", "failClosed",
	"loop_limit", "prompt", "server", "tool", "input", "statusMessage", "sequential",
	// MCP servers.
	"url", "headers", "allowedEnvVars", "env", "cwd", "oauth", "roots", "auth", "trust", "version",
	"alwaysAllow", "alwaysLoad", "bareElicitationCapability", "api_key", "bearer_token_env_var", "connectionTimeout",
	"default_tools_approval_mode", "disabled_tools", "enabled_tools", "env_http_headers",
	"env_vars", "excludeTools", "includeTools", "http_headers_helper", "oauth_callback_port",
	"oauth_client_id", "oauth_client_secret", "oauth_resource", "oauthClientId",
	"oauthClientSecret", "oauthResource", "requestOptions", "required", "sandboxEnabled",
	"scopes", "sessionless", "startup_timeout_ms", "startup_timeout_sec", "tool_timeout_sec",
	"experimental_environment", "autoApprove", "disabledTools", "oauthScopes", "connectTimeout",
	"headersHelper", "envFile", "dev",
}

// hookKeys are keys only hook specs read. Short keys such as `on:` sit
// one edit from many words, so other kinds do not count them.
var hookKeys = []string{"on", "match", "decision"}

// kindKeys lists the keys a spec of kind reads.
func kindKeys(kind spec.Kind) []string {
	if kind == spec.KindHook {
		return append(slices.Clone(specKeys), hookKeys...)
	}
	return specKeys
}

// targetKeys are top-level keys only some targets read. On a project that
// configures none of them they do nothing, so one that is a near miss of
// a portable key still gets flagged there: `glob:` meant as `globs:`.
var targetKeys = map[string][]string{
	"glob": {"qoder"},
	"mode": {"kilo", "opencode"},
}
