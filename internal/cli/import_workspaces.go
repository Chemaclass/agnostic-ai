package cli

import (
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// withWorkspaces returns the skill spec text with its top-level
// `workspaces` key listing scopes. A key that already lists them, in any
// order or notation, leaves the text as it is. One that lists others is
// replaced where it stands, with the comments around it, and a spec
// with none gets the key as its last line. It fails, naming why, when
// the frontmatter is not a block mapping or the edit would change
// anything but that key.
func withWorkspaces(text string, scopes []string) (string, error) {
	const delim = "---"
	if !strings.HasPrefix(text, delim+"\n") {
		return text, nil
	}
	after := text[len(delim):]
	end := strings.Index(after, "\n"+delim)
	if end < 0 {
		return text, nil
	}
	front := after[:end]
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(front), &doc); err != nil {
		return "", fmt.Errorf("parse frontmatter: %w", err)
	}
	var mapping *yaml.Node
	switch {
	case doc.Kind == yaml.DocumentNode && len(doc.Content) == 1 && doc.Content[0].Kind == yaml.MappingNode && doc.Content[0].Style&yaml.FlowStyle == 0:
		mapping = doc.Content[0]
	case strings.TrimSpace(front) == "":
	default:
		return "", fmt.Errorf("frontmatter is not a block mapping")
	}
	var key, value *yaml.Node
	if mapping != nil {
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			if mapping.Content[i].Value == "workspaces" {
				key, value = mapping.Content[i], mapping.Content[i+1]
			}
		}
	}
	if key == nil {
		return delim + front + "\n" + workspacesLine(scopes, "") + after[end:], nil
	}
	if sameWorkspaces(value, scopes) {
		return text, nil
	}
	if key.Column != 1 {
		return "", fmt.Errorf("`workspaces` is not at the start of its line")
	}
	lines := strings.Split(front, "\n")
	first := key.Line - 1
	last := frontmatterBlockEnd(lines, first)
	comment := key.LineComment
	if comment == "" && value.Line == key.Line {
		comment = value.LineComment
	}
	lines = slices.Replace(lines, first, last, workspacesLine(scopes, comment))
	out := delim + strings.Join(lines, "\n") + after[end:]
	if err := onlyWorkspacesChanged(text, out, scopes); err != nil {
		return "", err
	}
	return out, nil
}

// workspacesLine renders the `workspaces` key as a flow list, with the
// comment that sat on the line it replaces.
func workspacesLine(scopes []string, comment string) string {
	items := make([]string, len(scopes))
	for i, scope := range scopes {
		items[i] = encodeYAMLScalar(scope)
	}
	line := "workspaces: [" + strings.Join(items, ", ") + "]"
	if comment != "" {
		line += " " + comment
	}
	return line
}

// sameWorkspaces reports whether value lists exactly the directories in
// scopes, ignoring order, repeats, and a trailing slash, as the Cursor
// adapter does when it picks where to copy the skill.
func sameWorkspaces(value *yaml.Node, scopes []string) bool {
	var listed []string
	if value.Kind != yaml.SequenceNode || value.Decode(&listed) != nil {
		return false
	}
	return slices.Equal(workspaceSet(listed), workspaceSet(scopes))
}

func workspaceSet(dirs []string) []string {
	set := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		set = append(set, strings.Trim(filepath.ToSlash(dir), "/"))
	}
	slices.Sort(set)
	return slices.Compact(set)
}

// onlyWorkspacesChanged checks that after holds before's frontmatter with
// nothing but `workspaces` set to scopes, and the same body. A key written
// twice counts once, with its last value, as the loader reads it.
func onlyWorkspacesChanged(before, after string, scopes []string) error {
	was, err := spec.ParseMarkdownBytes(spec.KindSkill, []byte(before))
	if err != nil {
		return err
	}
	now, err := spec.ParseMarkdownBytes(spec.KindSkill, []byte(after))
	if err != nil {
		return fmt.Errorf("the edit does not parse: %w", err)
	}
	items := make([]any, len(scopes))
	for i, scope := range scopes {
		items[i] = scope
	}
	was.Meta["workspaces"] = items
	if was.Body != now.Body || !reflect.DeepEqual(was.Meta, now.Meta) {
		return fmt.Errorf("the edit changes more than `workspaces`")
	}
	return nil
}
