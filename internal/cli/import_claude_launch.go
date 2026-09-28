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
	var doc struct {
		AutoVerify     *bool            `json:"autoVerify"`
		Configurations []map[string]any `json:"configurations"`
	}
	if err := json.Unmarshal(stripped, &doc); err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}

	// Sync rebuilds launch.json from the spec alone, so a configuration
	// the spec cannot express would be dropped. Import all or nothing.
	var commands []any
	for _, c := range doc.Configurations {
		name, _ := c["name"].(string)
		argv, whole := launchArgv(c)
		reason := ""
		switch {
		case !whole:
			reason = "an argument is not a string"
		case name == "" || len(argv) == 0:
			reason = "it has no command to start"
		}
		if reason != "" {
			summaryf("  ! left %s as written: configuration %q cannot be a dev command (%s), and sync would drop it\n",
				filepath.ToSlash(claudeLaunchFile), name, reason)
			return 0, nil
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
