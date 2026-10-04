package cli

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

func hasWebPair(list []any) bool {
	for i := 0; i+1 < len(list); i++ {
		if list[i] == "WebFetch" && list[i+1] == "WebSearch" {
			return true
		}
	}
	return false
}

func collapseWebPairs(src string, path []string) (string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		return "", err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return "", fmt.Errorf("not a mapping")
	}
	node := doc.Content[0]
	for _, key := range path {
		if node.Kind != yaml.MappingNode {
			return "", fmt.Errorf("not a mapping")
		}
		_, node = topLevelPair(node, key)
		if node == nil {
			return src, nil
		}
	}
	if node.Kind != yaml.SequenceNode {
		return src, nil
	}
	lines := strings.SplitAfter(src, "\n")
	var edits []yamlSplice
	for i := 0; i+1 < len(node.Content); i++ {
		first, second := node.Content[i], node.Content[i+1]
		if first.Value != "WebFetch" || second.Value != "WebSearch" {
			continue
		}
		if first.Anchor != "" || second.Anchor != "" || second.HeadComment != "" || second.FootComment != "" {
			return "", fmt.Errorf("web pair has an anchor or comment; combine it by hand")
		}
		a := []rune(lines[first.Line-1])
		end, ok := scalarEnd(a, first.Column-1, first)
		if !ok {
			return "", fmt.Errorf("WebFetch is not a one-line scalar")
		}
		if node.Style&yaml.FlowStyle != 0 {
			if first.Line != second.Line {
				return "", fmt.Errorf("web pair spans flow lines; combine it by hand")
			}
			endSecond, ok := scalarEnd(a, second.Column-1, second)
			if !ok {
				return "", fmt.Errorf("WebSearch is not a one-line scalar")
			}
			edits = append(edits, yamlSplice{line: first.Line - 1, start: first.Column - 1, end: endSecond, text: quoteLike(first.Style, "web")})
		} else {
			if second.LineComment != "" {
				return "", fmt.Errorf("web pair has a comment; combine it by hand")
			}
			edits = append(edits, yamlSplice{line: first.Line - 1, start: first.Column - 1, end: end, text: quoteLike(first.Style, "web")}, yamlSplice{line: second.Line - 1, start: 0, end: len([]rune(lines[second.Line-1])), text: ""})
		}
		i++
	}
	slices.SortFunc(edits, func(a, b yamlSplice) int {
		if a.line != b.line {
			return b.line - a.line
		}
		return b.start - a.start
	})
	for _, ed := range edits {
		line := []rune(lines[ed.line])
		lines[ed.line] = string(line[:ed.start]) + ed.text + string(line[ed.end:])
	}
	out := strings.Join(lines, "")
	var check yaml.Node
	if err := yaml.Unmarshal([]byte(out), &check); err != nil {
		return "", fmt.Errorf("web rewrite does not parse: %w", err)
	}
	var before, after any
	if err := yaml.Unmarshal([]byte(src), &before); err != nil {
		return "", err
	}
	if err := yaml.Unmarshal([]byte(out), &after); err != nil {
		return "", err
	}
	current := before
	for _, key := range path[:len(path)-1] {
		mapping, ok := current.(map[string]any)
		if !ok {
			return "", fmt.Errorf("not a mapping")
		}
		current = mapping[key]
	}
	parent, ok := current.(map[string]any)
	if !ok {
		return "", fmt.Errorf("not a mapping")
	}
	list, ok := parent[path[len(path)-1]].([]any)
	if !ok {
		return "", fmt.Errorf("not a list")
	}
	var wanted []any
	for i := 0; i < len(list); i++ {
		if i+1 < len(list) && list[i] == "WebFetch" && list[i+1] == "WebSearch" {
			wanted = append(wanted, "web")
			i++
		} else {
			wanted = append(wanted, list[i])
		}
	}
	parent[path[len(path)-1]] = wanted
	if !reflect.DeepEqual(before, after) {
		return "", fmt.Errorf("web rewrite changes other values")
	}
	return out, nil
}

func withoutAdjacentWebPair(list []string) []string {
	var out []string
	for i := 0; i < len(list); i++ {
		if i+1 < len(list) && list[i] == "WebFetch" && list[i+1] == "WebSearch" {
			i++
			continue
		}
		out = append(out, list[i])
	}
	return out
}
