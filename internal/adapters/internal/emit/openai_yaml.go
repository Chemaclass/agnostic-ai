package emit

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// OpenAIYAMLKeys are the top-level fields Codex reads from
// `<skill>/agents/openai.yaml`. Only these pass through from x-codex;
// anything else under `x-codex` stays in SKILL.md frontmatter.
var OpenAIYAMLKeys = []string{"interface", "policy", "dependencies"}

// OpenAIYAMLRel is the Codex skill metadata path, relative to the skill
// folder, in the slash form skip predicates receive.
const OpenAIYAMLRel = "agents/openai.yaml"

// OpenAIYAML renders a skill's Codex `agents/openai.yaml` body, or ""
// when Codex needs no file beyond a verbatim copy of the bundled one.
// A bundled openai.yaml is the base, x-codex keys layer over it, and a
// manual-only skill gets `allow_implicit_invocation: false` unless one
// of the two sets it. Codex reads no disable-model-invocation key.
func OpenAIYAML(s spec.Entry) (string, error) {
	out, err := OpenAIYAMLFields(s)
	if err != nil || out == nil {
		return "", err
	}
	data, err := yaml.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// OpenAIYAMLFields returns the merged openai.yaml document OpenAIYAML
// renders, or nil when there is nothing to render.
func OpenAIYAMLFields(s spec.Entry) (map[string]any, error) {
	bundled, err := bundledOpenAIYAML(s)
	if err != nil {
		return nil, err
	}
	out := maps.Clone(bundled)
	if out == nil {
		out = map[string]any{}
	}
	changed := false
	x, _ := s.Meta["x-codex"].(map[string]any)
	for _, k := range OpenAIYAMLKeys {
		v, ok := x[k]
		if !ok {
			continue
		}
		base, baseIsMap := out[k].(map[string]any)
		over, overIsMap := v.(map[string]any)
		if baseIsMap && overIsMap {
			merged := maps.Clone(base)
			maps.Copy(merged, over)
			v = merged
		}
		out[k] = v
		changed = true
	}
	if manual, _ := ResolveMeta(s.Meta, "codex")["disable-model-invocation"].(bool); manual {
		policy, isMap := out["policy"].(map[string]any)
		_, set := policy["allow_implicit_invocation"].(bool)
		if (isMap || out["policy"] == nil) && !set {
			policy = maps.Clone(policy)
			if policy == nil {
				policy = map[string]any{}
			}
			policy["allow_implicit_invocation"] = false
			out["policy"] = policy
			changed = true
		}
	}
	if !changed {
		return nil, nil
	}
	return out, nil
}

// SkillOpenAIYAMLPolicySet reports whether the skill's openai.yaml, as
// Codex will read it, sets allow_implicit_invocation.
func SkillOpenAIYAMLPolicySet(s spec.Entry) bool {
	out, err := OpenAIYAMLFields(s)
	if err != nil {
		return false
	}
	if out == nil {
		out, _ = bundledOpenAIYAML(s)
	}
	policy, _ := out["policy"].(map[string]any)
	_, ok := policy["allow_implicit_invocation"].(bool)
	return ok
}

func bundlesOpenAIYAML(s spec.Entry) bool {
	if !FolderBasedSkill(s) {
		return false
	}
	_, err := os.Stat(filepath.Join(s.SkillAssetDir(), filepath.FromSlash(OpenAIYAMLRel)))
	return err == nil
}

func bundledOpenAIYAML(s spec.Entry) (map[string]any, error) {
	if !FolderBasedSkill(s) {
		return nil, nil
	}
	path := filepath.Join(s.SkillAssetDir(), filepath.FromSlash(OpenAIYAMLRel))
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return doc, nil
}
