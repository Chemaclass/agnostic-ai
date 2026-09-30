package cli

import (
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

var topLevelModelLineRE = regexp.MustCompile(`^((?:model|'model'|"model")[ \t]*:)[ \t]*(\S.*?)([ \t]+#.*)?$`)

// scopeClaudeModel rewrites a top-level `model:` holding a Claude model
// name as `model: {claude: <name>}`, so every other target falls back to
// its own default model instead of loading one it cannot run. Every other
// line stays as written. The document comes back unchanged when the model
// is not a Claude name or the edit does not parse to the same data.
func scopeClaudeModel(doc string) string {
	rest, end, before := claudeFrontmatter(doc)
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

// claudeFrontmatter returns the text after the opening `---` line, the
// offset where the frontmatter ends in it, and the parsed frontmatter.
// The map is nil when doc has no frontmatter that parses.
func claudeFrontmatter(doc string) (string, int, map[string]any) {
	rest, ok := strings.CutPrefix(doc, "---\n")
	if !ok {
		return "", 0, nil
	}
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", 0, nil
	}
	var meta map[string]any
	if err := yaml.Unmarshal([]byte(rest[:end]), &meta); err != nil {
		return "", 0, nil
	}
	return rest, end, meta
}

// noteRepeatedClaudeModels suggests a `models:` tier for each Claude model
// name that two or more Claude agents in dir set, so the name lives in
// one place once the project targets more tools.
func noteRepeatedClaudeModels(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	counts := map[string]int{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		_, _, meta := claudeFrontmatter(header.Strip(string(data)))
		if model, _ := meta["model"].(string); model != "inherit" && adapters.ClaudeModel(model) {
			counts[model]++
		}
	}
	models := make([]string, 0, len(counts))
	for model, n := range counts {
		if n > 1 {
			models = append(models, model)
		}
	}
	sort.Strings(models)
	for _, model := range models {
		summaryf("  → %d agents set model %s; to name it once, add models: {<tier>: {claude: %s}} to %s and write model: <tier> in each\n",
			counts[model], model, model, config.ConfigFileName)
	}
}
