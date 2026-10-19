package ellipsis

import (
	"bytes"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const environmentDir = ".ellipsis/environments"

// plannedFile is a file the adapter has fully rendered and not yet
// written, so a problem in one file stops both.
type plannedFile struct {
	path, content string
}

// portableEnvironment is what the environment specs say: the last spec
// that sets a field wins, as in every environment field.
type portableEnvironment struct {
	install, setup string
}

// environmentSpecKeys are the environment spec keys the Ellipsis emit
// reads or notes itself, plus the spec's identity fields.
var environmentSpecKeys = map[string]bool{
	"name": true, "description": true, "scope": true, "install": true, "setup": true,
	"setup-windows": true, "cleanup": true, "dev-commands": true, "terminals": true,
}

// environmentFieldsWithoutEffect are the environment fields Ellipsis has
// no stage or key for, with the reason each note gives.
var environmentFieldsWithoutEffect = []struct{ field, reason string }{
	{"setup-windows", "Ellipsis runs scripts in its own sandbox under bash, with no Windows setup"},
	{"cleanup", "Ellipsis has no cleanup stage; a session ends with its sandbox"},
	{"dev-commands", "Ellipsis has no dev server list; start a service in the background from setup and wait until it is ready"},
	{"terminals", "Ellipsis has no terminal list"},
}

// planEnvironment renders the environment file when x-ellipsis.environment
// selects one. It returns the file, the environment name the review file
// must reference ("" when no file is written or deployed).
func planEnvironment(b spec.Bundle, cfg *config.Config) (*plannedFile, string, error) {
	mode := ""
	if cfg != nil {
		mode = cfg.OnUnsupported
	}
	env, problems, selected := readEnvironment(b.Settings)
	if !selected {
		env = &environmentConfig{}
		if len(b.Environments) > 0 || len(b.MCPs) > 0 {
			problems = []string{"environment and MCP specs reach this target, but no environment is selected: add a settings spec whose x-ellipsis.environment names the environment and its one repository"}
		} else {
			return nil, "", nil
		}
	}
	var file *plannedFile
	if len(problems) == 0 {
		file, problems = buildEnvironment(env, b, cfg, mode)
	}
	if len(problems) > 0 {
		// A selection that is set but broken must not take the deployed
		// file down with it. Without a selection the sweep removes it.
		var kept *plannedFile
		linked := ""
		if selected {
			var err error
			kept, linked, err = deployedEnvironment(cfg, env.name)
			if err != nil {
				return nil, "", err
			}
		}
		return kept, linked, reportProblems(problems, mode, "environment file")
	}
	return file, env.name, nil
}

func deployedEnvironment(cfg *config.Config, name string) (*plannedFile, string, error) {
	expected := emit.OutputEnvironmentFile(cfg, target, path.Join(environmentDir, name+".yaml"))
	if name != "" {
		if file, linked := readDeployedEnvironment(expected); file != nil {
			return file, linked, nil
		}
	}
	var files []*plannedFile
	var names []string
	err := filepath.WalkDir(".ellipsis", func(p string, entry fs.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		if entry.IsDir() {
			return nil
		}
		if ext := filepath.Ext(p); ext != ".yaml" && ext != ".yml" {
			return nil
		}
		if file, linked := readDeployedEnvironment(p); file != nil {
			files = append(files, file)
			names = append(names, linked)
			if len(files) > 1 {
				return fs.SkipAll
			}
		}
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	if len(files) > 1 {
		return nil, "", fmt.Errorf("ellipsis: broken environment selection cannot choose between %d deployed environment files; repair x-ellipsis.environment before syncing", len(files))
	}
	if len(files) == 1 {
		return files[0], names[0], nil
	}
	return nil, "", nil
}

func readDeployedEnvironment(p string) (*plannedFile, string) {
	file := keepDeployed(p)
	if file == nil {
		return nil, ""
	}
	var doc struct {
		Ellipsis struct {
			Kind string `yaml:"kind"`
			Name string `yaml:"name"`
		} `yaml:"ellipsis"`
	}
	if err := yaml.Unmarshal([]byte(file.content), &doc); err != nil || doc.Ellipsis.Kind != environmentKind || !environmentName.MatchString(doc.Ellipsis.Name) {
		return nil, ""
	}
	return file, doc.Ellipsis.Name
}

// buildEnvironment renders the file, or returns the problems that stop it.
func buildEnvironment(env *environmentConfig, b spec.Bundle, cfg *config.Config, mode string) (*plannedFile, []string) {
	noteEnvironmentNoOps(b.Environments)
	portable := mergeEnvironments(b.Environments)
	problems := hookConflicts(env, portable)
	filePath := emit.OutputEnvironmentFile(cfg, target, path.Join(environmentDir, env.name+".yaml"))
	if problem := environmentPathProblem(filePath); problem != "" {
		problems = append(problems, problem)
	}
	servers, omitted := buildServers(b.MCPs, env.repoName)
	if len(problems) > 0 {
		return nil, problems
	}
	if err := reportOmitted(omitted, mode); err != nil {
		return nil, []string{err.Error()}
	}
	doc, err := renderEnvironment(env, portable, servers)
	if err != nil {
		return nil, []string{err.Error()}
	}
	return &plannedFile{path: filePath, content: emit.WithHeader(doc, emit.FormatYAML)}, nil
}

// hookConflicts names each stage with two sources. A portable script and a
// native one for the same stage would leave the file with a guess.
func hookConflicts(env *environmentConfig, portable portableEnvironment) []string {
	var problems []string
	if portable.install != "" && env.hooks.buildBase != "" {
		problems = append(problems, "the environment spec's install and x-ellipsis.environment.hooks.build_base.run both set the build_base script: keep one")
	}
	if portable.setup != "" && env.hooks.afterCheckout != "" {
		problems = append(problems, "the environment spec's setup and x-ellipsis.environment.hooks.after_checkout both set the after_checkout script: keep one")
	}
	if env.hooks.hasInputs && portable.install == "" && env.hooks.buildBase == "" {
		problems = append(problems, "x-ellipsis.environment.hooks.build_base has inputs but no script: set hooks.build_base.run, or give an environment spec an install")
	}
	return problems
}

// environmentPathProblem checks the output path. Ellipsis reads
// environment files from `.ellipsis/` on the default branch only.
func environmentPathProblem(p string) string {
	clean := path.Clean(filepath.ToSlash(p))
	ext := path.Ext(clean)
	if filepath.IsAbs(p) || !strings.HasPrefix(clean, ".ellipsis/") || (ext != ".yaml" && ext != ".yml") {
		return fmt.Sprintf("outputs.ellipsis.environment-file %q must be a .yaml file under .ellipsis/, the only place Ellipsis reads environment files from", p)
	}
	return ""
}

// mergeEnvironments reads install and setup from the environment specs.
func mergeEnvironments(envs []spec.Entry) portableEnvironment {
	var doc portableEnvironment
	for _, e := range envs {
		m := emit.ResolveMeta(e.Meta, target)
		if v, ok := m["install"]; ok {
			doc.install = scriptText(v)
		}
		if v, ok := m["setup"]; ok {
			doc.setup = scriptText(v)
		}
	}
	return doc
}

// scriptText reads a script written as one command or a list, as the
// script that runs each command on its own line.
func scriptText(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimRight(s, "\n")
	}
	return strings.Join(emit.StringSlice(v), "\n")
}

// inCheckout runs script from the repository checkout. Ellipsis starts a
// hook in /sandbox, the parent of every checkout.
func inCheckout(repo, script string) string {
	if script == "" {
		return ""
	}
	return "cd " + sandboxRoot + repo + "\n" + script + "\n"
}

// noteEnvironmentNoOps notes each environment field Ellipsis has no place for.
func noteEnvironmentNoOps(envs []spec.Entry) {
	for _, f := range environmentFieldsWithoutEffect {
		emit.NoteFieldNoOp(target, spec.KindEnvironment, f.field, emit.EnvironmentsWithField(target, envs, f.field), f.reason)
	}
	counts := map[string]int{}
	for _, e := range envs {
		for k := range emit.ResolveMeta(e.Meta, target) {
			if !environmentSpecKeys[k] {
				counts[k]++
			}
		}
	}
	for _, k := range slices.Sorted(maps.Keys(counts)) {
		emit.NoteFieldNoOp(target, spec.KindEnvironment, k, counts[k], "Ellipsis has no file key for it")
	}
}

// renderEnvironment writes the file with the keys in the order the schema
// documents them. Nested values keep their own content and sort by key,
// so a sync is stable.
func renderEnvironment(env *environmentConfig, portable portableEnvironment, servers []*yaml.Node) (string, error) {
	top := mapping()
	head := mapping()
	add(head, "kind", textNode(environmentKind))
	add(head, "name", textNode(env.name))
	if env.description != "" {
		add(head, "description", textNode(env.description))
	}
	if env.metadata != nil {
		add(head, "metadata", orderedMap(env.metadata, []string{"labels", "annotations"}))
	}
	add(top, "ellipsis", head)
	add(top, "repositories", &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{orderedMap(env.repository, []string{"name", "owner", "ref"})}})
	if len(env.variables) > 0 {
		vars := &yaml.Node{Kind: yaml.SequenceNode}
		for _, v := range env.variables {
			m, _ := v.(map[string]any)
			vars.Content = append(vars.Content, orderedMap(m, []string{"name", "value"}))
		}
		add(top, "variables", vars)
	}
	if hooks := hooksNode(env, portable); len(hooks.Content) > 0 {
		add(top, "hooks", hooks)
	}
	if env.compute != nil {
		add(top, "compute", orderedMap(env.compute, []string{"cpu", "memory", "timeout"}))
	}
	if list := serverList(env, servers); len(list.Content) > 0 {
		add(top, "mcp_servers", list)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(top); err != nil {
		return "", fmt.Errorf("ellipsis: render environment file: %w", err)
	}
	if err := enc.Close(); err != nil {
		return "", fmt.Errorf("ellipsis: render environment file: %w", err)
	}
	return buf.String(), nil
}

// hooksNode lists the stages in the order they run. A portable script
// runs from the checkout; a native one is written as given.
func hooksNode(env *environmentConfig, portable portableEnvironment) *yaml.Node {
	hooks := mapping()
	buildBase := env.hooks.buildBase
	if portable.install != "" {
		buildBase = inCheckout(env.repoName, portable.install)
	}
	if buildBase != "" {
		stage := mapping()
		if env.hooks.hasInputs {
			inputs := &yaml.Node{Kind: yaml.SequenceNode}
			for _, file := range env.hooks.inputs {
				inputs.Content = append(inputs.Content, textNode(file))
			}
			add(stage, "inputs", inputs)
		}
		add(stage, "run", textNode(buildBase))
		add(hooks, "build_base", stage)
	}
	afterCheckout := env.hooks.afterCheckout
	if portable.setup != "" {
		afterCheckout = inCheckout(env.repoName, portable.setup)
	}
	if afterCheckout != "" {
		add(hooks, "after_checkout", textNode(afterCheckout))
	}
	if env.hooks.beforeStart != "" {
		add(hooks, "before_start", textNode(env.hooks.beforeStart))
	}
	if env.hooks.validate != "" {
		add(hooks, "validate", textNode(env.hooks.validate))
	}
	return hooks
}

// serverList puts the connected integrations first, then each server
// sorted by name.
func serverList(env *environmentConfig, servers []*yaml.Node) *yaml.Node {
	list := &yaml.Node{Kind: yaml.SequenceNode}
	for _, name := range env.integration {
		list.Content = append(list.Content, textNode(name))
	}
	list.Content = append(list.Content, servers...)
	return list
}
