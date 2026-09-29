package cli

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

// jsoncObjectToYAML converts a JSON object with comments into a YAML
// mapping node that keeps key order and turns each `//` or `/* */`
// comment into a YAML comment: a comment on its own lines heads the key
// or list item after it, and one after a value on the same line trails
// that value. A comment inside an empty object, or after the last key of
// the file, becomes a foot comment of the last key; one in an empty
// object is dropped.
func jsoncObjectToYAML(data []byte) (*yaml.Node, error) {
	stripped, _ := adapters.StripJSONC(data)
	var doc map[string]any
	if err := json.Unmarshal(stripped, &doc); err != nil {
		return nil, err
	}
	// The data is valid JSON with comments, so the parser below can
	// trust its shape.
	p := &jsoncNodes{src: data}
	head := p.comments()
	root := p.object()
	if n := len(root.Content); n > 0 {
		root.Content[0].HeadComment = joinComments(head, root.Content[0].HeadComment)
		root.Content[n-2].FootComment = joinComments([]string{root.Content[n-2].FootComment}, strings.Join(p.comments(), "\n"))
	}
	return root, nil
}

type jsoncNodes struct {
	src []byte
	pos int
}

func (p *jsoncNodes) peek() byte {
	if p.pos < len(p.src) {
		return p.src[p.pos]
	}
	return 0
}

func (p *jsoncNodes) startsComment() bool {
	return p.pos+1 < len(p.src) && p.src[p.pos] == '/' && (p.src[p.pos+1] == '/' || p.src[p.pos+1] == '*')
}

// comments skips whitespace and comments, and returns each comment.
func (p *jsoncNodes) comments() []string {
	var out []string
	for p.pos < len(p.src) {
		switch c := p.src[p.pos]; {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			p.pos++
		case p.startsComment():
			out = append(out, p.comment())
		default:
			return out
		}
	}
	return out
}

// comment reads the comment at pos as YAML comment lines.
func (p *jsoncNodes) comment() string {
	if p.src[p.pos+1] == '/' {
		end := bytes.IndexByte(p.src[p.pos:], '\n')
		if end < 0 {
			end = len(p.src) - p.pos
		}
		text := string(p.src[p.pos+2 : p.pos+end])
		p.pos += end
		return "#" + strings.TrimRight(text, " \t\r")
	}
	start := p.pos + 2
	end := bytes.Index(p.src[start:], []byte("*/"))
	p.pos = start + end + 2
	var lines []string
	for i, line := range strings.Split(string(p.src[start:start+end]), "\n") {
		line = strings.TrimSpace(line)
		if i > 0 {
			line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
		}
		if line == "" {
			lines = append(lines, "#")
		} else {
			lines = append(lines, "# "+line)
		}
	}
	for len(lines) > 0 && lines[0] == "#" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1] == "#" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// trailing reads the optional comma and one-line comment after a value.
func (p *jsoncNodes) trailing() string {
	p.skipInline()
	if p.peek() == ',' {
		p.pos++
		p.skipInline()
	}
	if !p.startsComment() {
		return ""
	}
	saved := p.pos
	c := p.comment()
	if strings.Contains(c, "\n") {
		p.pos = saved
		return ""
	}
	return c
}

func (p *jsoncNodes) skipInline() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t' || p.src[p.pos] == '\r') {
		p.pos++
	}
}

// heads reads the comments before the next member, past a comma a
// comment pushed onto its own line.
func (p *jsoncNodes) heads() []string {
	out := p.comments()
	if p.peek() == ',' {
		p.pos++
		out = append(out, p.comments()...)
	}
	return out
}

func (p *jsoncNodes) object() *yaml.Node {
	p.pos++ // {
	n := &yaml.Node{Kind: yaml.MappingNode}
	for {
		heads := p.heads()
		if p.peek() == '}' {
			p.pos++
			if k := len(n.Content); k > 0 {
				n.Content[k-2].FootComment = joinComments([]string{n.Content[k-2].FootComment}, strings.Join(heads, "\n"))
			}
			break
		}
		key := p.value()
		heads = append(heads, p.comments()...)
		p.pos++ // :
		heads = append(heads, p.comments()...)
		key.HeadComment = joinComments(heads, "")
		val := p.value()
		attachTrailing(key, val, p.trailing())
		n.Content = append(n.Content, key, val)
	}
	if len(n.Content) == 0 {
		n.Style = yaml.FlowStyle
	}
	return n
}

func (p *jsoncNodes) array() *yaml.Node {
	p.pos++ // [
	n := &yaml.Node{Kind: yaml.SequenceNode}
	for {
		heads := p.heads()
		if p.peek() == ']' {
			p.pos++
			break
		}
		item := p.value()
		item.HeadComment = joinComments(heads, "")
		attachTrailing(item, item, p.trailing())
		n.Content = append(n.Content, item)
	}
	if len(n.Content) == 0 {
		n.Style = yaml.FlowStyle
	}
	return n
}

// attachTrailing puts a comment that follows a value on that value when
// it is a scalar. After a mapping or list it goes under owner instead,
// since a YAML line comment cannot follow a block.
func attachTrailing(owner, val *yaml.Node, comment string) {
	switch {
	case comment == "":
	case val.Kind == yaml.ScalarNode:
		val.LineComment = comment
	default:
		owner.FootComment = joinComments([]string{owner.FootComment}, comment)
	}
}

func (p *jsoncNodes) value() *yaml.Node {
	switch p.peek() {
	case '{':
		return p.object()
	case '[':
		return p.array()
	case '"':
		end := p.pos + 1
		for p.src[end] != '"' {
			if p.src[end] == '\\' {
				end++
			}
			end++
		}
		end++
		var s string
		_ = json.Unmarshal(p.src[p.pos:end], &s)
		p.pos = end
		var n yaml.Node
		_ = n.Encode(s)
		return &n
	}
	start := p.pos
	for p.pos < len(p.src) && !strings.ContainsRune(",}] \t\r\n/", rune(p.src[p.pos])) {
		p.pos++
	}
	return literalNode(string(p.src[start:p.pos]))
}

// literalNode reads a JSON number, true, false, or null.
func literalNode(text string) *yaml.Node {
	var n yaml.Node
	switch text {
	case "null":
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	case "true", "false":
		_ = n.Encode(text == "true")
	default:
		if i, err := strconv.ParseInt(text, 10, 64); err == nil {
			_ = n.Encode(i)
		} else {
			f, _ := strconv.ParseFloat(text, 64)
			_ = n.Encode(f)
		}
	}
	return &n
}

// joinComments joins comment blocks with a newline, dropping empty ones.
func joinComments(blocks []string, more string) string {
	var out []string
	for _, b := range append(blocks, more) {
		if b != "" {
			out = append(out, b)
		}
	}
	return strings.Join(out, "\n")
}
