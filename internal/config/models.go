package config

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// ModelTier is one named model role under `models:`. Models maps a target
// name, or `default` for every other target, to that target's model id.
// Effort is the effort the tier carries: a scalar, or a map keyed the
// same way. A spec whose `model` names the tier takes both. Description
// says when to pick the tier; sync lists it in the instructions.
type ModelTier struct {
	Models      map[string]string `yaml:",inline"               json:"-"`
	Effort      any               `yaml:"effort,omitempty"      json:"-"`
	Description string            `yaml:"description,omitempty" json:"-"`
}

// MarshalJSON writes the tier in its YAML shape, so the config
// fingerprint changes when a tier does.
func (t ModelTier) MarshalJSON() ([]byte, error) {
	out := make(map[string]any, len(t.Models)+2)
	for target, model := range t.Models {
		out[target] = model
	}
	if t.Effort != nil {
		out["effort"] = t.Effort
	}
	if t.Description != "" {
		out["description"] = t.Description
	}
	return json.Marshal(out)
}

// ValidateModels rejects a tier with an empty name or model id, an
// effort that is neither a scalar nor a map of scalars, and a
// description that would break the rendered line, naming source.
func ValidateModels(tiers map[string]ModelTier, source string) error {
	names := make([]string, 0, len(tiers))
	for name := range tiers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if name == "" {
			return errs.Coded(errs.CodeConfigDecode, "%s: models: a tier needs a name", source)
		}
		tier := tiers[name]
		if len(tier.Models) == 0 && tier.Effort == nil {
			return errs.Coded(errs.CodeConfigDecode, "%s: models.%s: a tier needs a model or an effort", source, name)
		}
		for target, model := range tier.Models {
			if model == "" {
				return errs.Coded(errs.CodeConfigDecode, "%s: models.%s.%s: model id is empty", source, name, target)
			}
		}
		if !validTierEffort(tier.Effort) {
			return errs.Coded(errs.CodeConfigDecode, "%s: models.%s.effort must be a scalar or a map of target to scalar", source, name)
		}
		if tier.Description == "" {
			continue
		}
		if !renderableTierText(name) {
			return errs.Coded(errs.CodeConfigDecode, "%s: models.%s: tier name must be one line without backticks or HTML comment markers when the tier has a description", source, name)
		}
		if !renderableTierText(tier.Description) {
			return errs.Coded(errs.CodeConfigDecode, "%s: models.%s.description must be one line without backticks or HTML comment markers", source, name)
		}
	}
	return nil
}

// renderableTierText reports whether text can sit in the one-line tier
// list sync writes inside marked blocks.
func renderableTierText(text string) bool {
	return !strings.ContainsAny(text, "\n\r`") && !strings.Contains(text, "<!--") && !strings.Contains(text, "-->")
}

func validTierEffort(effort any) bool {
	switch v := effort.(type) {
	case nil, string, int, int64:
		return true
	case map[string]any:
		for _, value := range v {
			switch value.(type) {
			case string, int, int64:
			default:
				return false
			}
		}
		return true
	}
	return false
}
