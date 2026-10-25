package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/overview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// The marker distinguishes a server-created scaffold from a user-created one
// with the same YAML values. Only the former is removed when emptied.
const fieldCreatedMarker = "# Created by agnostic-ai serve.\n"

var errFieldNotInFile = errors.New("the field is not in this file")

const fieldCreatedSpec = fieldCreatedMarker + "name: defaults\ndescription: Default model, effort, and session settings.\n"

// fieldEdit resolves an editable source spec, never an arbitrary client path.
func (h *editHandler) fieldEdit(shown, key string, raw json.RawMessage) (string, func([]byte) ([]byte, error), error) {
	if key != "model" && key != "effort" && key != "session-retention-days" {
		return "", nil, fmt.Errorf("%s is not an editable field", key)
	}
	if key == "session-retention-days" && !h.global {
		return "", nil, errors.New("session retention is editable in the global view only")
	}
	scope, err := loadCheckScope(h.global)
	if err != nil {
		return "", nil, err
	}
	var value any
	if !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		switch key {
		case "session-retention-days":
			var n json.Number
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.UseNumber()
			if dec.Decode(&n) != nil {
				return "", nil, errors.New("session-retention-days must be an integer of at least 1")
			}
			i, e := strconv.Atoi(string(n))
			if e != nil || i < 1 {
				return "", nil, errors.New("session-retention-days must be an integer of at least 1")
			}
			value = i
		default:
			var v string
			if json.Unmarshal(raw, &v) != nil || strings.TrimSpace(v) == "" || strings.ContainsAny(v, "\r\n") {
				return "", nil, fmt.Errorf("%s must be a non-empty name", key)
			}
			if key == "effort" {
				allowed := editableEffortLevels(scope.targets)
				if !slices.ContainsFunc(scope.targets, func(target string) bool { return adapters.AcceptsSettingsEffort(target, v) }) {
					return "", nil, fmt.Errorf("effort must be a level a configured target accepts: %s", strings.Join(allowed, ", "))
				}
			}
			value = v
		}
	}
	settings := scope.bundle.Settings
	var candidates []spec.Entry
	for _, e := range settings {
		if overview.SettingsEditCandidate(e, key) {
			candidates = append(candidates, e)
		}
	}
	inEffect := overview.SettingsInEffect(settings, key, scope.targets)
	target := overview.SettingsWriteTarget(settings, inEffect, key, scope.targets)
	unreachable := inEffect >= 0 && target < 0
	defaultFile := filepath.Join(h.root, "settings", "defaults.yaml")
	if !h.global {
		defaultFile = filepath.Join(config.ResolveSourcePath(h.root, scope.cfg.Sources.Settings), "defaults.yaml")
	}
	defaultShown := overviewDisplay(h.root, defaultFile)
	var file string
	var chosen spec.Entry
	if shown != "" {
		at := slices.IndexFunc(settings, func(e spec.Entry) bool {
			return overview.SettingsEditCandidate(e, key) && overview.SpecPath(h.root, e) == shown
		})
		switch {
		case at >= 0:
			if j := overview.SettingsOverride(settings, at, key, scope.targets); j >= 0 {
				return "", nil, fmt.Errorf("%s sets %s and overrides %s; edit that spec", overview.SpecPath(h.root, settings[j]), key, shown)
			}
			chosen, file = settings[at], settings[at].Path
		case shown == defaultShown:
			if j := overview.SettingsOverride(candidates, -1, key, scope.targets); j >= 0 {
				return "", nil, fmt.Errorf("%s sets %s and overrides %s; edit that spec", overview.SpecPath(h.root, candidates[j]), key, shown)
			}
			if len(candidates) > 0 && !unreachable {
				return "", nil, fmt.Errorf("%s is not a settings spec of this configuration", shown)
			}
			file = defaultFile
		default:
			return "", nil, fmt.Errorf("%s is not a settings spec of this configuration", shown)
		}
	} else if target >= 0 {
		chosen, file = settings[target], settings[target].Path
	} else if unreachable || len(candidates) == 0 {
		file = defaultFile
	} else if len(candidates) == 1 {
		chosen, file = candidates[0], candidates[0].Path
	} else {
		return "", nil, errors.New("several editable settings specs exist; choose a path")
	}
	if chosen.Path == "" {
		if value == nil {
			if unreachable {
				return "", nil, fmt.Errorf("%s sets %s; clear it there", overview.SpecPath(h.root, settings[inEffect]), key)
			}
			return "", nil, errors.New("there is no field to clear")
		}
		if at := slices.IndexFunc(settings, func(e spec.Entry) bool { return e.Name == "defaults" }); at >= 0 {
			return "", nil, fmt.Errorf("%s is already the settings spec named defaults; edit it or choose another file", overview.SpecPath(h.root, settings[at]))
		}
		if _, err := os.Lstat(file); err == nil {
			return "", nil, errors.New("defaults.yaml exists but is not a loaded settings spec")
		} else if !os.IsNotExist(err) {
			return "", nil, err
		}
	}
	return file, func(data []byte) ([]byte, error) {
		if len(data) == 0 {
			data = []byte(fieldCreatedSpec)
		}
		out, err := setScalarField(data, key, value)
		if errors.Is(err, errFieldNotInFile) {
			if _, inherited := chosen.Meta[key]; inherited {
				return nil, fmt.Errorf("this file inherits %s from the base spec named %q; clear it there", key, chosen.Name)
			}
			return nil, fmt.Errorf("this file does not set %s", key)
		}
		if err == nil && len(bytes.TrimSpace(out)) == 0 && !bytes.HasPrefix(data, []byte(fieldCreatedMarker)) {
			return nil, errors.New("clearing would leave an empty spec; delete the file instead")
		}
		return out, err
	}, nil
}

func editableEffortLevels(targets []string) []string {
	var levels []string
	for _, target := range targets {
		adapter, err := adapters.Resolve(target)
		if err != nil {
			continue
		}
		accepter, ok := adapter.(adapters.SettingsEffortAccepter)
		if !ok {
			continue
		}
		for _, level := range accepter.SettingsEffortLevels() {
			if !slices.Contains(levels, level) {
				levels = append(levels, level)
			}
		}
	}
	slices.Sort(levels)
	return levels
}

// setScalarField changes a top-level scalar through byte splices. Unsupported
// YAML shapes are refused rather than round-tripped through an encoder.
func setScalarField(data []byte, key string, value any) ([]byte, error) {
	_, mapping, err := yamlDocument(data)
	if err != nil {
		return nil, err
	}
	if mapping.Anchor != "" || mapping.Style&yaml.FlowStyle != 0 {
		return nil, errors.New("flow-style or anchored settings are not safely editable")
	}
	for _, node := range mapping.Content {
		if node.Anchor != "" || node.Kind == yaml.AliasNode {
			return nil, errors.New("anchored settings are not safely editable")
		}
	}
	var old map[string]any
	if err := yaml.Unmarshal(data, &old); err != nil {
		return nil, err
	}
	if old == nil {
		old = map[string]any{}
	}
	_, node := yamlMapValue(mapping, key)
	if _, present := old[key]; present && node == nil {
		return nil, errors.New("inherited settings are not safely editable")
	}
	var out []byte
	if node == nil {
		if value == nil {
			return nil, errFieldNotInFile
		}
		out, err = editYAMLPair(data, mapping, key, value)
		if err != nil {
			return nil, err
		}
	} else {
		if node.Kind != yaml.ScalarNode || node.Anchor != "" || node.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
			return nil, errors.New("multi-line, anchored, or non-scalar settings are not safely editable")
		}
		start, end, e := yamlScalarSpan(data, node)
		if e != nil {
			return nil, fmt.Errorf("field is not safely editable: %w", e)
		}
		if value == nil {
			lineStart := bytes.LastIndexByte(data[:start], '\n') + 1
			lineEnd := end
			for lineEnd < len(data) && data[lineEnd] != '\n' {
				lineEnd++
			}
			if len(bytes.TrimSpace(data[end:lineEnd])) > 0 {
				return nil, errors.New("a field with an inline comment is not safely clearable")
			}
			if lineEnd < len(data) {
				lineEnd++
			}
			if bytes.Contains(data[lineStart:start], []byte("#")) {
				return nil, errors.New("field line is not safely clearable")
			}
			out = spliceEditYAML(data, lineStart, lineEnd, "")
		} else {
			var raw []byte
			raw, err = yaml.Marshal(value)
			if err != nil {
				return nil, err
			}
			out = spliceEditYAML(data, start, end, strings.TrimSpace(string(raw)))
		}
	}
	expected := map[string]any{}
	for k, v := range old {
		expected[k] = v
	}
	if value == nil {
		delete(expected, key)
	} else {
		expected[key] = value
	}
	var actual map[string]any
	if err := yaml.Unmarshal(out, &actual); err != nil {
		return nil, fmt.Errorf("the edit does not parse: %w", err)
	}
	if actual == nil {
		actual = map[string]any{}
	}
	if !reflect.DeepEqual(expected, actual) {
		return nil, errors.New("the edit changes other values")
	}
	if value == nil && string(out) == fieldCreatedSpec {
		return nil, nil
	}
	return out, nil
}
