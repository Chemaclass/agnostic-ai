package approvals

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// KiroReviewPreset is the policy preset that turns on Kiro's reviewer,
// Approve for me (kiro.dev/docs/permissions/#approve-for-me).
const KiroReviewPreset = "approve-for-me"

const kiroPoliciesKey = "policies"

var kiroDocumentEnd = regexp.MustCompile(`(?m)^\.\.\.(?:[ \t\r]|$)`)

// KiroPolicyOwned is what an add put in a permissions file beyond the
// item itself, so removing the item restores the bytes it found.
type KiroPolicyOwned struct {
	CreatedKey   bool `json:"createdKey,omitempty"`
	AddedNewline bool `json:"addedNewline,omitempty"`
}

// KiroPolicyAdd is a permissions file after AddKiroPolicy.
type KiroPolicyAdd struct {
	Data    []byte
	Changed bool
	Owned   KiroPolicyOwned
}

// kiroPolicies is the policies entry of a permissions file: key is nil
// when the file has none, and list is the sequence or the empty value.
type kiroPolicies struct {
	key, list *yaml.Node
}

func (k kiroPolicies) item(preset string) *yaml.Node {
	if k.list == nil || k.list.Kind != yaml.SequenceNode {
		return nil
	}
	for _, item := range k.list.Content {
		if item.Kind == yaml.ScalarNode && item.Value == preset {
			return item
		}
	}
	return nil
}

// parseKiroPolicies finds the policies entry. It refuses a file it
// cannot edit line by line: not one block mapping at the left margin,
// or a policies value that is neither a list nor empty.
func parseKiroPolicies(path string, data []byte) (kiroPolicies, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := decoder.Decode(&doc); errors.Is(err, io.EOF) {
		return kiroPolicies{}, nil
	} else if err != nil {
		return kiroPolicies{}, fmt.Errorf("parse %s: %w", path, err)
	}
	var next yaml.Node
	if err := decoder.Decode(&next); !errors.Is(err, io.EOF) {
		return kiroPolicies{}, fmt.Errorf("parse %s: holds more than one YAML document", path)
	}
	if len(doc.Content) == 0 {
		return kiroPolicies{}, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode || root.Style&yaml.FlowStyle != 0 || root.Column != 1 {
		return kiroPolicies{}, fmt.Errorf("%s: not a block mapping at the left margin", path)
	}
	var policies kiroPolicies
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Tag == "!!merge" {
			return kiroPolicies{}, fmt.Errorf("%s: top-level YAML merge keys cannot be edited in place", path)
		}
		if root.Content[i].Value != kiroPoliciesKey {
			continue
		}
		if policies.key != nil {
			return kiroPolicies{}, fmt.Errorf("%s: more than one %s key", path, kiroPoliciesKey)
		}
		key, list := root.Content[i], root.Content[i+1]
		empty := list.Kind == yaml.ScalarNode && list.Tag == "!!null" && list.Value == ""
		if list.Kind != yaml.SequenceNode && !empty {
			return kiroPolicies{}, fmt.Errorf("%s: %s is not a list", path, kiroPoliciesKey)
		}
		policies = kiroPolicies{key: key, list: list}
	}
	return policies, nil
}

// KiroListsPolicy reports whether the policies list in data holds preset.
func KiroListsPolicy(path string, data []byte, preset string) (bool, error) {
	policies, err := parseKiroPolicies(path, data)
	if err != nil {
		return false, err
	}
	return policies.item(preset) != nil, nil
}

// AddKiroPolicy lists preset under policies, editing the text in place
// so every other byte stays. A file that already lists it is unchanged.
func AddKiroPolicy(path string, data []byte, preset string) (KiroPolicyAdd, error) {
	policies, err := parseKiroPolicies(path, data)
	if err != nil {
		return KiroPolicyAdd{}, err
	}
	if policies.item(preset) != nil {
		return KiroPolicyAdd{Data: data}, nil
	}
	eol := lineEnd(data)
	text := string(data)
	var owned KiroPolicyOwned
	switch {
	case policies.key == nil:
		owned.CreatedKey = true
		at := len(text)
		if end := kiroDocumentEnd.FindStringIndex(text); end != nil {
			at = end[0]
		} else if text != "" && !strings.HasSuffix(text, "\n") {
			text += eol
			at = len(text)
			owned.AddedNewline = true
		}
		text = text[:at] + kiroPoliciesKey + ":" + eol + "  - " + preset + eol + text[at:]
	case policies.list.Kind == yaml.SequenceNode && policies.list.Style&yaml.FlowStyle != 0:
		at, err := offsetOf(path, text, policies.list.Line, policies.list.Column)
		if err != nil {
			return KiroPolicyAdd{}, err
		}
		if !strings.HasPrefix(text[at:], "[") {
			return KiroPolicyAdd{}, fmt.Errorf("%s: cannot find the %s list", path, kiroPoliciesKey)
		}
		item := preset
		if len(policies.list.Content) > 0 {
			item += ", "
		}
		text = text[:at+1] + item + text[at+1:]
	default:
		indent := policies.key.Column + 1
		if policies.list.Kind == yaml.SequenceNode {
			indent = policies.list.Column - 1
		}
		starts := lineStarts(text)
		at := len(text)
		if policies.key.Line < len(starts) {
			at = starts[policies.key.Line]
		} else if !strings.HasSuffix(text, "\n") {
			text += eol
			at = len(text)
			owned.AddedNewline = true
		}
		text = text[:at] + strings.Repeat(" ", indent) + "- " + preset + eol + text[at:]
	}
	edited := []byte(text)
	listed, err := KiroListsPolicy(path, edited, preset)
	if err != nil {
		return KiroPolicyAdd{}, err
	}
	if !listed {
		return KiroPolicyAdd{}, fmt.Errorf("%s: could not list %s under %s", path, preset, kiroPoliciesKey)
	}
	restored, err := RemoveKiroPolicy(path, edited, preset, owned)
	if err != nil {
		return KiroPolicyAdd{}, err
	}
	if !bytes.Equal(restored, data) {
		return KiroPolicyAdd{}, fmt.Errorf("%s: cannot edit the %s list in place", path, kiroPoliciesKey)
	}
	return KiroPolicyAdd{Data: edited, Changed: true, Owned: owned}, nil
}

// RemoveKiroPolicy takes preset out of the policies list, along with
// the key and final newline an add recorded in owned once nothing else
// needs them. A file without the item is unchanged.
func RemoveKiroPolicy(path string, data []byte, preset string, owned KiroPolicyOwned) ([]byte, error) {
	policies, err := parseKiroPolicies(path, data)
	if err != nil {
		return nil, err
	}
	item := policies.item(preset)
	if item == nil {
		return data, nil
	}
	text := string(data)
	atEnd := false
	if policies.list.Style&yaml.FlowStyle != 0 {
		if text, err = removeFlowItem(path, text, item); err != nil {
			return nil, err
		}
	} else {
		pattern := regexp.MustCompile(`^ *- +(` + regexp.QuoteMeta(preset) + `|'` + regexp.QuoteMeta(preset) + `'|"` + regexp.QuoteMeta(preset) + `")[ \t]*\r?$`)
		if text, atEnd, err = removeLine(path, text, item.Line, pattern); err != nil {
			return nil, err
		}
		if owned.CreatedKey {
			policies, err := parseKiroPolicies(path, []byte(text))
			if err != nil {
				return nil, err
			}
			if policies.key != nil && policies.list.Kind == yaml.ScalarNode {
				keyLine := regexp.MustCompile(`^` + kiroPoliciesKey + `:[ \t]*\r?$`)
				var keyAtEnd bool
				if text, keyAtEnd, err = removeLine(path, text, policies.key.Line, keyLine); err != nil {
					return nil, err
				}
				atEnd = atEnd || keyAtEnd
			}
		}
	}
	if owned.AddedNewline && atEnd {
		text = strings.TrimSuffix(strings.TrimSuffix(text, "\n"), "\r")
	}
	edited := []byte(text)
	listed, err := KiroListsPolicy(path, edited, preset)
	if err != nil {
		return nil, err
	}
	if listed {
		return nil, fmt.Errorf("%s: more than one copy of %s under %s", path, preset, kiroPoliciesKey)
	}
	return edited, nil
}

// removeFlowItem cuts item and the comma that joins it to a neighbor
// out of a flow list.
func removeFlowItem(path, text string, item *yaml.Node) (string, error) {
	at, err := offsetOf(path, text, item.Line, item.Column)
	if err != nil {
		return "", err
	}
	token := item.Value
	if item.Style&(yaml.SingleQuotedStyle|yaml.DoubleQuotedStyle) != 0 {
		token = text[at:at+1] + token + text[at:at+1]
	}
	if !strings.HasPrefix(text[at:], token) {
		return "", fmt.Errorf("%s: cannot find %s in the %s list", path, item.Value, kiroPoliciesKey)
	}
	from, to := at, at+len(token)
	if rest := strings.TrimLeft(text[to:], " \t"); strings.HasPrefix(rest, ",") {
		to = len(text) - len(strings.TrimPrefix(rest[1:], " "))
	} else if head := strings.TrimRight(text[:from], " \t"); strings.HasSuffix(head, ",") {
		from = len(head) - 1
	}
	return text[:from] + text[to:], nil
}

// removeLine deletes the 1-based line when it matches pattern, and says
// whether that line was the last one.
func removeLine(path, text string, line int, pattern *regexp.Regexp) (string, bool, error) {
	starts := lineStarts(text)
	if line < 1 || line > len(starts) {
		return "", false, fmt.Errorf("%s: line %d is out of range", path, line)
	}
	from, to := starts[line-1], len(text)
	if line < len(starts) {
		to = starts[line]
	}
	if !pattern.MatchString(strings.TrimSuffix(text[from:to], "\n")) {
		return "", false, fmt.Errorf("%s: line %d holds more than the %s entry agnostic-ai wrote", path, line, kiroPoliciesKey)
	}
	return text[:from] + text[to:], to == len(text), nil
}

// lineStarts is the byte offset where each line begins.
func lineStarts(text string) []int {
	starts := []int{0}
	for i := range len(text) {
		if text[i] == '\n' && i+1 < len(text) {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// offsetOf turns a 1-based line and rune column into a byte offset.
func offsetOf(path, text string, line, column int) (int, error) {
	starts := lineStarts(text)
	if line < 1 || line > len(starts) {
		return 0, fmt.Errorf("%s: line %d is out of range", path, line)
	}
	at := starts[line-1]
	for range column - 1 {
		if at >= len(text) {
			return 0, fmt.Errorf("%s: column %d of line %d is out of range", path, column, line)
		}
		_, size := utf8.DecodeRuneInString(text[at:])
		at += size
	}
	return at, nil
}

func lineEnd(data []byte) string {
	if bytes.Contains(data, []byte("\r\n")) {
		return "\r\n"
	}
	return "\n"
}
