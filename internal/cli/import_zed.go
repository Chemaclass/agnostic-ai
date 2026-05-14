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

	"github.com/chemaclass/agnostic-ai/internal/config"
)

const (
	zedMainFile  = ".rules"
	zedTasksFile = ".zed/tasks.json"
	zedSettings  = ".zed/settings.json"
	zedMCPKey    = "context_servers"
)

// importFromZed reads an existing Zed project (`.rules`,
// `.zed/tasks.json`, `.zed/settings.json`) under root and writes specs
// into the configured source directories.
func importFromZed(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Hooks, src.MCPs); err != nil {
		return err
	}
	rules, err := sliceMainFileByH2(root, zedMainFile, filepath.Join(root, src.Rules))
	if err != nil {
		return err
	}
	hooks, err := importZedTasks(root, filepath.Join(root, src.Hooks))
	if err != nil {
		return err
	}
	mcps, err := importZedContextServers(root, filepath.Join(root, src.MCPs))
	if err != nil {
		return err
	}
	if err := mirrorMainFile(root, zedMainFile); err != nil {
		return err
	}
	summaryf("imported %d rules, %d hooks, %d mcps\n", rules, hooks, mcps)
	return nil
}

// importZedTasks reads `.zed/tasks.json` and writes one hook yaml per
// task entry. Each Zed task has `label`, `command`, and optional `args`;
// we synthesize an `event: OnDemand` since Zed has no native lifecycle
// hooks. The full shell command (command + args, joined) becomes the
// hook command so the round-trip is lossless.
func importZedTasks(root, dstDir string) (int, error) {
	src := filepath.Join(root, zedTasksFile)
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	var tasks []map[string]any
	if err := json.Unmarshal(data, &tasks); err != nil {
		return 0, fmt.Errorf("parse %s: %w", src, err)
	}
	count := 0
	for i, t := range tasks {
		label, _ := t["label"].(string)
		cmd, _ := t["command"].(string)
		if cmd == "" {
			continue
		}
		args := toStringSlice(t["args"])
		// Reverse the `sh -c "<command>"` shape the emitter writes by
		// extracting the inner command when present.
		fullCmd := cmd
		if cmd == "sh" && len(args) == 2 && args[0] == "-c" {
			fullCmd = args[1]
		} else if len(args) > 0 {
			fullCmd = strings.TrimSpace(cmd + " " + strings.Join(args, " "))
		}
		name, desc := splitZedTaskLabel(label, i)
		doc := map[string]any{
			"name":    name,
			"event":   "OnDemand",
			"command": fullCmd,
		}
		if desc != "" {
			doc["description"] = desc
		}
		raw, err := yaml.Marshal(doc)
		if err != nil {
			return count, fmt.Errorf("marshal hook %s: %w", name, err)
		}
		path := filepath.Join(dstDir, name+".yaml")
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			return count, fmt.Errorf("write %s: %w", path, err)
		}
		count++
	}
	return count, nil
}

// splitZedTaskLabel reverses the emitter's `<name> — <description>`
// label shape. Falls back to `task-<index>` when the label is empty.
func splitZedTaskLabel(label string, index int) (name, description string) {
	if label == "" {
		return fmt.Sprintf("task-%d", index+1), ""
	}
	if i := strings.Index(label, " — "); i >= 0 {
		return slugify(label[:i]), strings.TrimSpace(label[i+len(" — "):])
	}
	return slugify(label), ""
}

func toStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// importZedContextServers reads `.zed/settings.json` and writes one
// yaml per `context_servers` entry. Each server has a nested `command:
// {path, args, env}` block which we flatten to `command`/`args`/`env`.
func importZedContextServers(root, dstDir string) (int, error) {
	src := filepath.Join(root, zedSettings)
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return 0, fmt.Errorf("parse %s: %w", src, err)
	}
	servers, ok := doc[zedMCPKey].(map[string]any)
	if !ok || len(servers) == 0 {
		return 0, nil
	}
	flat := map[string]any{}
	for name, raw := range servers {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		out := map[string]any{}
		if cmd, ok := entry["command"].(map[string]any); ok {
			if p, ok := cmd["path"].(string); ok && p != "" {
				out["command"] = p
			}
			if args := toStringSlice(cmd["args"]); len(args) > 0 {
				out["args"] = args
			}
			if env, ok := cmd["env"].(map[string]any); ok && len(env) > 0 {
				out["env"] = env
			}
		}
		if _, hasCmd := out["command"]; !hasCmd {
			continue
		}
		flat[name] = out
	}
	if len(flat) == 0 {
		return 0, nil
	}
	return writeMCPYAMLs(flat, dstDir)
}
