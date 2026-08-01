package claude

import "github.com/chemaclass/agnostic-ai/internal/config"

// buildConfigSettings renders the first-class
// `outputs.claude.settings.*` fields into the map shape that
// `.claude/settings.json` expects. Returns an empty map when no
// settings are configured so the caller can layer it unconditionally.
//
// Each top-level key is set only when the corresponding config field
// is non-zero, so the resulting map can be merged on top of the
// captured overlay without overwriting keys the user did not declare.
func buildConfigSettings(cfg *config.Config) map[string]any {
	out := map[string]any{}
	if cfg == nil {
		return out
	}
	o, ok := cfg.Outputs[target]
	if !ok || o.Settings == nil {
		return out
	}
	s := o.Settings
	if s.Model != "" {
		out["model"] = s.Model
	}
	if s.OutputStyle != "" {
		out["outputStyle"] = s.OutputStyle
	}
	if s.APIKeyHelper != "" {
		out["apiKeyHelper"] = s.APIKeyHelper
	}
	if s.CleanupPeriodDays != nil {
		out["cleanupPeriodDays"] = *s.CleanupPeriodDays
	}
	if s.IncludeCoAuthoredBy != nil {
		out["includeCoAuthoredBy"] = *s.IncludeCoAuthoredBy
	}
	if len(s.EnabledPlugins) > 0 {
		plugins := map[string]any{}
		for k, v := range s.EnabledPlugins {
			plugins[k] = v
		}
		out["enabledPlugins"] = plugins
	}
	if len(s.Env) > 0 {
		env := map[string]any{}
		for k, v := range s.Env {
			env[k] = v
		}
		out["env"] = env
	}
	if s.StatusLine != nil {
		if sl := statusLineMap(s.StatusLine); len(sl) > 0 {
			out["statusLine"] = sl
		}
	}
	if s.Permissions != nil {
		if p := permissionsMap(s.Permissions); len(p) > 0 {
			out["permissions"] = p
		}
	}
	return out
}

func statusLineMap(s *config.ClaudeStatusLine) map[string]any {
	m := map[string]any{}
	if s.Type != "" {
		m["type"] = s.Type
	}
	if s.Command != "" {
		m["command"] = s.Command
	}
	if s.Padding != nil {
		m["padding"] = *s.Padding
	}
	return m
}

func permissionsMap(p *config.ClaudePermissions) map[string]any {
	m := map[string]any{}
	if len(p.Allow) > 0 {
		m["allow"] = stringSliceToAny(p.Allow)
	}
	if len(p.Deny) > 0 {
		m["deny"] = stringSliceToAny(p.Deny)
	}
	if len(p.Ask) > 0 {
		m["ask"] = stringSliceToAny(p.Ask)
	}
	return m
}

// stringSliceToAny converts a typed string slice into a slice of any.
// JSON encoding handles []string fine, but downstream merge helpers
// inspect map values with type switches that only know []any.
func stringSliceToAny(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}
