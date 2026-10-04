package kiro

import (
	"fmt"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// kiroCategoryTools lists the built-in tools a Kiro category grants
// (kiro.dev/docs/tools/): `write` covers `fs_write`, `fs_append`,
// `str_replace`, and `delete_file`; `web` covers `web_fetch` and
// `web_search`. A category missing here grants nothing beyond what the
// names that select it ask for.
var kiroCategoryTools = map[string][]string{
	"write": {"fs_write", "fs_append", "str_replace", "delete_file"},
	"web":   {"web_fetch", "web_search"},
}

// kiroToolAsks lists the Kiro tools a Claude-style name asks for. Edit
// and Write each change files, as Claude Code's Edit rules cover every
// tool that edits files; neither deletes one.
var kiroToolAsks = map[string][]string{
	"Delete":    {"delete_file"},
	"Edit":      {"fs_write", "fs_append", "str_replace"},
	"Write":     {"fs_write", "fs_append", "str_replace"},
	"WebFetch":  {"web_fetch"},
	"WebSearch": {"web_search"},
}

// widerTools returns one sentence per category that grants more than
// the names selecting it ask for, in first-seen order.
func widerTools(names []string) []string {
	var order []string
	selected := map[string][]string{}
	for _, n := range names {
		category, ok := emit.CapabilityTool(toolCapabilities, n, false)
		if !ok {
			continue
		}
		if _, seen := selected[category]; !seen {
			order = append(order, category)
		}
		selected[category] = append(selected[category], n)
	}
	var out []string
	for _, category := range order {
		asked := map[string]bool{}
		var labels []string
		for _, n := range selected[category] {
			for _, tool := range kiroToolAsks[n] {
				asked[tool] = true
			}
			label := n
			if c, ok := spec.NeutralCapability(n); ok {
				label = c
			}
			if !slices.Contains(labels, label) {
				labels = append(labels, label)
			}
		}
		var extra []string
		for _, tool := range kiroCategoryTools[category] {
			if !asked[tool] {
				extra = append(extra, tool)
			}
		}
		if len(extra) == 0 {
			continue
		}
		verb := "becomes"
		if len(labels) > 1 {
			verb = "become"
		}
		out = append(out, fmt.Sprintf("%s %s Kiro's %s category, which also allows %s",
			strings.Join(labels, " and "), verb, category, strings.Join(extra, " and ")))
	}
	return out
}

func (Adapter) CapabilityWidening(names []string) []string { return widerTools(names) }
