package ellipsis

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// reviewerOrder follows the order Ellipsis documents a reviewer in.
var reviewerOrder = []string{"name", "paths", "claude_code", "codex", "environment", "skills", "permissions", "budget", "comments"}

// render writes the file with the keys in documented order. Nested
// settings keep their own values and sort by key, so a sync is stable.
func render(f *reviewConfig, prompt string) (string, error) {
	top := mapping()
	header := map[string]any{"kind": fileKind, "name": f.name}
	for k, v := range f.header {
		header[k] = v
	}
	add(top, "ellipsis", orderedMap(header, headerKeys))
	if f.pullRequests != nil {
		add(top, "pull_requests", orderedMap(f.pullRequests, nil))
	}
	add(top, "reviewers", &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{reviewerNode(f, prompt)}})
	if f.output != nil {
		add(top, "output", orderedMap(f.output, nil))
	}
	if f.budget != nil {
		add(top, "budget", orderedMap(f.budget, nil))
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(top); err != nil {
		return "", fmt.Errorf("ellipsis: render code-review file: %w", err)
	}
	if err := enc.Close(); err != nil {
		return "", fmt.Errorf("ellipsis: render code-review file: %w", err)
	}
	return buf.String(), nil
}

func reviewerNode(f *reviewConfig, prompt string) *yaml.Node {
	reviewer := orderedMap(f.reviewer, reviewerOrder)
	engine, _ := f.reviewer[f.engine].(map[string]any)
	session := orderedMap(engine, []string{"model"})
	session.Content = append([]*yaml.Node{keyNode("prompt"), textNode(prompt)}, session.Content...)
	for i := 0; i+1 < len(reviewer.Content); i += 2 {
		if reviewer.Content[i].Value == f.engine {
			reviewer.Content[i+1] = session
		}
	}
	return reviewer
}

// textNode keeps multi-line text, a prompt or a script, as a block
// scalar, so the file reads like the specs it came from.
func textNode(text string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: text}
	if strings.Contains(text, "\n") {
		n.Style = yaml.LiteralStyle
	}
	return n
}

func mapping() *yaml.Node { return &yaml.Node{Kind: yaml.MappingNode} }

func keyNode(key string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
}

func add(m *yaml.Node, key string, value *yaml.Node) {
	m.Content = append(m.Content, keyNode(key), value)
}

// orderedMap lists the keys of m named in first, in that order, then the
// rest sorted. Values are encoded as they are.
func orderedMap(m map[string]any, first []string) *yaml.Node {
	out := mapping()
	done := map[string]bool{}
	emitKey := func(k string) {
		v, ok := m[k]
		if !ok || done[k] {
			return
		}
		done[k] = true
		var value yaml.Node
		if err := value.Encode(v); err != nil {
			value = yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: fmt.Sprint(v)}
		}
		add(out, k, &value)
	}
	for _, k := range first {
		emitKey(k)
	}
	for _, k := range sortedKeys(m) {
		emitKey(k)
	}
	return out
}
