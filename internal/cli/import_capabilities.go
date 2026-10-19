package cli

import (
	"bytes"
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
	meta, _ := splitMdcFrontmatter(data)
	value, set := meta[field]
	if !set {
		return data, nil
	}
	mapping, rest, ok := frontmatterNode(data)
	if !ok {
		return data, nil
	}
	at := -1
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == field {
			at = i
		}
	}
	if at < 0 {
		return data, nil
	}
	if portable, exact := importNativeAgentCan(value, target); exact {
		var node yaml.Node
		if err := node.Encode(portable); err != nil {
			return nil, fmt.Errorf("encode %s agent can: %w", target, err)
		}
		node.Style = mapping.Content[at+1].Style
		node.LineComment = mapping.Content[at+1].LineComment
		mapping.Content[at].Value = "can"
		mapping.Content[at+1] = &node
	} else {
		native := mapping.Content[at+1]
		mapping.Content = append(mapping.Content[:at], mapping.Content[at+2:]...)
		setMappingNode(xTargetMapping(mapping, "x-"+target), field, native)
	}
	return encodeFrontmatter(mapping, rest, target)
}

// encodeFrontmatter writes mapping back as the frontmatter of a
// Markdown spec, followed by rest, the text from its closing `---` on.
func encodeFrontmatter(mapping *yaml.Node, rest, target string) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(mapping); err != nil {
		return nil, fmt.Errorf("encode %s agent frontmatter: %w", target, err)
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return []byte("---\n" + buf.String() + rest), nil
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
