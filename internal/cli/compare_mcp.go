package cli

import (
	"encoding/json"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

type compareMCPError struct {
	path string
	err  error
}

func (e compareMCPError) Error() string {
	return e.path + ": cannot compare MCP configuration: adapter emission failed"
}

func (e compareMCPError) Unwrap() error { return e.err }

func comparedMCPFields(e spec.Entry, keys, targets []string) []string {
	fields := []string{"type"}
	add := func(meta map[string]any, keys []string) {
		for _, key := range keys {
			switch key {
			case "command", "args", "url":
				if !slices.Contains(fields, key) {
					fields = append(fields, key)
				}
			case "env", "headers":
				entries, _ := meta[key].(map[string]any)
				for _, name := range slices.Sorted(maps.Keys(entries)) {
					field := key + "." + name
					if !slices.Contains(fields, field) {
						fields = append(fields, field)
					}
				}
			}
		}
	}
	add(e.Meta, keys)
	for _, target := range targets {
		if override, ok := e.Meta["x-"+target].(map[string]any); ok {
			add(override, slices.Sorted(maps.Keys(override)))
		}
	}
	return fields
}

func classifyMCPEntry(cfg *config.Config, e spec.Entry, fields []string, target string, adapter adapters.Adapter, base []adapters.CapturedFile, notes []adapters.Note) (map[string]compareResult, []compareNote, error) {
	results := make(map[string]compareResult, len(fields))
	if len(base) == 0 || adapters.OmittedEntry(target, spec.KindMCP, e.Name) {
		result := noMCPOutputResult(notes, target)
		for _, field := range fields {
			results[field] = result
		}
		return results, nil, nil
	}
	var specNotes []compareNote
	for _, note := range notes {
		switch note.Shape {
		case adapters.NoteSurface:
			specNotes = append(specNotes, compareNote{Target: target, Text: "some target surfaces do not read this MCP configuration"})
		case adapters.NoteGap:
			specNotes = append(specNotes, compareNote{Target: target, Text: "the adapter reports a gap in this MCP configuration"})
		}
	}
	for _, field := range fields {
		r := compareResult{Target: target}
		_, noted := fieldNote(notes, field)
		value := mcpCompareValue(e, field, target)
		if field == "type" {
			absent, err := captureEmit(adapter, spec.Bundle{}, cfg)
			adapters.DrainNotes()
			if err != nil {
				r.Status, r.Reason = statusUnknown, "the adapter cannot isolate this server's transport"
				results[field] = r
				continue
			}
			paths, verbatim := mcpChangedKeyPaths(base, absent, field, value, e.Name)
			switch {
			case len(paths) == 0:
				r.Status, r.Reason = statusUnknown, "the native output does not state an explicit transport"
			case verbatim:
				r.Status, r.Paths = statusPreserved, paths
			default:
				r.Status, r.Paths, r.Reason = statusTranslated, paths, "the native output uses another transport spelling"
			}
			results[field] = r
			continue
		}
		projection := mcpFieldVariant(e, field, target, base)
		variant, _, err := captureEntry(adapter, cfg, target, projection)
		if err != nil {
			r.Status, r.Reason = statusUnknown, "the adapter cannot emit a valid comparison for this field"
			results[field] = r
			continue
		}
		changed, landed := fieldEffect(base, variant)
		keyed, verbatim := mcpChangedKeyPaths(base, variant, field, value, e.Name)
		switch {
		case noted && !changed:
			r.Status, r.Reason = statusUnsupported, "the adapter omits or ignores this field's configuration or reference"
		case len(keyed) > 0 && verbatim:
			r.Status, r.Paths = statusPreserved, keyed
		case len(keyed) > 0:
			r.Status, r.Paths, r.Reason = statusTranslated, keyed, "same key, rewritten values"
		case changed:
			r.Status, r.Paths, r.Reason = statusTranslated, landed, "written under another key or shape"
		default:
			r.Status, r.Reason = statusUnsupported, "no native output carries this field"
		}
		results[field] = r
	}
	return results, specNotes, nil
}

func noMCPOutputResult(notes []adapters.Note, target string) compareResult {
	r := compareResult{Target: target, Status: statusUnknown, Reason: "the adapter writes no MCP configuration and gives no reason"}
	for _, note := range notes {
		if note.Shape == adapters.NoteUnsupportedKind {
			r.Status, r.Reason = statusUnsupported, target+" has no native MCP support"
			return r
		}
	}
	for _, note := range notes {
		if note.Shape == adapters.NoteField {
			r.Status, r.Reason = statusUnsupported, "the server is omitted because the adapter cannot write its "+note.Field+" configuration or reference"
			return r
		}
	}
	for _, note := range notes {
		if note.Shape == adapters.NoteGap {
			r.Status, r.Reason = statusUnsupported, "the adapter omits this server's transport or configuration"
			return r
		}
	}
	return r
}

func mcpCompareValue(e spec.Entry, field, target string) any {
	parent, key, nested := strings.Cut(field, ".")
	value := resolvedValue(e, parent, target)
	if nested {
		entries, _ := value.(map[string]any)
		return entries[key]
	}
	if field == "type" && value == nil {
		return "stdio"
	}
	return value
}

func mcpFieldVariant(e spec.Entry, field, target string, files []adapters.CapturedFile) spec.Entry {
	parent, key, nested := strings.Cut(field, ".")
	if nested {
		e.Meta = maps.Clone(e.Meta)
		remove := func(meta map[string]any) {
			if entries, ok := meta[parent].(map[string]any); ok {
				entries = maps.Clone(entries)
				delete(entries, key)
				meta[parent] = entries
			}
		}
		remove(e.Meta)
		if override, ok := e.Meta["x-"+target].(map[string]any); ok {
			override = maps.Clone(override)
			remove(override)
			e.Meta["x-"+target] = override
		}
		return e
	}
	variant := withoutField(e, field, target)
	if field != "command" && field != "url" {
		return variant
	}
	probe := "agnostic_ai_compare_mcp_command"
	if field == "url" {
		probe = "https://agnostic-ai-compare.invalid/mcp"
	}
	for mcpProbePresent(probe, files) || slices.Contains(scalarStrings(mcpCompareValue(e, field, target)), probe) {
		probe += "_"
	}
	variant.Meta[field] = probe
	if override, ok := e.Meta["x-"+target].(map[string]any); ok {
		if _, present := override[field]; present {
			variant.Meta["x-"+target].(map[string]any)[field] = probe
		}
	}
	return variant
}

func mcpProbePresent(probe string, files []adapters.CapturedFile) bool {
	for _, file := range files {
		if strings.Contains(file.Content, probe) {
			return true
		}
	}
	return false
}

func mcpChangedKeyPaths(base, variant []adapters.CapturedFile, field string, value any, server string) ([]string, bool) {
	without := make(map[string]map[string]int, len(variant))
	for _, file := range variant {
		without[file.Path] = mcpKeyValues(file, field, server)
	}
	var paths []string
	verbatim := true
	expected, err := json.Marshal(value)
	for _, file := range base {
		values := mcpKeyValues(file, field, server)
		for v, count := range without[file.Path] {
			values[v] -= count
		}
		changed := false
		for _, count := range values {
			if count > 0 {
				changed = true
			}
		}
		if changed {
			paths = append(paths, filepath.ToSlash(file.Path))
			if err != nil || values[string(expected)] < 1 {
				verbatim = false
			}
		}
	}
	sort.Strings(paths)
	return paths, verbatim
}

func mcpKeyValues(file adapters.CapturedFile, field, server string) map[string]int {
	values := map[string]int{}
	var document any
	if filepath.Ext(file.Path) == ".toml" {
		var parsed map[string]any
		if _, err := toml.Decode(file.Content, &parsed); err != nil {
			return values
		}
		document = parsed
	} else if yaml.Unmarshal([]byte(file.Content), &document) != nil {
		return values
	}
	parent, key, nested := strings.Cut(field, ".")
	collect := func(record map[string]any) {
		value := record[parent]
		if nested {
			entries, _ := value.(map[string]any)
			value = entries[key]
		}
		if value != nil {
			raw, err := json.Marshal(value)
			if err == nil {
				values[string(raw)]++
			}
		}
	}
	var walk func(any)
	connection := func(record map[string]any) bool {
		for _, key := range []string{"command", "url", "serverUrl", "httpUrl", "cmd", "uri"} {
			if record[key] != nil {
				return true
			}
		}
		return false
	}
	walk = func(node any) {
		switch node := node.(type) {
		case map[string]any:
			if name, _ := node["name"].(string); name == server && connection(node) {
				collect(node)
				return
			}
			for name, value := range node {
				if name == server {
					if record, ok := value.(map[string]any); ok && connection(record) {
						collect(record)
						continue
					}
				}
				walk(value)
			}
		case []any:
			for _, value := range node {
				walk(value)
			}
		}
	}
	walk(document)
	return values
}
