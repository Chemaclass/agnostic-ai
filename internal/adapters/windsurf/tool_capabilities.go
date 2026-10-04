package windsurf

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

var toolCapabilities = emit.ToolCapabilityTable{
	"read":  {Agent: []string{"read"}, Permission: []string{"read"}},
	"Grep":  {Agent: []string{"grep"}, Permission: []string{"grep"}},
	"Glob":  {Agent: []string{"glob"}, Permission: []string{"glob"}},
	"shell": {Agent: []string{"exec"}, Permission: []string{"exec"}},
	"write": {Agent: []string{"write"}, Permission: []string{"write"}},
	"edit":  {Agent: []string{"edit"}, Permission: []string{"edit"}},
	"web":   {Permission: []string{"", "web_search"}},
}

func (Adapter) ToolCapabilities() emit.ToolCapabilityTable { return toolCapabilities }

func (Adapter) TranslatePermission(list, rule string) ([]string, bool) {
	native, ok := devinPermissionRule(rule, list)
	if !ok {
		return nil, false
	}
	return []string{native}, true
}

func reportPermissionWidening(settings []spec.Entry, mode string) error {
	for _, entry := range settings {
		rules, native := entryRules(entry, "deny")
		if native {
			continue
		}
		for _, rule := range rules {
			scope, arg, scoped := spec.SplitPermissionRule(rule)
			if !scoped || scope != "Bash" || strings.HasSuffix(arg, ":*") || strings.ContainsAny(arg, "*?") {
				continue
			}
			if _, ok := devinPermissionRule(rule, "deny"); !ok {
				continue
			}
			line := fmt.Sprintf("permissions.deny rule %s becomes Devin's Exec prefix, which also blocks commands with extra arguments", rule)
			if err := emit.ReportWidening(target, entry.Path, line, mode); err != nil {
				return err
			}
		}
	}
	return nil
}

func (Adapter) TranslatePermissionContext(list, rule string, entry spec.Entry, _ []spec.Entry, _ *config.Config) emit.CapabilityTranslation {
	if custom, overridden := emit.SettingsCustomObject(entry, target, permissionsKey); overridden {
		body, _ := json.Marshal(custom[list])
		return emit.CapabilityTranslation{Native: []string{list + ": " + string(body)}, Supported: true, Override: "x-windsurf.permissions"}
	}
	native, ok := (Adapter{}).TranslatePermission(list, rule)
	result := emit.CapabilityTranslation{Native: native, Supported: ok}
	if scope, arg, scoped := spec.SplitPermissionRule(rule); scoped && scope == "Bash" && list == "deny" && !strings.HasSuffix(arg, ":*") && !strings.ContainsAny(arg, "*?") && ok {
		result.Widening = []string{"Devin's Exec prefix also blocks commands with extra arguments"}
	}
	return result
}
