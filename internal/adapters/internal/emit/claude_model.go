package emit

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// ClaudeModelNames are the Claude Code model names: the aliases it
// resolves itself, and "claude-*" for any full Claude model id.
var ClaudeModelNames = []string{"sonnet", "opus", "haiku", "inherit", "claude-*"}

// ClaudeModel reports whether model is a Claude Code model name.
func ClaudeModel(model string) bool {
	return matchesModelName(ClaudeModelNames, model)
}

func matchesModelName(names []string, model string) bool {
	return slices.ContainsFunc(names, func(name string) bool {
		if prefix, ok := strings.CutSuffix(name, "*"); ok {
			return strings.HasPrefix(model, prefix)
		}
		return model == name
	})
}

// noteForeignClaudeModels reports each agent, and the winning settings
// spec, whose shared `model` (a scalar or the map's `default`) is a Claude
// model name the target cannot load. A value set under `model.<target>` or
// `x-<target>.model` is the author's choice for that target and passes, and
// so does a settings model the target's own config replaces.
func noteForeignClaudeModels(c Capabilities, b spec.Bundle, mode string) error {
	if len(c.ForeignClaudeModels) == 0 || mode == OnUnsupportedSilent {
		return nil
	}
	foreign := func(model string) bool {
		return ClaudeModel(model) && matchesModelName(c.ForeignClaudeModels, model)
	}
	type hit struct {
		kind  spec.Kind
		path  string
		model string
	}
	var hits []hit
	if c.supports(spec.KindAgent) {
		for _, a := range b.Agents {
			if model := sharedModel(a.Meta, c.Target); foreign(model) {
				hits = append(hits, hit{spec.KindAgent, a.Path, model})
			}
		}
	}
	if c.supports(spec.KindSettings) && !c.SettingsModelOverridden {
		if path, model := sharedSettingsModel(b.Settings, c.Target); foreign(model) {
			hits = append(hits, hit{spec.KindSettings, path, model})
		}
	}
	if len(hits) == 0 {
		return nil
	}
	fix := func(model string) string {
		return fmt.Sprintf("is a Claude model name; write model: {claude: %s} so %s uses its own default", model, c.Target)
	}
	if mode == OnUnsupportedError {
		h := hits[0]
		return fmt.Errorf("%s: model %q %s", h.path, h.model, fix(h.model))
	}
	type group struct {
		kind  spec.Kind
		model string
	}
	counts := map[group]int{}
	for _, h := range hits {
		counts[group{h.kind, h.model}]++
	}
	groups := make([]group, 0, len(counts))
	for g := range counts {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].kind != groups[j].kind {
			return groups[i].kind < groups[j].kind
		}
		return groups[i].model < groups[j].model
	})
	for _, g := range groups {
		NoteFieldNoOp(c.Target, g.kind, "model", counts[g], g.model+" "+fix(g.model))
	}
	return nil
}

// sharedModel returns the model meta gives target through a value every
// target shares: a string `model`, or the map's `default`. It returns ""
// when `x-<target>.model` or `model.<target>` names one for target.
func sharedModel(meta map[string]any, target string) string {
	if custom, ok := meta[XPrefix+target].(map[string]any); ok {
		if _, set := custom["model"]; set {
			return ""
		}
	}
	switch model := meta["model"].(type) {
	case string:
		return model
	case map[string]any:
		switch model[target].(type) {
		case string, int, int64, float64:
			return ""
		}
		shared, _ := model["default"].(string)
		return shared
	}
	return ""
}

// sharedSettingsModel returns the path and model of the settings spec
// whose `model` wins for target, when that value is shared across
// targets. The last spec that resolves a model wins, as in SettingsModel.
func sharedSettingsModel(entries []spec.Entry, target string) (string, string) {
	var path, model string
	for _, entry := range entries {
		if SettingsModel([]spec.Entry{entry}, target) == "" {
			continue
		}
		path, model = entry.Path, sharedModel(map[string]any{"model": entry.Meta["model"]}, target)
	}
	return path, model
}
