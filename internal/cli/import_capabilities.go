package cli

import (
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func importNeutralToolNames(doc, field string, rename bool) string {
	rewritten, err := rewriteFrontmatter(doc, func(front string) (string, error) {
		entry, err := spec.ParseMarkdownBytes(spec.KindSkill, []byte("---\n"+front+"\n---\n"))
		if err != nil {
			return "", err
		}
		list, ok := entry.Meta[field].([]any)
		if !ok {
			if scalar, isScalar := entry.Meta[field].(string); isScalar && !rename {
				return importNeutralSkillScalar(front, field, scalar)
			}
			return front, nil
		}
		items := map[int]string{}
		for i, raw := range list {
			if name, ok := raw.(string); ok {
				if neutral, exact := spec.NeutralCapability(name); exact {
					items[i] = neutral
				}
			}
		}
		if len(items) == 0 {
			return front, nil
		}
		key := field
		if rename {
			key = "can"
		}
		return rewriteTopLevelYAMLSequence(front, yamlSequenceRewrite{Key: field, NewKey: key, Items: items})
	})
	if err != nil {
		return doc
	}
	return rewritten
}

func importNativeAgentTools(data []byte, target, field string) ([]byte, error) {
	meta, body := splitMdcFrontmatter(data)
	value, set := meta[field]
	if !set {
		return data, nil
	}
	portable, exact := importNativeAgentCan(value, target)
	delete(meta, field)
	if exact {
		meta["can"] = portable
	} else {
		custom, _ := meta["x-"+target].(map[string]any)
		if custom == nil {
			custom = map[string]any{}
		}
		custom[field] = value
		meta["x-"+target] = custom
	}
	front, err := yaml.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("marshal %s agent frontmatter: %w", target, err)
	}
	return []byte("---\n" + string(front) + "---\n\n" + strings.TrimRight(body, "\n") + "\n"), nil
}

func importNeutralPermissionRule(rule string) string {
	if capability, exact := spec.NeutralPermission(rule); exact {
		return capability
	}
	return rule
}

func importNativeAgentCan(value any, target string) ([]string, bool) {
	list, ok := value.([]any)
	if !ok || len(list) == 0 {
		return nil, false
	}
	native := toStringSlice(value)
	if len(native) != len(list) {
		return nil, false
	}
	portable, exact := adapters.AgentCapabilitiesFromNative(target, native)
	if !exact {
		return nil, false
	}
	var emitted []string
	for _, capability := range portable {
		translated := adapters.TranslateCapability(target, capability)
		if !translated.Supported {
			return nil, false
		}
		for _, name := range translated.Native {
			if !slices.Contains(emitted, name) {
				emitted = append(emitted, name)
			}
		}
	}
	return portable, slices.Equal(native, emitted)
}

func importNeutralSkillScalar(front, field, value string) (string, error) {
	names := spec.SplitToolList(value)
	changed := false
	for i, name := range names {
		names[i] = name
		if neutral, exact := spec.NeutralCapability(name); exact {
			names[i] = neutral
			changed = changed || name != neutral
		}
	}
	if !changed {
		return front, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(front), &doc); err != nil {
		return "", err
	}
	node, err := onlyValue(doc.Content[0], field)
	if err != nil {
		return "", err
	}
	node.Value = strings.Join(names, ", ")
	rewritten, err := yaml.Marshal(&doc)
	if err != nil {
		return "", err
	}
	return string(rewritten), nil
}
