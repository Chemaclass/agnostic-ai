package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"sort"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

var hookCompareFields = map[string]bool{
	"on": true, "match": true, "event": true, "matcher": true,
	"command": true, "args": true, "timeout": true, "failClosed": true,
}

func compareHookField(e spec.Entry, field, target string, files []adapters.CapturedFile, notes []adapters.Note) (compareResult, bool) {
	result := compareResult{Target: target}
	if reason, ok := fieldNote(notes, field); ok {
		result.Status, result.Reason = statusUnsupported, reason
		return result, true
	}
	if field != "on" && field != "event" && field != "match" {
		return result, false
	}
	native, reason := e.NativeHook(target)
	if reason != "" {
		result.Status, result.Reason = statusUnsupported, reason
		return result, true
	}
	native = adapters.TargetHook(target, native)
	key, value := "matcher", native.Meta["matcher"]
	if field == "on" || field == "event" {
		key, _ = native.Meta["event"].(string)
		value = nil
	}
	paths, verbatim := pathsWithKey(files, key, value)
	if key == "" || len(paths) == 0 || !verbatim {
		result.Status = statusUnknown
		result.Reason = "the written files do not identify this hook's " + field + " field"
		return result, true
	}
	result.Status, result.Paths = statusTranslated, paths
	if field == "match" {
		result.Reason = fmt.Sprintf("written as matcher: %v", value)
	} else {
		result.Reason = "written as the " + key + " event group"
	}
	return result, true
}

// Keep the required command present so the comparison never emits an invalid handler.
func compareHookCommandVariant(e spec.Entry, target string) spec.Entry {
	variant := withoutField(e, "command", target)
	probe := "printf agnostic_ai_compare_command"
	for slices.Contains(scalarStrings(resolvedValue(e, "command", target)), probe) {
		probe += "_"
	}
	variant.Meta["command"] = probe
	variant.MetaKeys = slices.Clone(e.MetaKeys)
	if override, ok := e.Meta["x-"+target].(map[string]any); ok {
		if _, present := override["command"]; present {
			variant.Meta["x-"+target].(map[string]any)["command"] = probe
		}
	}
	return variant
}

func hookChangedKeyPaths(base, variant []adapters.CapturedFile, field string, value any) ([]string, bool) {
	without := make(map[string]map[string]int, len(variant))
	for _, file := range variant {
		without[file.Path] = hookJSONKeyValues(file.Content, field)
	}
	var paths []string
	verbatim := true
	for _, file := range base {
		values := hookJSONKeyValues(file.Content, field)
		for v, count := range without[file.Path] {
			values[v] -= count
		}
		changed := false
		for _, count := range values {
			if count > 0 {
				changed = true
			}
		}
		if !changed {
			continue
		}
		paths = append(paths, filepath.ToSlash(file.Path))
		expected := []any{value}
		if list, ok := value.([]any); field == "command" && ok {
			expected = list
		}
		for _, v := range expected {
			raw, err := json.Marshal(v)
			if err != nil || values[string(raw)] < 1 {
				verbatim = false
			}
			values[string(raw)]--
		}
	}
	sort.Strings(paths)
	return paths, verbatim
}

func hookJSONKeyValues(content, field string) map[string]int {
	values := map[string]int{}
	var document any
	if json.Unmarshal([]byte(content), &document) != nil {
		return values
	}
	var walk func(any)
	walk = func(node any) {
		switch node := node.(type) {
		case map[string]any:
			for key, value := range node {
				if key == field {
					raw, _ := json.Marshal(value)
					values[string(raw)]++
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
