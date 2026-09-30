package cli

import (
	"maps"
	"reflect"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

var topLevelModelLineRE = regexp.MustCompile(`^((?:model|'model'|"model")[ \t]*:)[ \t]*(\S.*?)([ \t]+#.*)?$`)

// scopeClaudeModel rewrites a top-level `model:` holding a Claude model
// name as `model: {claude: <name>}`, so every other target falls back to
// its own default model instead of loading one it cannot run. Every other
// line stays as written. The document comes back unchanged when the model
// is not a Claude name or the edit does not parse to the same data.
func scopeClaudeModel(doc string) string {
	if !strings.HasPrefix(doc, "---\n") {
		return doc
	}
	rest := doc[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return doc
	}
	var before map[string]any
	if err := yaml.Unmarshal([]byte(rest[:end]), &before); err != nil {
		return doc
	}
	model, _ := before["model"].(string)
	if !adapters.ClaudeModel(model) {
		return doc
	}
	want := maps.Clone(before)
	want["model"] = map[string]any{"claude": model}

	lines := strings.Split(rest[:end], "\n")
	for i, line := range lines {
		m := topLevelModelLineRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key, value, comment := m[1], m[2], m[3]
		for _, scoped := range []string{
			key + " {claude: " + value + "}" + comment,
			key + "\n  claude: " + value + comment,
		} {
			edited := append(append(append([]string{}, lines[:i]...), scoped), lines[i+1:]...)
			front := strings.Join(edited, "\n")
			var after map[string]any
			if err := yaml.Unmarshal([]byte(front), &after); err == nil && reflect.DeepEqual(after, want) {
				return "---\n" + front + rest[end:]
			}
		}
		break
	}
	return doc
}
