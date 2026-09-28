package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// claudeLaunchFile is where Claude Code Desktop reads its preview servers.
var claudeLaunchFile = filepath.Join(".claude", "launch.json")

// claudeLaunchSpecName names the environment spec launch.json imports into.
const claudeLaunchSpecName = "dev"

// importClaudeLaunch reads a hand-written .claude/launch.json into an
// environment spec's dev-commands, the reverse of the claude emit
// (#1340). A configuration with no command to start, such as one that
// only opens a `url`, stays behind with a note. A file sync wrote, or a
// spec already at the destination, is left alone.
func importClaudeLaunch(root string, src config.Sources) (int, error) {
	if src.Environments == "" {
		return 0, nil
	}
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
	var doc struct {
		AutoVerify     *bool            `json:"autoVerify"`
		Configurations []map[string]any `json:"configurations"`
	}
	if err := json.Unmarshal(stripped, &doc); err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}

	var commands []any
	for _, c := range doc.Configurations {
		name, _ := c["name"].(string)
		argv := launchArgv(c)
		if name == "" || len(argv) == 0 {
			summaryf("  ! skipped %s configuration %q: it has no command to start\n", filepath.ToSlash(claudeLaunchFile), name)
			continue
		}
		cmd := yaml.Node{Kind: yaml.MappingNode}
		addYAMLField(&cmd, "name", name)
		addYAMLField(&cmd, "command", devCommandValue(argv))
		if cwd, _ := c["cwd"].(string); cwd != "" {
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
	addYAMLField(&spec, "dev-commands", commands)
	if doc.AutoVerify != nil {
		addYAMLField(&spec, "x-claude", map[string]any{"autoVerify": *doc.AutoVerify})
	}
	raw, err := yaml.Marshal(&spec)
	if err != nil {
		return 0, fmt.Errorf("marshal %s: %w", out, err)
	}
	if err := importMkdirAll(dstDir, 0o755); err != nil {
		return 0, fmt.Errorf("mkdir %s: %w", dstDir, err)
	}
	if err := importWriteFile(out, raw, 0o644); err != nil {
		return 0, fmt.Errorf("write %s: %w", out, err)
	}
	return 1, nil
}

// launchArgv reads the command a launch.json configuration starts:
// runtimeExecutable and runtimeArgs, or `node` with program and args.
func launchArgv(c map[string]any) []string {
	if exe, _ := c["runtimeExecutable"].(string); exe != "" {
		return append([]string{exe}, jsonStrings(c["runtimeArgs"])...)
	}
	if program, _ := c["program"].(string); program != "" {
		return append([]string{"node", program}, jsonStrings(c["args"])...)
	}
	return nil
}

// devCommandValue writes argv as the spec's command: `sh -c <command>`
// as that command, plain words as one string, and anything a string
// would split differently as a list.
func devCommandValue(argv []string) any {
	if len(argv) == 3 && argv[0] == "sh" && argv[1] == "-c" {
		return argv[2]
	}
	for _, a := range argv {
		if a == "" || strings.ContainsAny(a, " \t\n'\"\\=") || strings.ContainsAny(a, "|&;<>()$`*?[]{}~#") {
			return argv
		}
	}
	return strings.Join(argv, " ")
}

func jsonStrings(v any) []string {
	list, _ := v.([]any)
	var out []string
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
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
