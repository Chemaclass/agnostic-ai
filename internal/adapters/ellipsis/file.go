package ellipsis

import (
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	fileKind    = "code_review"
	defaultName = "code-review"
)

var (
	engines = []string{"claude_code", "codex", "gemini"}
	// settingsKeys are the x-ellipsis keys that map to a block of the
	// file. Ellipsis fails a sync on an unknown field, so any other key
	// stops the file here instead of there.
	settingsKeys = []string{"ellipsis", "pull_requests", "reviewer", "output", "budget", "environment"}
	// headerKeys are the keys of the file's `ellipsis` block.
	headerKeys = []string{"kind", "name", "description", "enabled", "metadata"}
)

// reviewConfig is the x-ellipsis block after validation.
type reviewConfig struct {
	name         string
	header       map[string]any
	pullRequests map[string]any
	reviewer     map[string]any
	engine       string
	output       map[string]any
	budget       map[string]any
}

// readFile validates the `x-ellipsis` block merged across settings specs.
// Every problem is returned, so one sync names all of them. environment
// is the selected environment the reviewer must run in, "" for none.
func readFile(settings []spec.Entry, environment string) (*reviewConfig, []string) {
	x := emit.SettingsCustomKeys(settings, target)
	var problems []string
	for _, key := range sortedKeys(x) {
		if !contains(settingsKeys, key) {
			problems = append(problems, fmt.Sprintf("x-ellipsis.%s is not a key of an Ellipsis review file (use %s)", key, strings.Join(settingsKeys, ", ")))
		}
	}
	file := &reviewConfig{name: defaultName}
	problems = append(problems, file.readHeader(x)...)
	file.pullRequests, problems = optionalBlock(x, "pull_requests", problems)
	file.output, problems = optionalBlock(x, "output", problems)
	file.budget, problems = optionalBlock(x, "budget", problems)
	problems = append(problems, file.readReviewer(x, environment)...)
	if len(problems) > 0 {
		return nil, problems
	}
	return file, nil
}

func (f *reviewConfig) readHeader(x map[string]any) []string {
	raw, held := x["ellipsis"]
	if !held {
		return nil
	}
	block, ok := raw.(map[string]any)
	if !ok {
		return []string{"x-ellipsis.ellipsis must be a mapping"}
	}
	var problems []string
	f.header = map[string]any{}
	for _, key := range sortedKeys(block) {
		if key != "kind" && key != "name" {
			f.header[key] = block[key]
		}
		switch key {
		case "kind":
			if block[key] != fileKind {
				problems = append(problems, "x-ellipsis.ellipsis.kind must be "+fileKind+", the only kind this file holds")
			}
		case "name":
			if name, ok := block[key].(string); ok && name != "" {
				f.name = name
			} else {
				problems = append(problems, "x-ellipsis.ellipsis.name must be a non-empty string")
			}
		case "description":
			if _, ok := block[key].(string); !ok {
				problems = append(problems, "x-ellipsis.ellipsis.description must be a string")
			}
		case "enabled":
			if _, ok := block[key].(bool); !ok {
				problems = append(problems, "x-ellipsis.ellipsis.enabled must be a boolean")
			}
		case "metadata":
			if _, ok := block[key].(map[string]any); !ok {
				problems = append(problems, "x-ellipsis.ellipsis.metadata must be a mapping")
			}
		default:
			problems = append(problems, fmt.Sprintf("x-ellipsis.ellipsis.%s is not an Ellipsis key (use %s)", key, strings.Join(headerKeys, ", ")))
		}
	}
	return problems
}

func optionalBlock(x map[string]any, key string, problems []string) (map[string]any, []string) {
	raw, held := x[key]
	if !held {
		return nil, problems
	}
	block, ok := raw.(map[string]any)
	if !ok {
		return nil, append(problems, fmt.Sprintf("x-ellipsis.%s must be a mapping", key))
	}
	return block, problems
}

// readReviewer checks the one reviewer. Ellipsis requires its name and
// model, and the prompt belongs to the review specs, so a prompt here
// would leave two sources for one field.
func (f *reviewConfig) readReviewer(x map[string]any, environment string) []string {
	raw, held := x["reviewer"]
	if !held {
		return []string{"x-ellipsis.reviewer is not set: add a settings spec whose x-ellipsis.reviewer names the reviewer, one engine block (claude_code, codex, or gemini), and its model"}
	}
	reviewer, ok := raw.(map[string]any)
	if !ok {
		return []string{"x-ellipsis.reviewer must be a mapping"}
	}
	f.reviewer = reviewer
	var problems []string
	if named, ok := reviewer["environment"]; ok && named == nil {
		f.keepBasicSandbox()
	} else if environment != "" {
		problems = append(problems, f.linkEnvironment(environment)...)
	}
	if name, ok := reviewer["name"].(string); !ok || name == "" {
		problems = append(problems, "x-ellipsis.reviewer.name is not set")
	}
	var found []string
	for _, engine := range engines {
		if _, ok := reviewer[engine]; ok {
			found = append(found, engine)
		}
	}
	switch len(found) {
	case 0:
		return append(problems, "x-ellipsis.reviewer needs a claude_code, codex, or gemini block")
	case 2:
		return append(problems, "x-ellipsis.reviewer takes only one of claude_code, codex, and gemini")
	}
	f.engine = found[0]
	block, ok := reviewer[f.engine].(map[string]any)
	if !ok {
		return append(problems, fmt.Sprintf("x-ellipsis.reviewer.%s must be a mapping with a model", f.engine))
	}
	if model, ok := block["model"].(string); !ok || model == "" {
		problems = append(problems, fmt.Sprintf("x-ellipsis.reviewer.%s.model is not set", f.engine))
	}
	if _, ok := block["prompt"]; ok {
		problems = append(problems, fmt.Sprintf("x-ellipsis.reviewer.%s.prompt is set, but the prompt comes from the review specs; remove it", f.engine))
	}
	return problems
}

// linkEnvironment points the reviewer at the selected environment. A
// reviewer that names another one is a conflict, not a choice made here.
func (f *reviewConfig) linkEnvironment(environment string) []string {
	if named, ok := f.reviewer["environment"]; ok && named != environment {
		return []string{fmt.Sprintf("x-ellipsis.reviewer.environment is %v, but x-ellipsis.environment.name is %s: keep one, or remove the reviewer key so the review follows the selection", named, environment)}
	}
	linked := maps.Clone(f.reviewer)
	linked["environment"] = environment
	f.reviewer = linked
	return nil
}

// keepBasicSandbox drops a null reviewer environment. Ellipsis runs a
// reviewer without one in its basic sandbox, and import writes the null so
// a selected environment does not take that reviewer over.
func (f *reviewConfig) keepBasicSandbox() {
	kept := maps.Clone(f.reviewer)
	delete(kept, "environment")
	f.reviewer = kept
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
