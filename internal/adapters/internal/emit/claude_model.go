package emit

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// ClaudeModelAliases are the model aliases Claude Code resolves itself,
// from code.claude.com/docs/en/model-config. "default" is left out: it
// resets Claude's model rather than naming one.
var ClaudeModelAliases = []string{"sonnet", "opus", "haiku", "fable", "best", "opusplan", "sonnet[1m]", "opus[1m]"}

// ClaudeModelNames are the Claude Code model names: its aliases,
// "inherit" for a subagent, and "claude-*" for any full Claude model id.
var ClaudeModelNames = append(slices.Clone(ClaudeModelAliases), "inherit", "claude-*")

// FlowScalar quotes a value flow YAML would misread, such as the
// brackets in opus[1m], so a suggested {claude: <model>} parses.
func FlowScalar(s string) string {
	if strings.ContainsAny(s, "[]{},:#") {
		return strconv.Quote(s)
	}
	return s
}

// ClaudeModel reports whether model is a Claude Code model name.
func ClaudeModel(model string) bool {
	return matchesModelName(ClaudeModelNames, model)
}

// ForeignClaudeModel reports whether model is a Claude model name that
// matches names, the ones a target cannot load.
func ForeignClaudeModel(names []string, model string) bool {
	return ClaudeModel(model) && matchesModelName(names, model)
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
		return ForeignClaudeModel(c.ForeignClaudeModels, model)
	}
	type hit struct {
		kind  spec.Kind
		path  string
		model string
		tier  string
	}
	var hits []hit
	if c.supports(spec.KindAgent) {
		for _, a := range b.Agents {
			if model := SharedModel(a.Meta, c.Target); foreign(model) {
				hits = append(hits, hit{spec.KindAgent, a.Path, model, a.ModelTier})
			}
		}
	}
	if c.supports(spec.KindSettings) && !c.SettingsModelOverridden {
		if path, model, tier := sharedSettingsModel(b.Settings, c.Target); foreign(model) {
			hits = append(hits, hit{spec.KindSettings, path, model, tier})
		}
	}
	if len(hits) == 0 {
		return nil
	}
	fix := func(model, tier string) string {
		if tier != "" {
			return fmt.Sprintf("is a Claude model name from models.%s; add models.%s.%s so %s gets its own model", tier, tier, c.Target, c.Target)
		}
		return fmt.Sprintf("is a Claude model name; write model: {claude: %s} so %s uses its own default", FlowScalar(model), c.Target)
	}
	if mode == OnUnsupportedError {
		h := hits[0]
		return fmt.Errorf("%s: model %q %s", h.path, h.model, fix(h.model, h.tier))
	}
	type group struct {
		kind  spec.Kind
		model string
		tier  string
	}
	counts := map[group]int{}
	for _, h := range hits {
		counts[group{h.kind, h.model, h.tier}]++
	}
	groups := make([]group, 0, len(counts))
	for g := range counts {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].kind != groups[j].kind {
			return groups[i].kind < groups[j].kind
		}
		if groups[i].model != groups[j].model {
			return groups[i].model < groups[j].model
		}
		return groups[i].tier < groups[j].tier
	})
	for _, g := range groups {
		NoteFieldNoOp(c.Target, g.kind, "model", counts[g], g.model+" "+fix(g.model, g.tier))
	}
	return nil
}

// WithoutForeignClaudeModels returns b without the shared Claude model
// names noteForeignClaudeModels reports, so the target falls back to its
// own default as the note says. Entries are copied: other targets read
// the same bundle.
func WithoutForeignClaudeModels(c Capabilities, b spec.Bundle) spec.Bundle {
	if len(c.ForeignClaudeModels) == 0 {
		return b
	}
	foreign := func(model string) bool {
		return ForeignClaudeModel(c.ForeignClaudeModels, model)
	}
	if c.supports(spec.KindAgent) {
		b.Agents = withoutSharedModel(b.Agents, func(meta map[string]any) bool {
			return foreign(SharedModel(meta, c.Target))
		})
	}
	if c.supports(spec.KindSettings) && !c.SettingsModelOverridden {
		b.Settings = withoutSharedModel(b.Settings, func(meta map[string]any) bool {
			return foreign(SharedModel(map[string]any{"model": meta["model"]}, c.Target))
		})
	}
	return b
}

// withoutSharedModel copies entries, dropping the shared part of `model`
// (a scalar, or the map's `default`) from each one drop matches.
func withoutSharedModel(entries []spec.Entry, drop func(map[string]any) bool) []spec.Entry {
	out := slices.Clone(entries)
	for i, e := range out {
		if !drop(e.Meta) {
			continue
		}
		meta := maps.Clone(e.Meta)
		if model, ok := meta["model"].(map[string]any); ok && len(model) > 1 {
			model = maps.Clone(model)
			delete(model, "default")
			meta["model"] = model
		} else {
			delete(meta, "model")
		}
		out[i].Meta = meta
	}
	return out
}

// SharedModel returns the model meta gives target through a value every
// target shares: a string `model`, or the map's `default`. It returns ""
// when `x-<target>.model` or `model.<target>` names one for target.
func SharedModel(meta map[string]any, target string) string {
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

// sharedSettingsModel returns the path, model, and tier of the settings
// spec whose `model` wins for target, when that value is shared across
// targets. The last spec that resolves a model wins, as in SettingsModel.
func sharedSettingsModel(entries []spec.Entry, target string) (string, string, string) {
	var path, model, tier string
	for _, entry := range entries {
		if SettingsModel([]spec.Entry{entry}, target) == "" {
			continue
		}
		path, model, tier = entry.Path, SharedModel(map[string]any{"model": entry.Meta["model"]}, target), entry.ModelTier
	}
	return path, model, tier
}
