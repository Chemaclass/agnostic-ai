package authority

import (
	"bytes"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// lineIndex finds where a declaration sits in its spec file, so a
// client can open the file at the rule. A line it cannot place is 0,
// which the contract omits.
type lineIndex struct {
	docs map[string]*specDoc
}

// specDoc is a spec file's YAML, or its frontmatter. Frontmatter is
// parsed from just after the opening `---`, so its line numbers are the
// file's.
type specDoc struct {
	root *yaml.Node
}

func newLineIndex() *lineIndex {
	return &lineIndex{docs: map[string]*specDoc{}}
}

func (li *lineIndex) doc(e spec.Entry) *specDoc {
	if e.Path == "" {
		return nil
	}
	if d, ok := li.docs[e.Path]; ok {
		return d
	}
	li.docs[e.Path] = nil
	data, err := os.ReadFile(e.Path)
	if err != nil {
		return nil
	}
	if bytes.HasPrefix(data, []byte("---")) {
		rest := data[3:]
		end := bytes.Index(rest, []byte("\n---"))
		if end < 0 {
			return nil
		}
		data = rest[:end]
	}
	var node yaml.Node
	if yaml.Unmarshal(data, &node) != nil || node.Kind != yaml.DocumentNode || len(node.Content) == 0 {
		return nil
	}
	d := &specDoc{root: node.Content[0]}
	li.docs[e.Path] = d
	return d
}

// value returns the node at path below the document's top mapping.
func (d *specDoc) value(path ...string) *yaml.Node {
	n := d.root
	for _, key := range path {
		if n == nil || n.Kind != yaml.MappingNode {
			return nil
		}
		var next *yaml.Node
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == key {
				next = n.Content[i+1]
			}
		}
		n = next
	}
	return n
}

// itemLines returns the line of each item of the list at path, or nil
// when the file's list does not have want items, as after a layer
// merged it.
func (li *lineIndex) itemLines(e spec.Entry, want int, path ...string) []int {
	d := li.doc(e)
	if d == nil {
		return nil
	}
	n := d.value(path...)
	if n == nil || n.Kind != yaml.SequenceNode || len(n.Content) != want {
		return nil
	}
	lines := make([]int, len(n.Content))
	for i, item := range n.Content {
		lines[i] = item.Line
	}
	return lines
}

// keyLine returns the line of a top-level key, or 0.
func (li *lineIndex) keyLine(e spec.Entry, key string) int {
	d := li.doc(e)
	if d == nil || d.root.Kind != yaml.MappingNode {
		return 0
	}
	for i := 0; i+1 < len(d.root.Content); i += 2 {
		if d.root.Content[i].Value == key {
			return d.root.Content[i].Line
		}
	}
	return 0
}
