package cli

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// lintDevCommands flags an environment spec's dev command that no target
// can start (LINT016, error): an entry that is not a mapping, has no
// `name` or `command`, or repeats another entry's name. Claude Code drops
// such an entry from launch.json, and `sync` passes, so nothing else says
// the preview server is gone.
func lintDevCommands(envs []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, e := range envs {
		list, ok := e.Meta["dev-commands"].([]any)
		if !ok {
			if e.Meta["dev-commands"] != nil {
				out = append(out, devCommandFinding(e, "`dev-commands` must be a list of commands"))
			}
			continue
		}
		seen := map[string]bool{}
		for i, item := range list {
			m, ok := item.(map[string]any)
			if !ok {
				out = append(out, devCommandFinding(e, fmt.Sprintf("dev command %d is not a mapping with `name:` and `command:`", i+1)))
				continue
			}
			name, _ := m["name"].(string)
			switch {
			case name == "":
				out = append(out, devCommandFinding(e, fmt.Sprintf("dev command %d has no `name:`", i+1)))
			case seen[name]:
				out = append(out, devCommandFinding(e, fmt.Sprintf("dev command %q appears twice; names must be unique", name)))
			}
			seen[name] = true
			if !hasDevCommand(m["command"]) {
				out = append(out, devCommandFinding(e, fmt.Sprintf("dev command %d has no `command:`", i+1)))
			}
		}
	}
	return out
}

func hasDevCommand(v any) bool {
	switch t := v.(type) {
	case string:
		return t != ""
	case []any:
		return len(t) > 0
	}
	return false
}

func devCommandFinding(e spec.Entry, msg string) lintFinding {
	return lintFinding{Code: "LINT016", Severity: lintError, Path: e.Path, Message: msg}
}
