package ellipsis

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/preview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	environmentKind = "environment"
	// sandboxRoot is where Ellipsis checks a repository out:
	// "Each repository is checked out at `/sandbox/<name>`".
	sandboxRoot = "/sandbox/"
)

var (
	// environmentKeys are the keys of x-ellipsis.environment. The portable
	// environment and MCP specs fill the scripts and servers; everything
	// else Ellipsis keeps in an environment lives here.
	environmentKeys = []string{"name", "repository", "description", "metadata", "variables", "compute", "hooks", "mcp_servers"}
	hookNames       = []string{"build_base", "after_checkout", "before_start", "validate"}
	// integrations are the connected services an environment lists by name.
	integrations = []string{"slack", "linear"}
	// reservedVariablePrefixes and reservedVariables are names Ellipsis
	// rejects in `variables`.
	reservedVariablePrefixes = []string{"ANTHROPIC_", "OPENAI_", "CODEX_", "CLAUDE_CODE_USE_"}
	reservedVariables        = []string{"CLAUDE_CODE_OAUTH_TOKEN", "CLAUDE_CONFIG_DIR", "ELLIPSIS_LLM_PROXY_URL"}

	environmentName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
	repositoryName  = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	variableName    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// environmentConfig is the x-ellipsis.environment block after validation:
// the one environment this project selects, its one repository, and the
// native options no portable spec carries.
type environmentConfig struct {
	name        string
	repository  map[string]any
	repoName    string
	description string
	metadata    map[string]any
	variables   []any
	compute     map[string]any
	hooks       nativeHooks
	integration []string
}

// nativeHooks are the lifecycle scripts written in x-ellipsis, as given.
// A script here replaces the portable field for the same stage, and
// naming both is a conflict.
type nativeHooks struct {
	buildBase     string
	inputs        []string
	hasInputs     bool
	afterCheckout string
	beforeStart   string
	validate      string
}

// readEnvironment validates the `x-ellipsis.environment` block merged
// across settings specs. selected is false when no spec sets the block.
// Every problem is returned, so one sync names all of them; env then holds
// whatever read cleanly, such as the name.
func readEnvironment(settings []spec.Entry) (env *environmentConfig, problems []string, selected bool) {
	raw, held := emit.SettingsCustomKeys(settings, target)["environment"]
	if !held {
		return nil, nil, false
	}
	block, ok := raw.(map[string]any)
	if !ok {
		return &environmentConfig{}, []string{"x-ellipsis.environment must be a mapping"}, true
	}
	env = &environmentConfig{}
	for _, key := range sortedKeys(block) {
		switch {
		case slices.Contains(environmentKeys, key):
		case key == "repositories":
			problems = append(problems, "x-ellipsis.environment.repositories is not supported: this target checks out one repository, so set x-ellipsis.environment.repository")
		default:
			problems = append(problems, fmt.Sprintf("x-ellipsis.environment.%s is not a key of an Ellipsis environment (use %s)", key, strings.Join(environmentKeys, ", ")))
		}
	}
	problems = append(problems, env.readName(block)...)
	problems = append(problems, env.readRepository(block)...)
	problems = append(problems, env.readDescription(block)...)
	env.metadata, problems = optionalMapping(block, "metadata", "x-ellipsis.environment.metadata", problems)
	problems = append(problems, env.readVariables(block)...)
	problems = append(problems, env.readCompute(block)...)
	problems = append(problems, env.readHooks(block)...)
	problems = append(problems, env.readIntegrations(block)...)
	return env, problems, true
}

func optionalMapping(block map[string]any, key, path string, problems []string) (map[string]any, []string) {
	raw, held := block[key]
	if !held {
		return nil, problems
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, append(problems, path+" must be a mapping")
	}
	return m, problems
}

func (e *environmentConfig) readName(block map[string]any) []string {
	raw, held := block["name"]
	if !held {
		return []string{"x-ellipsis.environment.name is not set: name the environment the review and sessions select"}
	}
	name, ok := raw.(string)
	if !ok || !environmentName.MatchString(name) {
		return []string{fmt.Sprintf("x-ellipsis.environment.name %v must use letters, digits, - and _, and start with a letter or digit, so it is safe as a file name", raw)}
	}
	e.name = name
	return nil
}

func (e *environmentConfig) readRepository(block map[string]any) []string {
	raw, held := block["repository"]
	if !held {
		return []string{"x-ellipsis.environment.repository is not set: name the one repository the environment checks out"}
	}
	repo, ok := raw.(map[string]any)
	if !ok {
		return []string{"x-ellipsis.environment.repository must be a mapping with a name"}
	}
	var problems []string
	for _, key := range sortedKeys(repo) {
		if key != "name" && key != "owner" && key != "ref" {
			problems = append(problems, fmt.Sprintf("x-ellipsis.environment.repository.%s is not an Ellipsis repository key (use name, owner, ref)", key))
		}
	}
	name, _ := repo["name"].(string)
	switch {
	case name == "":
		problems = append(problems, "x-ellipsis.environment.repository.name is not set")
	case strings.Contains(name, "/"):
		problems = append(problems, fmt.Sprintf("x-ellipsis.environment.repository.name %q has a slash: give the repository name without the owner, and set repository.owner", name))
	case name == "." || name == ".." || !repositoryName.MatchString(name):
		problems = append(problems, fmt.Sprintf("x-ellipsis.environment.repository.name %q must be a plain repository name: letters, digits, - _ and ., since it becomes the folder %s<name>", name, sandboxRoot))
	default:
		e.repoName = name
	}
	for _, key := range []string{"owner", "ref"} {
		if v, ok := repo[key]; ok {
			if s, isString := v.(string); !isString || s == "" {
				problems = append(problems, fmt.Sprintf("x-ellipsis.environment.repository.%s must be a non-empty string", key))
			}
		}
	}
	e.repository = repo
	return problems
}

func (e *environmentConfig) readDescription(block map[string]any) []string {
	raw, held := block["description"]
	if !held {
		return nil
	}
	if s, ok := raw.(string); ok && s != "" {
		e.description = s
		return nil
	}
	return []string{"x-ellipsis.environment.description must be a non-empty string"}
}

func (e *environmentConfig) readVariables(block map[string]any) []string {
	raw, held := block["variables"]
	if !held {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		return []string{"x-ellipsis.environment.variables must be a list of {name, value} mappings"}
	}
	var problems []string
	for i, item := range list {
		path := fmt.Sprintf("x-ellipsis.environment.variables[%d]", i)
		v, ok := item.(map[string]any)
		if !ok {
			problems = append(problems, path+" must be a mapping with a name")
			continue
		}
		for _, key := range sortedKeys(v) {
			if key != "name" && key != "value" {
				problems = append(problems, fmt.Sprintf("%s.%s is not an Ellipsis variable key (use name, value)", path, key))
			}
		}
		name, _ := v["name"].(string)
		if !variableName.MatchString(name) {
			problems = append(problems, fmt.Sprintf("%s.name %q must use letters, digits and underscores, and not start with a digit", path, name))
			continue
		}
		if reservedVariable(name) {
			problems = append(problems, fmt.Sprintf("%s.name %s is rejected by Ellipsis (names starting with %s, and %s)", path, name,
				strings.Join(reservedVariablePrefixes, ", "), strings.Join(reservedVariables, ", ")))
		}
		if value, set := v["value"]; set {
			if _, isString := value.(string); !isString {
				problems = append(problems, path+".value must be text")
			} else if preview.CredentialKey(name) {
				problems = append(problems, fmt.Sprintf("%s sets a value for %s, a credential name: omit value so Ellipsis reads the stored secret of that name, and keep the secret out of git", path, name))
			}
		}
	}
	e.variables = list
	return problems
}

func reservedVariable(name string) bool {
	if slices.Contains(reservedVariables, name) {
		return true
	}
	for _, prefix := range reservedVariablePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func (e *environmentConfig) readCompute(block map[string]any) []string {
	raw, held := block["compute"]
	if !held {
		return nil
	}
	compute, ok := raw.(map[string]any)
	if !ok {
		return []string{"x-ellipsis.environment.compute must be a mapping"}
	}
	var problems []string
	for _, key := range sortedKeys(compute) {
		path := "x-ellipsis.environment.compute." + key
		switch key {
		case "cpu":
			if _, isInt := compute[key].(int); !isInt {
				problems = append(problems, path+" must be a whole number of vCPUs")
			}
		case "memory", "timeout":
			if s, isString := compute[key].(string); !isString || s == "" {
				problems = append(problems, path+" must be text such as 8GB or 30m")
			}
		default:
			problems = append(problems, fmt.Sprintf("%s is not an Ellipsis compute key (use cpu, memory, timeout)", path))
		}
	}
	e.compute = compute
	return problems
}

func (e *environmentConfig) readHooks(block map[string]any) []string {
	raw, held := block["hooks"]
	if !held {
		return nil
	}
	hooks, ok := raw.(map[string]any)
	if !ok {
		return []string{"x-ellipsis.environment.hooks must be a mapping of hook names to scripts"}
	}
	var problems []string
	for _, key := range sortedKeys(hooks) {
		path := "x-ellipsis.environment.hooks." + key
		if !slices.Contains(hookNames, key) {
			problems = append(problems, fmt.Sprintf("%s is not an Ellipsis hook (use %s)", path, strings.Join(hookNames, ", ")))
			continue
		}
		script, inputs, found, hookProblems := readHook(hooks[key], path, key == "build_base", e.repoName)
		problems = append(problems, hookProblems...)
		switch key {
		case "build_base":
			e.hooks.buildBase, e.hooks.inputs, e.hooks.hasInputs = script, inputs, found
		case "after_checkout":
			e.hooks.afterCheckout = script
		case "before_start":
			e.hooks.beforeStart = script
		case "validate":
			e.hooks.validate = script
		}
	}
	return problems
}

// readHook reads one hook, a script or a `{run, inputs}` mapping. found
// reports whether `inputs` was set, since an empty list means something
// ("the install reads no repository files").
func readHook(raw any, path string, takesInputs bool, repo string) (script string, inputs []string, found bool, problems []string) {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return "", nil, false, []string{path + " must not be empty"}
		}
		return v, nil, false, nil
	case map[string]any:
		for _, key := range sortedKeys(v) {
			if key != "run" && (key != "inputs" || !takesInputs) {
				allowed := "run"
				if takesInputs {
					allowed = "run, inputs"
				}
				problems = append(problems, fmt.Sprintf("%s.%s is not an Ellipsis hook key (use %s)", path, key, allowed))
			}
		}
		if run, ok := v["run"]; ok {
			if s, isString := run.(string); isString && s != "" {
				script = s
			} else {
				problems = append(problems, path+".run must be a non-empty script")
			}
		}
		if list, ok := v["inputs"]; ok && takesInputs {
			found = true
			inputs, problems = readInputs(list, path+".inputs", repo, problems)
		}
		return script, inputs, found, problems
	}
	return "", nil, false, []string{path + " must be a script or a mapping with run"}
}

// readInputs checks the files `build_base` caches on. Ellipsis lists
// "exact files, starting with the repository name", and "globs, missing
// files, and symlinks fail the build".
func readInputs(raw any, path, repo string, problems []string) ([]string, []string) {
	list, ok := raw.([]any)
	if !ok {
		return nil, append(problems, path+" must be a list of files")
	}
	inputs := []string{}
	for i, item := range list {
		at := fmt.Sprintf("%s[%d]", path, i)
		file, ok := item.(string)
		switch {
		case !ok || file == "":
			problems = append(problems, at+" must be a file path")
		case strings.ContainsAny(file, "*?["):
			problems = append(problems, fmt.Sprintf("%s %q is a glob; Ellipsis fails the build on globs, so list each exact file", at, file))
		case slices.Contains(strings.Split(file, "/"), ".."):
			problems = append(problems, fmt.Sprintf("%s %q climbs out of the repository with ..", at, file))
		case repo != "" && !strings.HasPrefix(file, repo+"/"):
			problems = append(problems, fmt.Sprintf("%s %q must start with %q, the repository name", at, file, repo+"/"))
		case strings.HasSuffix(file, "/"):
			problems = append(problems, fmt.Sprintf("%s %q names a folder; list exact files", at, file))
		default:
			inputs = append(inputs, file)
		}
	}
	return inputs, problems
}

func (e *environmentConfig) readIntegrations(block map[string]any) []string {
	raw, held := block["mcp_servers"]
	if !held {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		return []string{"x-ellipsis.environment.mcp_servers must be a list of slack or linear; custom servers come from mcp specs"}
	}
	var problems []string
	for i, item := range list {
		name, _ := item.(string)
		if !slices.Contains(integrations, name) {
			problems = append(problems, fmt.Sprintf("x-ellipsis.environment.mcp_servers[%d] must be slack or linear, the connected integrations; custom servers come from mcp specs", i))
			continue
		}
		if !slices.Contains(e.integration, name) {
			e.integration = append(e.integration, name)
		}
	}
	return problems
}
