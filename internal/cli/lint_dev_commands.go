package cli

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// lintDevCommands flags an environment spec's dev command Claude Code
// cannot start as written (LINT016, error): an entry that is not a
// mapping, has no `name` or `command`, repeats another entry's name, or
// sets a `port` that is not a number or an `env` value that is not a
// scalar. Claude Code drops or rewrites such an entry in launch.json, and
// `sync` passes, so nothing else says the preview server changed. The
// list is read as Claude Code resolves it, `x-claude` included.
func lintDevCommands(envs []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, e := range envs {
		meta := adapters.ResolveMeta(e.Meta, "claude")
		value, present := meta["dev-commands"]
		list, ok := value.([]any)
		if !ok {
			switch {
			case present && value == nil:
				out = append(out, devCommandFinding(e, "`dev-commands` is empty; write `dev-commands: []` to clear an earlier spec's list"))
			case present:
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
			for _, k := range slices.Sorted(maps.Keys(m)) {
				want, known := devCommandFieldTypes[k]
				if !known {
					out = append(out, devCommandFinding(e, fmt.Sprintf("dev command %d sets `%s`, which no target reads", i+1, k)))
					continue
				}
				if !want(m[k]) {
					out = append(out, devCommandFinding(e, fmt.Sprintf("dev command %d sets `%s` to a value of the wrong type", i+1, k)))
				}
			}
			if len(adapters.CommandArgv(m["command"])) == 0 {
				out = append(out, devCommandFinding(e, fmt.Sprintf("dev command %d has no `command:`", i+1)))
			}
			if port, ok := m["port"]; ok && !isPortValue(port) {
				out = append(out, devCommandFinding(e, fmt.Sprintf("dev command %d sets `port: %v`; use a number", i+1, port)))
			}
			env, isMap := m["env"].(map[string]any)
			if m["env"] != nil && !isMap {
				out = append(out, devCommandFinding(e, fmt.Sprintf("dev command %d sets `env` to something other than a mapping of names to values", i+1)))
			}
			for _, k := range slices.Sorted(maps.Keys(env)) {
				switch env[k].(type) {
				case string, int, int64, float64, bool:
				default:
					out = append(out, devCommandFinding(e, fmt.Sprintf("dev command %d sets `env.%s` to a list or mapping; use a string", i+1, k)))
				}
			}
		}
	}
	return out
}

// isPortValue reports whether v is a port number, written as a number or
// a numeric string.
func isPortValue(v any) bool {
	switch t := v.(type) {
	case int, int64:
		return true
	case float64:
		return t == float64(int(t))
	case string:
		_, err := strconv.Atoi(strings.TrimSpace(t))
		return err == nil
	}
	return false
}

func devCommandFinding(e spec.Entry, msg string) lintFinding {
	return lintFinding{Code: "LINT016", Severity: lintError, Path: e.Path, Message: msg}
}

// devCommandFieldTypes checks the value of each dev-command key. command,
// port, and env have their own findings, so any value passes here.
var devCommandFieldTypes = map[string]func(any) bool{
	"name":      func(v any) bool { _, ok := v.(string); return ok },
	"cwd":       func(v any) bool { _, ok := v.(string); return ok },
	"url":       func(v any) bool { _, ok := v.(string); return ok },
	"auto-port": func(v any) bool { _, ok := v.(bool); return ok },
	"icon":      func(v any) bool { _, ok := v.(string); return ok },
	"command":   func(any) bool { return true },
	"port":      func(any) bool { return true },
	"env":       func(any) bool { return true },
}
