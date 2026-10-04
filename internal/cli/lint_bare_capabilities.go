package cli

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func lintBareCapabilities(settings []spec.Entry, targets []string) []lintFinding {
	var out []lintFinding
	seen := map[string]bool{}
	for _, entry := range settings {
		permissions, _ := entry.Meta["permissions"].(map[string]any)
		for _, list := range []string{"allow", "ask"} {
			for _, rule := range toStringSlice(permissions[list]) {
				scope, alternative := bareCapabilityScope(rule)
				if scope == "" {
					continue
				}
				key := entry.Path + "\x00" + entry.Name + "\x00" + list + "\x00" + rule
				if seen[key] {
					continue
				}
				seen[key] = true
				var effects []string
				for _, target := range targets {
					if !entry.EmitsTo(target) {
						continue
					}
					names := bareCapabilityNativePermissions(rule, list, target)
					names = slices.DeleteFunc(names, func(name string) bool { return barePermissionOverridden(settings, entry, target, name) })
					if len(names) > 0 {
						effects = append(effects, target+": "+strings.Join(names, ", "))
					}
				}
				if len(effects) == 0 {
					continue
				}
				message := fmt.Sprintf("permissions.%s: bare %s now covers %s (%s); %s", list, rule, scope, strings.Join(effects, "; "), alternative)
				out = append(out, lintFinding{Code: "LINT038", Severity: lintWarn, Path: entry.Path, Message: message})
			}
		}
	}
	return out
}

func bareCapabilityScope(rule string) (string, string) {
	switch rule {
	case "shell":
		return "every shell command", "use shell(git status) to name a command"
	case "read":
		return "every file read", "use read(src/**) to name paths"
	case "edit":
		return "every file edit", "use edit(src/**) to name paths"
	case "write":
		return "every file write", "use edit(src/**) to name paths; it also covers edits"
	case "web":
		return "unrestricted native web tools", "use target-native permission fields to restrict web access, or WebFetch(domain:example.com) for fetches on a named domain"
	}
	if server, isMCP := strings.CutPrefix(rule, "mcp:"); isMCP && !strings.Contains(server, "/") {
		if names, problem := spec.PermissionRules("allow", rule); problem == "" && len(names) == 1 {
			return "every tool on MCP server " + server, "use mcp:" + server + "/<tool> to name one tool"
		}
	}
	return "", ""
}

func bareCapabilityNativePermissions(rule, list, target string) []string {
	if target == "cursor" {
		if server, ok := strings.CutPrefix(rule, "mcp:"); ok && list == "allow" {
			if _, problem := spec.PermissionRules(list, rule); problem == "" {
				return []string{"Mcp(" + server + ":*)"}
			}
		}
		return nil
	}
	adapter, registered := adapters.Get(target)
	if !registered || !slices.Contains(adapter.Capabilities(), spec.KindSettings) {
		return nil
	}
	return adapters.TranslatePermissionCapability(target, list, rule, nil).Native

}

func barePermissionOverridden(settings []spec.Entry, entry spec.Entry, target, name string) bool {
	custom, _ := entry.Meta["x-"+target].(map[string]any)
	if target == "windsurf" {
		_, replaced := custom["permissions"].(map[string]any)
		return replaced
	}
	if target == "kilo" {
		if _, replaced := custom["permission"].(map[string]any); replaced {
			return true
		}
	}
	if target == "kilo" || target == "opencode" {
		for _, candidate := range settings {
			if !candidate.EmitsTo(target) {
				continue
			}
			override, _ := candidate.Meta["x-"+target].(map[string]any)
			permissions, _ := override["permission"].(map[string]any)
			if _, replaced := permissions[name]; replaced {
				return true
			}
		}
	}
	return false
}

func warnBareCapabilities(cfg *config.Config, b spec.Bundle, targets []string) {
	warnBareCapabilitiesTo(logOut, cfg, b, targets)
}

func warnBareCapabilitiesTo(out io.Writer, cfg *config.Config, b spec.Bundle, targets []string) {
	if cfg.OnUnsupported == "silent" || verbosity < levelDefault {
		return
	}
	for _, finding := range lintBareCapabilities(b.Settings, targets) {
		_, _ = fmt.Fprintf(out, "%s %s: %s (%s)\n", bang(), finding.Path, finding.Message, finding.Code)
	}
}
