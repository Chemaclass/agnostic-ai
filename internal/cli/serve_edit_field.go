package cli

import (
	"bytes"
	"cmp"
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

const fieldCreatedSpec = fieldCreatedMarker + "name: defaults\ndescription: Default model, effort, session, and voice settings.\n"
const previousFieldCreatedSpec = fieldCreatedMarker + "name: defaults\ndescription: Default model, effort, and session settings.\n"

func isFieldCreatedSpec(data []byte) bool {
	text := string(data)
	return text == fieldCreatedSpec || text == previousFieldCreatedSpec
}

// fieldEdit resolves an editable source spec, never an arbitrary client path,
// and stores the loaded spec it edits in picked.
func (h *editHandler) fieldEdit(where editAudience, shown string, addr fieldAddress, raw json.RawMessage, picked *spec.Entry) (string, func([]byte) ([]byte, error), error) {
	key := addr.key
	if key != "model" && key != "effort" && key != "session-retention-days" && !voiceEditField(key) {
		return "", nil, fmt.Errorf("%s is not an editable field", key)
	}
	if key == "session-retention-days" && addr.target != "" {
		return "", nil, errors.New("session retention has no per-tool value; send target all")
	}
	if key == "session-retention-days" && !h.global {
		return "", nil, errors.New("session retention is editable in the global view only")
	}
	if voiceEditField(key) && addr.target != "" {
		return "", nil, errors.New("voice has no per-tool value; send target all")
	}
	if voiceEditField(key) && !h.global {
		return "", nil, errors.New("voice is editable in the global view only")
	}
	scope, err := loadCheckScope(h.global)
	if err != nil {
		return "", nil, err
	}
	if addr.target != "" && !slices.Contains(scope.targets, addr.target) {
		return "", nil, fmt.Errorf("target must be all or a configured tool: %s", strings.Join(scope.targets, ", "))
	}
	var value any
	if !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		switch key {
		case "voice.enabled", "voice.mode", "voice.language":
			value, err = decodeVoiceEdit(key, raw)
			if err != nil {
				return "", nil, err
			}
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
			if key == "effort" && addr.target != "" && !hasEffortSetting(addr.target) {
				return "", nil, fmt.Errorf("%s has no effort setting", addr.target)
			}
			if key == "effort" && addr.target != "" && !adapters.AcceptsSettingsEffort(addr.target, v) {
				return "", nil, fmt.Errorf("effort for %s must be one of: %s", addr.target, strings.Join(editableEffortLevels([]string{addr.target}), ", "))
			}
			if key == "effort" && addr.target == "" {
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
	edited := scope.targets
	if addr.target != "" {
		edited = []string{addr.target}
	}
	inEffect := overview.SettingsInEffect(settings, key, edited, h.global)
	target := overview.SettingsWriteTarget(settings, inEffect, key, edited)
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
			return overview.SettingsEditCandidate(e, key) && h.matchesShownSpec(e, shown)
		})
		switch {
		case at >= 0:
			if j := overview.SettingsOverride(settings, at, key, edited, h.global); j >= 0 {
				return "", nil, fmt.Errorf("%s sets %s and overrides %s; edit that spec", overview.SettingsValuePath(h.root, settings[j], key, edited, scope.models), key, shown)
			}
			chosen, file = settings[at], settings[at].Path
		case shown == defaultShown:
			later := candidates
			if unreachable {
				later = nil
				for _, e := range settings[inEffect+1:] {
					if overview.SettingsEditCandidate(e, key) {
						later = append(later, e)
					}
				}
			}
			if j := overview.SettingsOverride(later, -1, key, edited, h.global); j >= 0 {
				return "", nil, fmt.Errorf("%s sets %s and overrides %s; edit that spec", overview.SettingsValuePath(h.root, later[j], key, edited, scope.models), key, shown)
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
	if voiceEditField(key) {
		if reason := overview.VoiceEditDisabledReason(chosen, h.root); reason != "" {
			return "", nil, errors.New(reason)
		}
	}
	*picked = chosen
	if key == "effort" && addr.target != "" && value != nil {
		fromTier, err := tierGivesEffort(chosen, scope.models)
		if err != nil {
			return "", nil, err
		}
		if fromTier {
			return "", nil, tierEffortError{fmt.Errorf("effort comes from the model tier %s, which stops applying once the spec sets effort, so a value for %s alone would drop it for every other tool; set effort for every tool, or edit the spec", chosen.ModelTier, addr.target)}
		}
	}
	scaffold := fieldCreatedSpec
	if where == audiencePersonal {
		return h.localFieldEdit(chosen, addr, value)
	}
	if chosen.Path != "" && overview.LocalLayer(chosen) && chosen.Base == nil {
		return h.newTeamFieldSpec(chosen, filepath.Dir(defaultFile), addr, value)
	}
	if chosen.Path != "" {
		var err error
		if file, _, err = h.scopedSettingsFile(chosen, addr.personalKey(chosen), audienceShared); err != nil {
			return "", nil, err
		}
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
	return file, scalarFieldEdit(chosen, addr, value, nil, scaffold), nil
}

// localFieldEdit writes key to the personal file over chosen, or to a new
// personal defaults spec when no spec takes the edit. Clearing is the
// reset: it removes the personal value, and the file once it only names
// the spec.
func (h *editHandler) localFieldEdit(chosen spec.Entry, addr fieldAddress, value any) (string, func([]byte) ([]byte, error), error) {
	key := addr.key
	if value == nil && !overview.LocalSets(chosen, addr.String()) {
		if _, inherited := overview.SettingsFieldValue(chosen.Meta, key); inherited && addr.target == "" {
			return "", nil, noPersonalValueError{fmt.Errorf("this file inherits %s from the shared spec named %q; switch to Shared to clear it", key, chosen.Name)}
		}
		return "", nil, noPersonalValueError{fmt.Errorf("%s: there is no personal %s to reset", overview.SettingsOverlayPath(h.global, cmp.Or(chosen.Name, "defaults")), addr.label())}
	}
	if chosen.Path != "" {
		file, scaffold, err := h.scopedSettingsFile(chosen, key, audiencePersonal)
		if err != nil {
			return "", nil, err
		}
		base, err := teamFieldValue(chosen, key)
		if err != nil {
			return "", nil, err
		}
		return file, scalarFieldEdit(chosen, addr, value, base, scaffold), nil
	}
	file := filepath.Join(h.root, overview.SettingsOverlayPath(h.global, "defaults"))
	if _, err := os.Lstat(file); err == nil {
		return "", nil, fmt.Errorf("%s exists but is not a loaded settings spec", overviewDisplay(h.root, file))
	} else if !os.IsNotExist(err) {
		return "", nil, fmt.Errorf("%s: %w", file, err)
	}
	if err := h.checkPersonalTarget(file); err != nil {
		return "", nil, err
	}
	return file, scalarFieldEdit(chosen, addr, value, nil, fieldCreatedSpec), nil
}

// teamFieldValue is the value the team spec behind a personal edit of
// chosen gives key, as written, before any model tier applies. A loaded
// spec holds its value with tiers applied, so a spec with no personal
// file yet is read from disk; the base of an overlay keeps its own.
func teamFieldValue(chosen spec.Entry, key string) (any, error) {
	switch {
	case chosen.Base != nil:
		value, _ := overview.SettingsFieldValue(chosen.Base.Meta, key)
		return value, nil
	case overview.LocalLayer(chosen):
		return nil, nil
	}
	data, err := os.ReadFile(chosen.Path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", chosen.Path, err)
	}
	var values map[string]any
	if err := yaml.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("parse %s: %w", chosen.Path, err)
	}
	value, _ := overview.SettingsFieldValue(values, key)
	return value, nil
}

// newTeamFieldSpec starts the shared spec under a personal one that has
// none, so a Team save lands in the committed settings.
func (h *editHandler) newTeamFieldSpec(personal spec.Entry, dir string, addr fieldAddress, value any) (string, func([]byte) ([]byte, error), error) {
	if value == nil {
		return "", nil, fmt.Errorf("%s: there is no shared %s to clear", overview.SpecPath(h.root, personal), addr.label())
	}
	if blocking := addr.personalKey(personal); overview.LocalSets(personal, blocking) {
		return "", nil, fieldSourceError{fmt.Errorf("%s sets %s for you; reset it to edit the shared value", overview.SpecPath(h.root, personal), blocking)}
	}
	file := filepath.Join(dir, personal.Name+".yaml")
	if _, err := os.Lstat(file); err == nil {
		return "", nil, fmt.Errorf("%s exists but is not the settings spec named %q", overviewDisplay(h.root, file), personal.Name)
	} else if !os.IsNotExist(err) {
		return "", nil, fmt.Errorf("%s: %w", file, err)
	}
	scaffold, err := yaml.Marshal(struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}{personal.Name, "Shared settings."})
	if err != nil {
		return "", nil, err
	}
	return file, scalarFieldEdit(spec.Entry{}, addr, value, nil, fieldCreatedMarker+string(scaffold)), nil
}

// tierEffortError refuses a one-tool effort that would turn off the
// effort a model tier gives every tool.
type tierEffortError struct{ error }

// tierGivesEffort reports whether chosen's effort comes only from its
// model tier: the tier sets one and no file behind chosen sets effort.
func tierGivesEffort(chosen spec.Entry, tiers map[string]config.ModelTier) (bool, error) {
	if tier, ok := tiers[chosen.ModelTier]; !ok || tier.Effort == nil {
		return false, nil
	}
	if chosen.Base != nil {
		_, shared := chosen.Base.Meta["effort"]
		_, personal := chosen.LocalMeta["effort"]
		return !shared && !personal, nil
	}
	data, err := os.ReadFile(chosen.Path)
	if err != nil {
		return false, fmt.Errorf("%s: %w", chosen.Path, err)
	}
	var values map[string]any
	if err := yaml.Unmarshal(data, &values); err != nil {
		return false, fmt.Errorf("parse %s: %w", chosen.Path, err)
	}
	_, own := values["effort"]
	return !own, nil
}

func hasEffortSetting(target string) bool {
	adapter, err := adapters.Resolve(target)
	if err != nil {
		return false
	}
	_, ok := adapter.(adapters.SettingsEffortAccepter)
	return ok
}

// fieldSourceError refuses a team edit because a personal file sets the
// field; for a page that loaded before that, it is a conflict.
type fieldSourceError struct{ error }

// noPersonalValueError refuses a reset with nothing to reset, which a page
// that loaded before an earlier reset asks for again.
type noPersonalValueError struct{ error }

type staleFieldError struct {
	key     string
	current *fieldText
}

func (e staleFieldError) Error() string {
	now := "not set"
	if e.current != nil {
		now = "now " + e.current.shown
	}
	return fmt.Sprintf("%s changed after this page loaded and is %s. Reload to review it, then save again", e.key, now)
}

// fieldText is a field's value as written and as the page shows it,
// which hides values that look like credentials.
type fieldText struct{ raw, shown string }

func (t *fieldText) same(o *fieldText) bool {
	return t == nil && o == nil || t != nil && o != nil && t.raw == o.raw
}

// expectFieldValue refuses edit unless the destination still holds the
// value the page showed, or already holds the one the edit writes, as after
// a save whose follow-up reload failed. A key the file leaves out shows as
// unset, or as the value the loaded spec inherits, such as a model tier's
// effort.
func expectFieldValue(addr fieldAddress, expected *string, clearing bool, inherited *fieldText, edit func([]byte) ([]byte, error)) func([]byte) ([]byte, error) {
	key := addr.String()
	valueIn := func(data []byte) (*fieldText, error) {
		t, err := fieldValueText(data, addr)
		if t == nil && err == nil {
			return inherited, nil
		}
		return t, err
	}
	return func(data []byte) ([]byte, error) {
		current, err := valueIn(data)
		if err != nil {
			return nil, err
		}
		if clearing && current == nil {
			return data, nil
		}
		if current != nil && current.raw != current.shown {
			return nil, fmt.Errorf("the page hides the value of %s; edit the file directly", key)
		}
		if current == nil && expected == nil || current != nil && expected != nil && current.raw == *expected {
			return edit(data)
		}
		if out, err := edit(data); err == nil {
			if written, err := valueIn(out); err == nil && written.same(current) {
				return data, nil
			}
		}
		return nil, staleFieldError{key: key, current: current}
	}
}

// inheritedFieldText is the value the page shows for key at file, the one
// a field edit of chosen writes, when file leaves the key out.
func inheritedFieldText(where editAudience, chosen spec.Entry, file string, addr fieldAddress) *fieldText {
	local := overview.LocalLayer(chosen)
	if chosen.Path == "" || where == audiencePersonal && (!local || chosen.Base != nil) || where == audienceShared && local && chosen.Base == nil {
		return nil
	}
	if data, err := os.ReadFile(file); err == nil {
		var values map[string]any
		if err := yaml.Unmarshal(data, &values); err != nil {
			return nil
		}
		if _, own := overview.SettingsFieldValue(values, addr.key); own {
			return nil
		}
	}
	v, ok := overview.SettingsFieldValue(chosen.Meta, addr.key)
	if v, ok = addr.in(v, ok); !ok {
		return nil
	}
	return &fieldText{raw: overview.Scalar(v), shown: overview.ShownText(addr.String(), v)}
}

// fieldValueText is the value data gives the address, or nil when the
// file leaves it out.
func fieldValueText(data []byte, addr fieldAddress) (*fieldText, error) {
	var values map[string]any
	if err := yaml.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	v, ok := overview.SettingsFieldValue(values, addr.key)
	if v, ok = addr.in(v, ok); !ok {
		return nil, nil
	}
	return &fieldText{raw: overview.Scalar(v), shown: overview.ShownText(addr.String(), v)}, nil
}

// scalarFieldEdit sets or clears the address in a settings file. A new
// file starts from scaffold, and is removed again when clearing leaves
// only that. base is the team value a personal file merges over.
func scalarFieldEdit(chosen spec.Entry, addr fieldAddress, value, base any, scaffold string) func([]byte) ([]byte, error) {
	return func(data []byte) ([]byte, error) {
		if len(data) == 0 {
			data = []byte(scaffold)
		}
		out, err := setFieldAt(data, addr, value, base)
		if errors.Is(err, errFieldNotInFile) {
			if _, inherited := overview.SettingsFieldValue(chosen.Meta, addr.key); inherited && addr.target == "" {
				return nil, fmt.Errorf("this file inherits %s from the base spec named %q; clear it there", addr.key, chosen.Name)
			}
			return nil, fmt.Errorf("this file does not set %s", addr.label())
		}
		if err == nil && value == nil && (string(out) == scaffold || isFieldCreatedSpec(out)) {
			return nil, nil
		}
		if err == nil && len(bytes.TrimSpace(out)) == 0 && !bytes.HasPrefix(data, []byte(fieldCreatedMarker)) {
			return nil, errors.New("clearing would leave an empty spec; delete the file instead")
		}
		return out, err
	}
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
	if value == nil && isFieldCreatedSpec(out) {
		return nil, nil
	}
	return out, nil
}
