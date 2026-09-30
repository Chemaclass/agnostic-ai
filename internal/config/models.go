package config

import (
	"encoding/json"
	"sort"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// ModelTier is one named model role under `models:`. Models maps a target
// name, or `default` for every other target, to that target's model id.
// Effort is the effort the tier carries: a scalar, or a map keyed the
// same way. A spec whose `model` names the tier takes both.
type ModelTier struct {
	Models map[string]string `yaml:",inline"          json:"-"`
	Effort any               `yaml:"effort,omitempty" json:"-"`
}

// MarshalJSON writes the tier in its YAML shape, so the config
// fingerprint changes when a tier does.
func (t ModelTier) MarshalJSON() ([]byte, error) {
	out := make(map[string]any, len(t.Models)+1)
	for target, model := range t.Models {
		out[target] = model
	}
	if t.Effort != nil {
		out["effort"] = t.Effort
	}
	return json.Marshal(out)
}

// ValidateModels rejects a tier with an empty name or model id, and an
// effort that is neither a scalar nor a map of scalars, naming source.
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
	}
	return nil
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
