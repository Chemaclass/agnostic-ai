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
	all := worktreeCommands(doc["setup-worktree"])
	unix := worktreeCommands(doc["setup-worktree-unix"])
	windows := worktreeCommands(doc["setup-worktree-windows"])
	if unix == nil {
		unix = all
	}
	if windows == nil {
		windows = all
	}
	if unix == nil && windows == nil {
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
	if unix != nil {
		add("setup", unix)
	}
	if windows != nil {
		add("setup-windows", windows)
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

// worktreeCommands reads one worktrees.json value, a command list or a
// script path, as a spec value: one command as a string, more as a list.
func worktreeCommands(v any) any {
	switch t := v.(type) {
	case string:
		if t != "" {
			return t
		}
	case []any:
		var cmds []string
		for _, c := range t {
			if s, ok := c.(string); ok && s != "" {
				cmds = append(cmds, s)
			}
		}
		if len(cmds) == 1 {
			return cmds[0]
		}
		if len(cmds) > 1 {
			return cmds
		}
	}
	return nil
}
