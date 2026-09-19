package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// devinScopeToPortable reverses the vocabulary the windsurf adapter
// emits. Devin's `Exec` is a prefix matcher with no exact-command form,
// so it reads back as Claude's prefix spelling `Bash(x:*)` rather than
// the exact `Bash(x)`, which would narrow a rule the project already
// applies more widely.
var devinScopeToPortable = map[string]func(string) string{
	"Read":  func(arg string) string { return "Read(" + arg + ")" },
	"Write": func(arg string) string { return "Write(" + arg + ")" },
	"Fetch": func(arg string) string { return "WebFetch(" + arg + ")" },
	"Exec":  func(arg string) string { return "Bash(" + arg + ":*)" },
}

// devinBareTool reverses the bare tool names Devin publishes. `write`
// covers both portable Write and Edit on the way out, so the import
// picks Write: it is the wider of the two and re-emitting it produces
// the same Devin rule.
var devinBareTool = map[string]string{
	"read": "Read", "edit": "Edit", "grep": "Grep",
	"glob": "Glob", "exec": "Bash",
}

// portableDevinRule turns one Devin rule back into the portable
// spelling, or reports that it has none.
func portableDevinRule(rule string) (string, bool) {
	if strings.HasPrefix(rule, spec.MCPToolPrefix) {
		return rule, true
	}
	if scope, arg, ok := spec.SplitPermissionRule(rule); ok {
		if build, known := devinScopeToPortable[scope]; known {
			return build(arg), true
		}
		return "", false
	}
	if name, ok := devinBareTool[rule]; ok {
		return name, true
	}
	return "", false
}

// importWindsurfPermissions reads the committed project policy from
// `.devin/config.json` and writes it back as one Settings spec. Devin
// documents that file as the shareable half of its permission model
// ("Allow for project | `.devin/config.json` | Yes"), so a team that
// adopted agnostic-ai after writing one keeps it (#872).
func importWindsurfPermissions(root, dstDir string) (int, error) {
	var doc struct {
		Permissions map[string][]string `json:"permissions"`
	}
	src := filepath.Join(root, ".devin", "config.json")
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return 0, fmt.Errorf("parse %s: %w", src, err)
	}
	lists := map[string][]string{}
	for _, list := range []string{"allow", "deny", "ask"} {
		for _, rule := range doc.Permissions[list] {
			if portable, ok := portableDevinRule(rule); ok {
				lists[list] = appendUnique(lists[list], portable)
			}
		}
	}
	return writePermissionsSpec(dstDir, "permissions", lists)
}

// augmentBareTool reverses the six tool names Augment publishes. A rule
// matching on `shellInputRegex` has no portable spelling, so it is left
// in place rather than guessed at.
var augmentBareTool = map[string]string{
	"read": "Read", "edit": "Edit", "write": "Write",
	"terminal": "Bash", "web-fetch": "WebFetch", "web-search": "WebSearch",
}

// importAugmentPermissions reads `toolPermissions` from
// `.augment/settings.json`, the file Augment calls "the recommended way
// to enforce an organizational policy ... on every cloud agent that
// runs there", and writes the entries that have a portable spelling.
// Entries carrying a `shellInputRegex` are skipped: reversing a regex
// into a command rule would invent a boundary the project never wrote.
func importAugmentPermissions(root, dstDir string) (int, error) {
	var doc struct {
		ToolPermissions []struct {
			ToolName   string          `json:"toolName"`
			Permission json.RawMessage `json:"permission"`
			ShellRegex string          `json:"shellInputRegex"`
		} `json:"toolPermissions"`
	}
	src := filepath.Join(root, ".augment", "settings.json")
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return 0, fmt.Errorf("parse %s: %w", src, err)
	}
	lists := map[string][]string{}
	for _, entry := range doc.ToolPermissions {
		if entry.ShellRegex != "" {
			continue
		}
		portable, ok := augmentBareTool[entry.ToolName]
		if !ok {
			continue
		}
		list, ok := augmentPermissionList(entry.Permission)
		if !ok {
			continue
		}
		lists[list] = appendUnique(lists[list], portable)
	}
	return writePermissionsSpec(dstDir, "permissions-augment", lists)
}

// augmentPermissionList reads the permission type. Augment requires an
// object with a `type` field: "A rule with a bare-string permission is
// malformed and is dropped", so a bare string is not accepted here
// either.
func augmentPermissionList(raw json.RawMessage) (string, bool) {
	var obj struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", false
	}
	switch obj.Type {
	case "allow", "deny":
		return obj.Type, true
	}
	return "", false
}

// writePermissionsSpec writes one Settings spec holding the three
// lists, or nothing when every list is empty.
func writePermissionsSpec(dstDir, name string, lists map[string][]string) (int, error) {
	if len(lists) == 0 {
		return 0, nil
	}
	permissions := map[string][]string{}
	for list, rules := range lists {
		if len(rules) > 0 {
			permissions[list] = rules
		}
	}
	if len(permissions) == 0 {
		return 0, nil
	}
	body, err := yaml.Marshal(map[string]any{"permissions": permissions})
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return 0, err
	}
	path := filepath.Join(dstDir, name+".yaml")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return 0, fmt.Errorf("write %s: %w", path, err)
	}
	return 1, nil
}

// appendUnique keeps source order and drops a repeat, so two native
// rules that collapse onto one portable spelling write it once.
func appendUnique(list []string, value string) []string {
	if slices.Contains(list, value) {
		return list
	}
	return append(list, value)
}
