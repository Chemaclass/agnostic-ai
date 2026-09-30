package cli

import (
	"bytes"
	"fmt"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// validateTierTargets rejects a tier key that is neither a target, nor
// `default`, which reads as a typo that would leave the target on its
// tool default. source leads the message.
func validateTierTargets(tiers map[string]config.ModelTier, source string) error {
	for _, name := range sortedTierNames(tiers) {
		keys := make([]string, 0, len(tiers[name].Models))
		for key := range tiers[name].Models {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			if key == "default" || slices.Contains(adapters.Names(), key) {
				continue
			}
			if s := adapters.SuggestName(key, adapters.Names()); s != "" {
				return errs.Coded(errs.CodeConfigDecode, "%s: models.%s: unknown target %q (did you mean %s?)", source, name, key, s)
			}
			return errs.Coded(errs.CodeConfigDecode, "%s: models.%s: unknown target %q", source, name, key)
		}
	}
	return nil
}

// importModelTiers returns the tiers of the project config in root, or
// nil when it has none or does not load. Import only compares against
// them, so a broken config means no tier to keep.
func importModelTiers(root string) map[string]config.ModelTier {
	cfg, err := config.Load(root)
	if err != nil {
		return nil
	}
	return cfg.Models
}

// tierModelFor returns the model and effort that meta's tier gives target,
// and false when meta's `model` names no tier.
func tierModelFor(meta map[string]any, tiers map[string]config.ModelTier, target string) (string, any, bool) {
	b := spec.Bundle{Agents: []spec.Entry{{Kind: spec.KindAgent, Meta: meta}}}
	b.ApplyModelTiers(tiers)
	if b.Agents[0].ModelTier == "" {
		return "", nil, false
	}
	resolved := adapters.ResolveMeta(b.Agents[0].Meta, target)
	model, _ := resolved["model"].(string)
	return model, resolved["effort"], true
}

// keepClaudeTierModel puts the existing spec's `model` and `effort` back
// into an imported Claude agent when the imported values are the ones the
// spec's tier gives Claude, so a sync then import round trip keeps the
// tier. doc is the imported file with its frontmatter.
func keepClaudeTierModel(doc string, existing map[string]any, tiers map[string]config.ModelTier) string {
	model, effort, ok := tierModelFor(existing, tiers, "claude")
	if !ok {
		return doc
	}
	rest, end, imported := claudeFrontmatter(doc)
	if imported == nil {
		return doc
	}
	importedModel, _ := imported["model"].(string)
	if scoped, isMap := imported["model"].(map[string]any); isMap && len(scoped) == 1 {
		importedModel, _ = scoped["claude"].(string)
	}
	if importedModel != model || !sameScalar(imported["effort"], effort) {
		return doc
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(rest[:end]), &node); err != nil || len(node.Content) == 0 {
		return doc
	}
	mapping := node.Content[0]
	setMappingValue(mapping, "model", existing["model"])
	if value, set := existing["effort"]; set {
		setMappingValue(mapping, "effort", value)
	} else {
		removeMappingKey(mapping, "effort")
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(mapping); err != nil {
		return doc
	}
	if err := enc.Close(); err != nil {
		return doc
	}
	return "---\n" + buf.String() + rest[end+1:]
}

// sameScalar is equalScalar that also matches two absent values.
func sameScalar(a, b any) bool {
	return a == nil && b == nil || equalScalar(a, b)
}

func removeMappingKey(mapping *yaml.Node, key string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return
		}
	}
}

// tierNameShadowsClaudeModel explains why a tier named like a Claude
// model is risky: `model: opus` then names the tier on every target.
func tierNameShadowsClaudeModel(name string) string {
	return fmt.Sprintf("models.%s: the tier name is a Claude model name, so model: %s names the tier on every target; rename the tier", name, name)
}
