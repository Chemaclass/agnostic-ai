package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// claudeLaunchFileName is the file under the Claude directory where
// Claude Code Desktop reads its preview servers.
const claudeLaunchFileName = "launch.json"

// claudeLaunchSpecName names the environment spec launch.json imports into.
const claudeLaunchSpecName = "dev"

// importClaudeLaunch reads a hand-written .claude/launch.json into an
// environment spec's dev-commands, the reverse of the claude emit
// (#1340). A file with a configuration that has no command to start,
// such as one that only opens a `url`, is left whole with a note. So is a
// file sync wrote, or one whose destination spec already exists.
func importClaudeLaunch(root string, src config.Sources, layout claudeLayout) (int, error) {
	if src.Environments == "" {
		return 0, nil
	}
	claudeLaunchFile := filepath.Join(layout.dir, claudeLaunchFileName)
	for _, p := range readStateFile(root).Outputs {
		if filepath.Clean(filepath.FromSlash(p)) == claudeLaunchFile {
			return 0, nil
		}
	}
	path := filepath.Join(root, claudeLaunchFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	stripped, _ := adapters.StripJSONC(data)
	var raw map[string]any
	if err := json.Unmarshal(stripped, &raw); err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}

	// Sync rebuilds launch.json from the spec alone, so anything the spec
	// cannot express would be dropped. Import all or nothing.
	leave := func(reason string) (int, error) {
		summaryf("  ! left %s as written: %s, which a dev-commands spec cannot hold, so sync would drop it\n",
			filepath.ToSlash(claudeLaunchFile), reason)
		return 0, nil
	}
	for _, k := range slices.Sorted(maps.Keys(raw)) {
		switch v := raw[k]; k {
		case "version":
			if v != launchFileVersion {
				return leave(fmt.Sprintf("its version is %v, not %s", v, launchFileVersion))
			}
		case "autoVerify":
			if _, ok := v.(bool); !ok {
				return leave("its autoVerify is not true or false")
			}
		case "configurations":
		default:
			return leave(fmt.Sprintf("it sets %q", k))
		}
	}
	configurations, ok := raw["configurations"].([]any)
	if !ok {
		return leave("its configurations are not a list")
	}

	var commands []any
	for _, item := range configurations {
		c, _ := item.(map[string]any)
		name, _ := c["name"].(string)
		if problem := launchConfigurationProblem(c); problem != "" {
			return leave(fmt.Sprintf("configuration %q %s", name, problem))
		}
		argv, _ := launchArgv(c)
		cmd := yaml.Node{Kind: yaml.MappingNode}
		addYAMLField(&cmd, "name", name)
		addYAMLField(&cmd, "command", devCommandValue(argv))
		if cwd := portableLaunchCwd(c["cwd"]); cwd != "" {
			addYAMLField(&cmd, "cwd", cwd)
		}
		if port, ok := c["port"].(float64); ok {
			addYAMLField(&cmd, "port", int(port))
		}
		if auto, ok := c["autoPort"].(bool); ok {
			addYAMLField(&cmd, "auto-port", auto)
		}
		if env, ok := c["env"].(map[string]any); ok && len(env) > 0 {
			addYAMLField(&cmd, "env", env)
		}
		if url, _ := c["url"].(string); url != "" {
			addYAMLField(&cmd, "url", url)
		}
		commands = append(commands, &cmd)
	}
	if len(commands) == 0 {
		return 0, nil
	}

	dstDir := filepath.Join(root, src.Environments)
	out := filepath.Join(dstDir, claudeLaunchSpecName+".yaml")
	if fileExists(out) {
		summaryf("  ! skipped %s: %s already exists; move its dev-commands there by hand\n",
			filepath.ToSlash(claudeLaunchFile), filepath.ToSlash(src.Environments)+"/"+claudeLaunchSpecName+".yaml")
		return 0, nil
	}
	spec := yaml.Node{Kind: yaml.MappingNode}
	addYAMLField(&spec, "name", claudeLaunchSpecName)
	if scopeImportedEnvironment(root, "claude") {
		addYAMLField(&spec, "targets", importedTargetsNode("claude"))
	}
	addYAMLField(&spec, "dev-commands", commands)
	if autoVerify, ok := raw["autoVerify"].(bool); ok {
		addYAMLField(&spec, "x-claude", map[string]any{"autoVerify": autoVerify})
	}
	body, err := yaml.Marshal(&spec)
	if err != nil {
		return 0, fmt.Errorf("marshal %s: %w", out, err)
	}
	if err := importMkdirAll(dstDir, 0o755); err != nil {
		return 0, fmt.Errorf("mkdir %s: %w", dstDir, err)
	}
	if err := importWriteFile(out, body, 0o644); err != nil {
		return 0, fmt.Errorf("write %s: %w", out, err)
	}
	return 1, nil
}

// launchWorkspaceFolder is what Claude Code documents as the project root
// in a launch.json `cwd`.
const launchWorkspaceFolder = "${workspaceFolder}"

// portableLaunchCwd reads a launch.json `cwd` as the spec's project-relative
// form: a leading `${workspaceFolder}/` is dropped, and the project root
// itself, which is the default, becomes no cwd.
func portableLaunchCwd(v any) string {
	cwd, _ := v.(string)
	rest, ok := strings.CutPrefix(cwd, launchWorkspaceFolder)
	if !ok {
		return cwd
	}
	if rest == "" {
		return ""
	}
	if rel, ok := strings.CutPrefix(rest, "/"); ok {
		return rel
	}
	return cwd
}

// launchFileVersion is the only launch.json version sync writes.
const launchFileVersion = "0.0.1"

// launchConfigurationProblem says why a launch.json configuration cannot
// become a dev command exactly, or returns "" when it can.
func launchConfigurationProblem(c map[string]any) string {
	if c == nil {
		return "is not an object"
	}
	for _, k := range slices.Sorted(maps.Keys(c)) {
		var ok bool
		switch v := c[k]; k {
		case "name", "runtimeExecutable", "program", "cwd", "url":
			_, ok = v.(string)
		case "runtimeArgs", "args":
			_, ok = jsonStrings(v)
			ok = ok && v != nil
		case "port":
			n, isNum := v.(float64)
			ok = isNum && n == float64(int(n))
		case "autoPort":
			_, ok = v.(bool)
		case "env":
			env, isMap := v.(map[string]any)
			ok = isMap
			for _, val := range env {
				switch val.(type) {
				case string, float64, bool:
				default:
					ok = false
				}
			}
		default:
			return fmt.Sprintf("sets %q", k)
		}
		if !ok {
			return fmt.Sprintf("has a %q value of the wrong type", k)
		}
	}
	if _, hasArgs := c["args"]; hasArgs && c["program"] == nil {
		return `sets "args" without "program"`
	}
	if name, _ := c["name"].(string); name == "" {
		return "has no name"
	}
	if argv, _ := launchArgv(c); len(argv) == 0 {
		return "has no command to start"
	}
	return ""
}

// launchArgv reads the command a launch.json configuration starts:
// runtimeExecutable (`node` when only program is set), its runtimeArgs,
// then program and its args. It reports false when an argument is not a
// string, since the command could not be kept whole.
func launchArgv(c map[string]any) ([]string, bool) {
	exe, _ := c["runtimeExecutable"].(string)
	program, _ := c["program"].(string)
	if exe == "" && program != "" {
		exe = "node"
	}
	if exe == "" {
		return nil, true
	}
	argv := []string{exe}
	runtimeArgs, ok := jsonStrings(c["runtimeArgs"])
	if !ok {
		return nil, false
	}
	argv = append(argv, runtimeArgs...)
	if program != "" {
		args, ok := jsonStrings(c["args"])
		if !ok {
			return nil, false
		}
		argv = append(append(argv, program), args...)
	}
	return argv, true
}

// devCommandValue writes argv as the spec's command in the shortest form
// that starts the same argv: the command of `sh -c <command>`, or the
// words joined by spaces, when the spec reads either back unchanged;
// otherwise the list itself.
func devCommandValue(argv []string) any {
	if len(argv) == 3 && argv[0] == "sh" && argv[1] == "-c" &&
		slices.Equal(adapters.CommandArgv(argv[2]), argv) {
		return argv[2]
	}
	if joined := strings.Join(argv, " "); slices.Equal(adapters.CommandArgv(joined), argv) {
		return joined
	}
	return argv
}

func jsonStrings(v any) ([]string, bool) {
	if v == nil {
		return nil, true
	}
	list, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		s, ok := item.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

// importedTargetsNode is the `targets: [<tool>]` an imported environment
// spec carries when another tool's environment file is imported too.
// Environment specs merge by top-level key across targets, so without it
// one tool's dev commands or setup would reach the others and the sync
// after the import would not reproduce the files it read. Remove the line
// to share the spec.
func importedTargetsNode(tool string) *yaml.Node {
	return &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Value: tool}}}
}

// environmentSourceFiles are the hand-written environment files each tool
// keeps, which import reads into environment specs.
var environmentSourceFiles = []struct{ tool, path string }{
	{"claude", filepath.Join(".claude", claudeLaunchFileName)},
	{"codex", filepath.Join(".codex", "environments", "environment.toml")},
	{"cursor", filepath.Join(".cursor", "environment.json")},
	{"cursor", filepath.Join(".cursor", "worktrees.json")},
}

// scopeImportedEnvironment reports whether a spec imported from tool's
// environment file must carry `targets: [<tool>]`: another tool keeps a
// hand-written environment file, not one sync wrote, whose spec would
// otherwise merge with this one.
func scopeImportedEnvironment(root, tool string) bool {
	written := map[string]bool{}
	for _, p := range readStateFile(root).Outputs {
		written[filepath.Clean(filepath.FromSlash(p))] = true
	}
	for _, f := range environmentSourceFiles {
		if f.tool != tool && !written[f.path] && fileExists(filepath.Join(root, f.path)) {
			return true
		}
	}
	return false
}

func addYAMLField(n *yaml.Node, key string, value any) {
	var v yaml.Node
	if node, ok := value.(*yaml.Node); ok {
		v = *node
	} else if nodes, ok := value.([]any); ok {
		v = yaml.Node{Kind: yaml.SequenceNode}
		for _, item := range nodes {
			if node, ok := item.(*yaml.Node); ok {
				v.Content = append(v.Content, node)
			}
		}
	} else {
		_ = v.Encode(value)
	}
	n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, &v)
}
