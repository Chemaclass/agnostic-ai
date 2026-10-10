package cli

import (
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

type permissionCapture struct {
	files []adapters.CapturedFile
	notes []adapters.Note
}

func comparedPermissionFields(e spec.Entry) []string {
	permissions, _ := e.Meta["permissions"].(map[string]any)
	var fields []string
	for _, list := range spec.PermissionLists {
		for index := range toStringSlice(permissions[list]) {
			fields = append(fields, fmt.Sprintf("permissions.%s[%d]", list, index))
		}
	}
	if _, ok := permissions["default-mode"]; ok {
		fields = append(fields, "permissions.default-mode")
	}
	return fields
}

func permissionFieldSource(e spec.Entry, field string) (string, string, int) {
	list, position, _ := strings.Cut(strings.TrimPrefix(field, "permissions."), "[")
	index, _ := strconv.Atoi(strings.TrimSuffix(position, "]"))
	permissions, _ := e.Meta["permissions"].(map[string]any)
	rules := toStringSlice(permissions[list])
	if index >= len(rules) {
		return list, "", index
	}
	return list, rules[index], index
}

func withoutPermission(settings []spec.Entry, entry spec.Entry, field string) []spec.Entry {
	result := slices.Clone(settings)
	for i, candidate := range settings {
		if candidate.Path != entry.Path || candidate.Name != entry.Name {
			continue
		}
		meta := maps.Clone(candidate.Meta)
		permissions, _ := meta["permissions"].(map[string]any)
		permissions = maps.Clone(permissions)
		if field == "permissions.default-mode" {
			delete(permissions, "default-mode")
		} else {
			list, _, index := permissionFieldSource(candidate, field)
			rules := slices.Clone(toStringSlice(permissions[list]))
			if index < len(rules) {
				rules = slices.Delete(rules, index, index+1)
			}
			values := make([]any, len(rules))
			for j, rule := range rules {
				values[j] = rule
			}
			permissions[list] = values
		}
		meta["permissions"] = permissions
		result[i].Meta = meta
		break
	}
	return result
}

func capturePermissions(cfg *config.Config, settings []spec.Entry, target string) (permissionCapture, error) {
	adapter, _ := adapters.Get(target)
	adapters.ResetCoverageNotes()
	files, err := captureEmit(adapter, spec.Bundle{Settings: settings}, cfg)
	notes := adapters.DrainNotes()
	if err != nil {
		return permissionCapture{}, fmt.Errorf("compare settings permissions: %w", err)
	}
	return permissionCapture{files: files, notes: notes}, nil
}

func classifyPermissions(cfg *config.Config, settings []spec.Entry, e spec.Entry, fields []string, target string, cache map[string]permissionCapture) (map[string]compareResult, error) {
	results := make(map[string]compareResult, len(fields))
	if !e.EmitsTo(target) {
		for _, field := range fields {
			results[field] = compareResult{Target: target, Status: statusExcluded, Reason: "the spec's target filter skips " + target, Next: "edit target, targets, target-exclude, or targets-exclude in " + filepath.ToSlash(e.Path)}
		}
		return results, nil
	}
	base, ok := cache[target]
	if !ok {
		var err error
		base, err = capturePermissions(cfg, settings, target)
		if err != nil {
			return nil, err
		}
		cache[target] = base
	}
	for _, field := range fields {
		r := compareResult{Target: target}
		if field == "permissions.default-mode" {
			permissions, _ := e.Meta["permissions"].(map[string]any)
			r.Status, r.Reason = statusUnsupported, fmt.Sprintf("permissions.default-mode %v maps during global sync; project comparison has no portable mode mapping", permissions["default-mode"])
			results[field] = r
			continue
		}
		list, rule, _ := permissionFieldSource(e, field)
		translated := adapters.TranslatePermissionCapabilityIn(target, list, rule, e, settings, cfg)
		variant, err := capturePermissions(cfg, withoutPermission(settings, e, field), target)
		if err != nil {
			r.Status, r.Reason = statusUnknown, rule+": the adapter cannot emit a valid comparison for this permission"
			results[field] = r
			continue
		}
		paths, preserved, meaning := permissionNativePaths(base.files, list, rule, translated)
		r.permissionMeaning = meaning
		_, changed := fieldEffect(base.files, variant.files)
		switch {
		case len(paths) > 0 && len(translated.Native) > 0:
			r.permissionComplete = translated.Supported
			r.Status, r.Paths = statusTranslated, paths
			if preserved && translated.Supported && translated.Override == "" && len(translated.Widening) == 0 {
				r.Status = statusPreserved
			}
			r.Reason = rule + ": " + list + " maps to " + strings.Join(translated.Native, ", ")
			if !translated.Supported {
				r.Reason += "; part of this permission is unsupported"
			}
		case !translated.Supported:
			r.Status, r.Reason = statusUnsupported, rule+": the adapter does not write this portable "+list+" permission"
			if target == "codex" && !cfg.Outputs[target].ExecPoliciesFromPermissions {
				r.Next = "set outputs.codex.exec-policies-from-permissions: true to translate supported shell permissions"
			}
		case len(base.files) == 0 && translated.Override == "":
			r = noOutputResult(base.notes, spec.KindSettings, target)
			r.Target = target
			r.Reason = rule + ": " + r.Reason
		default:
			r.Status, r.Reason = statusUnknown, rule+": the emitted permission cannot be attributed to this source rule"
			if len(changed) == 0 {
				r.Reason = rule + ": this source rule makes no isolated native permission change"
			}
		}
		if translated.Override != "" {
			r.Reason += "; native override: " + translated.Override
		}
		for _, widening := range translated.Widening {
			r.Reason += "; " + widening
		}
		results[field] = r
	}
	return results, nil
}

func permissionNativePaths(files []adapters.CapturedFile, list, rule string, translated adapters.CapabilityTranslation) ([]string, bool, string) {
	var paths []string
	preserved := true
	var meanings []string
	for _, file := range files {
		matched, same, meaning := permissionFileEvidence(file, list, rule, translated)
		if matched {
			paths = append(paths, filepath.ToSlash(file.Path))
			preserved = preserved && same
			meanings = append(meanings, meaning...)
		}
	}
	sort.Strings(paths)
	sort.Strings(meanings)
	return slices.Compact(paths), preserved, strings.Join(slices.Compact(meanings), ",")
}

var permissionPrefixStatement = regexp.MustCompile(`(?s)prefix_rule\(\s*pattern\s*=\s*(\[[^\]]*\]),\s*decision\s*=\s*("(?:[^"\\]|\\.)*"),`)

func permissionFileEvidence(file adapters.CapturedFile, list, rule string, translated adapters.CapabilityTranslation) (bool, bool, []string) {
	decision := list
	switch list {
	case "ask":
		decision = "prompt"
	case "deny":
		decision = "forbidden"
	}
	var document map[string]any
	if filepath.Ext(file.Path) == ".toml" {
		_, _ = toml.Decode(file.Content, &document)
	} else {
		_ = yaml.Unmarshal([]byte(file.Content), &document)
	}
	matched := false
	foundCount := 0
	preserved := true
	var meanings []string
	for _, native := range translated.Native {
		found, same, effective := false, false, list
		if rest, ok := strings.CutPrefix(native, "prefix_rule("); ok {
			pattern, override, _ := strings.Cut(rest, ")")
			wanted := strings.Fields(pattern)
			if value := strings.TrimSpace(strings.TrimPrefix(override, ":")); value != "" {
				decision = value
			}
			for _, match := range permissionPrefixStatement.FindAllStringSubmatch(file.Content, -1) {
				var words []string
				var actual string
				if json.Unmarshal([]byte(match[1]), &words) == nil && json.Unmarshal([]byte(match[2]), &actual) == nil && slices.Equal(words, wanted) && actual == decision {
					found, effective = true, actual
				}
			}
		} else {
			for _, parent := range []string{"permissions", "permission"} {
				permissions, _ := document[parent].(map[string]any)
				if slices.Contains(toStringSlice(permissions[list]), native) {
					found, same = true, parent == "permissions" && native == rule
				}
				key, raw, overridden := strings.Cut(native, ": ")
				if overridden {
					var expected any
					if json.Unmarshal([]byte(raw), &expected) == nil && expected != nil && reflect.DeepEqual(permissions[key], expected) {
						found, effective = true, raw
					}
				} else {
					key, pattern, scoped := strings.Cut(native, "(")
					if scoped {
						pattern = strings.TrimSuffix(pattern, ")")
					} else {
						pattern = "*"
					}
					actual := permissions[key]
					if mapping, ok := actual.(map[string]any); ok {
						actual = mapping[pattern]
					}
					if actual == list {
						found = true
					}
				}
			}
			var expected map[string]any
			if json.Unmarshal([]byte(native), &expected) == nil && expected["toolName"] != nil {
				for _, raw := range asAnyList(document["toolPermissions"]) {
					if reflect.DeepEqual(raw, expected) {
						found = true
						body, _ := json.Marshal(expected["permission"])
						effective = string(body)
					}
				}
			}
		}
		if found {
			matched = true
			foundCount++
			preserved = preserved && same
			meanings = append(meanings, permissionDecisionMeaning(effective))
		}
	}
	if translated.Supported && foundCount != len(translated.Native) {
		return false, false, nil
	}
	return matched, preserved, meanings
}

func asAnyList(value any) []any {
	list, _ := value.([]any)
	return list
}

func permissionDecisionMeaning(value string) string {
	var parsed any
	if json.Unmarshal([]byte(value), &parsed) == nil {
		if scalar, ok := parsed.(string); ok {
			value = scalar
		} else if object, ok := parsed.(map[string]any); ok {
			if decision, ok := object["type"].(string); ok {
				value = decision
			}
		}
	}
	switch value {
	case "forbidden":
		return "deny"
	case "prompt":
		return "ask"
	default:
		return value
	}
}
