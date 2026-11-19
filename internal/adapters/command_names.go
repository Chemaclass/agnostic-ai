package adapters

import (
	"maps"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// commandNamespacer is the optional interface an adapter implements when
// its tool turns a `group:name` command into a `group/name` file, as
// Gemini CLI and Qwen Code do. Every other tool reads the name from the
// filename, where a colon breaks a Windows checkout and the skill name
// rule.
type commandNamespacer interface {
	NamespacedCommands() bool
}

// withFlatCommandNames renames each `group:name` command to `group-name`
// for a target without command namespaces. A command already named
// `group-name` keeps that name, and the namespaced one stays as it is.
func withFlatCommandNames(b spec.Bundle, target string) spec.Bundle {
	if n, ok := registry[target].(commandNamespacer); ok && n.NamespacedCommands() {
		return b
	}
	taken := map[string]bool{}
	for _, c := range b.Commands {
		taken[c.Name] = true
	}
	var commands []spec.Entry
	for i, c := range b.Commands {
		flat := strings.ReplaceAll(c.Name, ":", "-")
		if flat == c.Name || taken[flat] {
			continue
		}
		if commands == nil {
			commands = append([]spec.Entry{}, b.Commands...)
		}
		taken[flat] = true
		if c.Meta["name"] == c.Name {
			c.Meta = maps.Clone(c.Meta)
			c.Meta["name"] = flat
		}
		c.Name = flat
		commands[i] = c
	}
	if commands != nil {
		b.Commands = commands
	}
	return b
}
