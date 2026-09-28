package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// cursorWorktreesFile is where Cursor reads the commands it runs in a new
// worktree (cursor.com/docs/configuration/worktrees).
var cursorWorktreesFile = filepath.Join(".cursor", "worktrees.json")

// cursorWorktreeSpecName names the environment spec a worktrees.json
// imports into.
const cursorWorktreeSpecName = "worktree"

// importCursorWorktrees reads a hand-written .cursor/worktrees.json into
// an environment spec's `setup` and `setup-windows`, the reverse of the
// cursor emit (#1339). Cursor's `setup-worktree` runs on every OS, so it
// fills whichever OS key the file leaves unset. A file sync wrote, or a
// spec already at the destination, is left alone.
func importCursorWorktrees(root string, src config.Sources) (int, error) {
	if src.Environments == "" {
		return 0, nil
	}
	for _, p := range readStateFile(root).Outputs {
		if filepath.Clean(filepath.FromSlash(p)) == cursorWorktreesFile {
			return 0, nil
		}
	}
	path := filepath.Join(root, cursorWorktreesFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	// A string is a script path relative to .cursor/worktrees.json. It
	// stays a Cursor key under x-cursor; only command lists are portable.
	native := map[string]any{}
	commands := map[string][]string{}
	for _, key := range []string{"setup-worktree", "setup-worktree-unix", "setup-worktree-windows"} {
		switch v := doc[key].(type) {
		case string:
			if v != "" {
				native[key] = v
			}
		case []any:
			for _, c := range v {
				if s, ok := c.(string); ok && s != "" {
					commands[key] = append(commands[key], s)
				}
			}
		}
	}
	setup, setupWindows := commands["setup-worktree"], commands["setup-worktree-windows"]
	// A unix list is the portable `setup` only when Windows has its own
	// setup and no all-OS key is set; otherwise `setup` would start
	// running it on Windows too.
	if unix := commands["setup-worktree-unix"]; unix != nil {
		_, allNative := native["setup-worktree"]
		_, windowsNative := native["setup-worktree-windows"]
		if setup == nil && !allNative && (setupWindows != nil || windowsNative) {
			setup = unix
		} else {
			native["setup-worktree-unix"] = unix
		}
	}
	if setup == nil && setupWindows == nil && len(native) == 0 {
		return 0, nil
	}

	dstDir := filepath.Join(root, src.Environments)
	out := filepath.Join(dstDir, cursorWorktreeSpecName+".yaml")
	if fileExists(out) {
		summaryf("  ! skipped %s: %s already exists; move its setup commands there by hand\n",
			filepath.ToSlash(cursorWorktreesFile), filepath.ToSlash(src.Environments)+"/"+cursorWorktreeSpecName+".yaml")
		return 0, nil
	}
	spec := yaml.Node{Kind: yaml.MappingNode}
	add := func(key string, value any) {
		var v yaml.Node
		_ = v.Encode(value)
		spec.Content = append(spec.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, &v)
	}
	add("name", cursorWorktreeSpecName)
	if setup != nil {
		add("setup", specCommands(setup))
	}
	if setupWindows != nil {
		add("setup-windows", specCommands(setupWindows))
	}
	if len(native) > 0 {
		add("x-cursor", native)
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

// specCommands writes one command as a string and more as a list.
func specCommands(cmds []string) any {
	if len(cmds) == 1 {
		return cmds[0]
	}
	return cmds
}
