package cli

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	globalStart = "<!-- agnostic-ai:global:start -->"
	globalEnd   = "<!-- agnostic-ai:global:end -->"
)

type globalSyncOptions struct {
	targets, only, except                                                    []string
	dryRun, check, backup, plan, watch, watchPoll, jsonOut, allTargets, diff bool
	format, gitignore                                                        string
	jobs                                                                     int
}

type globalState struct {
	Version     int              `json:"version"`
	Files       []string         `json:"files"`
	ClaudeHooks map[string][]any `json:"claudeHooks,omitempty"`
	CursorHooks map[string][]any `json:"cursorHooks,omitempty"`
}

type globalWrite struct {
	path string
	data []byte
	mode fs.FileMode
}

func runGlobalSync(cmd *cobra.Command, o globalSyncOptions) error {
	if o.watch || o.watchPoll || o.plan || o.jsonOut || o.allTargets || o.diff || o.gitignore != "" || o.jobs != 0 {
		return errs.Coded(errs.CodeFlagConflict, "--global does not support --watch, --watch-poll, --plan, --json, --all, --diff, --gitignore, or --jobs")
	}
	if len(o.only) > 0 && len(o.except) > 0 {
		return errs.Coded(errs.CodeFlagConflict, "--only and --except are mutually exclusive")
	}
	if err := validateCheckFormat(o.format); err != nil {
		return err
	}
	if o.format != checkFormatHuman && !o.check {
		return errs.Coded(errs.CodeFlagConflict, "--format requires --check with --global")
	}
	targets := o.targets
	if len(targets) == 0 {
		targets = []string{"claude", "cursor"}
	}
	var err error
	targets, err = filterTargets(targets, o.only, o.except)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if target != "claude" && target != "cursor" {
			return fmt.Errorf("--global: unsupported target %q (supported: claude, cursor)", target)
		}
	}

	home, err := globalUserHome()
	if err != nil {
		return err
	}
	sourceHome := os.Getenv("AGNOSTIC_AI_HOME")
	if sourceHome == "" {
		sourceHome = filepath.Join(home, ".agnostic-ai")
	}
	source := filepath.Join(sourceHome, "global")
	cfg := &config.Config{Sources: config.Sources{Rules: "rules", Hooks: "hooks", Skills: "skills"}}
	bundle, err := spec.LoadBundle(source, cfg)
	if err != nil {
		return err
	}
	for _, rule := range bundle.Rules {
		if rule.Scope != "" || hasGlobalRuleCondition(rule.Meta) {
			return fmt.Errorf("global rule %q is scoped or conditional; global rules must apply unconditionally", rule.Name)
		}
	}
	instructions, err := os.ReadFile(filepath.Join(source, "AGNOSTIC_AI.md"))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", filepath.Join(source, "AGNOSTIC_AI.md"), err)
	}

	statePath := filepath.Join(sourceHome, "state", "global.json")
	old, err := loadGlobalState(statePath)
	if err != nil {
		return err
	}
	writes, next, err := buildGlobalWrites(home, source, targets, instructions, bundle, old)
	if err != nil {
		return err
	}
	stateData, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", statePath, err)
	}
	writes = append(writes, globalWrite{statePath, append(stateData, '\n'), 0o644})
	if err := preflightGlobalWrites(writes, old, next); err != nil {
		return err
	}
	if o.check {
		for _, w := range writes {
			data, err := os.ReadFile(w.path)
			if err != nil || !reflect.DeepEqual(data, w.data) {
				return fmt.Errorf("global configuration drift: %s", w.path)
			}
		}
		return nil
	}
	if o.dryRun {
		for _, w := range writes {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "dry-run: write %s\n", w.path); err != nil {
				return fmt.Errorf("write dry-run output: %w", err)
			}
		}
		return nil
	}
	removals := removedGlobalFiles(old.Files, next.Files)
	if err := applyGlobalChanges(writes, removals, o.backup); err != nil {
		return err
	}
	if _, err = fmt.Fprintf(cmd.OutOrStdout(), "Synced global configuration to %d target(s).\n", len(targets)); err != nil {
		return fmt.Errorf("write sync summary: %w", err)
	}
	return nil
}

func hasGlobalRuleCondition(meta map[string]any) bool {
	for _, key := range []string{"scope", "globs", "paths", "target", "targets", "target-exclude", "targets-exclude"} {
		if _, ok := meta[key]; ok {
			return true
		}
	}
	return false
}

func loadGlobalState(path string) (globalState, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return globalState{Version: 1}, nil
	}
	if err != nil {
		return globalState{}, fmt.Errorf("read %s: %w", path, err)
	}
	var state globalState
	if err := json.Unmarshal(data, &state); err != nil || state.Version != 1 {
		return globalState{}, fmt.Errorf("parse %s: corrupt global ownership state", path)
	}
	return state, nil
}

func buildGlobalWrites(home, source string, targets []string, intro []byte, b spec.Bundle, old globalState) ([]globalWrite, globalState, error) {
	next := globalState{Version: 1, Files: append([]string(nil), old.Files...), ClaudeHooks: cloneHookState(old.ClaudeHooks), CursorHooks: cloneHookState(old.CursorHooks)}
	var writes []globalWrite
	body := strings.TrimSpace(string(intro))
	for _, rule := range b.Rules {
		if body != "" {
			body += "\n\n"
		}
		body += "## " + rule.Name + "\n\n" + strings.TrimSpace(rule.Body)
	}
	managed := globalStart + "\n" + body + "\n" + globalEnd
	for _, target := range targets {
		base := filepath.Join(home, "."+target)
		next.Files = removePathPrefix(next.Files, base+string(filepath.Separator))
		instructionsPath := filepath.Join(base, map[string]string{"claude": "CLAUDE.md", "cursor": "AGENTS.md"}[target])
		merged, err := mergeGlobalBlock(instructionsPath, managed)
		if err != nil {
			return nil, next, err
		}
		writes = append(writes, globalWrite{instructionsPath, []byte(merged), 0o644})
		next.Files = append(next.Files, instructionsPath)
		for _, skill := range b.Skills {
			dst := filepath.Join(base, "skills", skill.Name)
			if err := filepath.WalkDir(filepath.Dir(skill.Path), func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}
				rel, err := filepath.Rel(filepath.Dir(skill.Path), path)
				if err != nil {
					return err
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return fmt.Errorf("read %s: %w", path, err)
				}
				out := filepath.Join(dst, rel)
				info, err := d.Info()
				if err != nil {
					return fmt.Errorf("stat %s: %w", path, err)
				}
				writes = append(writes, globalWrite{out, data, info.Mode().Perm()})
				next.Files = append(next.Files, out)
				return nil
			}); err != nil {
				return nil, next, err
			}
		}
		if target == "claude" {
			next.ClaudeHooks = map[string][]any{}
			path := filepath.Join(base, "settings.json")
			doc, err := mergeGlobalHooks(path, "claude", b.Hooks, old.ClaudeHooks, next.ClaudeHooks)
			if err != nil {
				return nil, next, err
			}
			if doc != nil {
				writes = append(writes, globalWrite{path, doc, 0o644})
				next.Files = append(next.Files, path)
			}
		} else {
			next.CursorHooks = map[string][]any{}
			bridge, command, script, mode := cursorGlobalBridge(base, body)
			writes = append(writes, globalWrite{bridge, []byte(script), mode})
			next.Files = append(next.Files, bridge)
			path := filepath.Join(base, "hooks.json")
			hooks := append([]spec.Entry{}, b.Hooks...)
			hooks = append(hooks, spec.Entry{Meta: map[string]any{"event": "sessionStart", "command": command}})
			doc, err := mergeGlobalHooks(path, "cursor", hooks, old.CursorHooks, next.CursorHooks)
			if err != nil {
				return nil, next, err
			}
			writes = append(writes, globalWrite{path, doc, 0o644})
			next.Files = append(next.Files, path)
		}
	}
	sort.Strings(next.Files)
	return writes, next, nil
}

func mergeGlobalBlock(path, managed string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	body := string(data)
	start, end := strings.Index(body, globalStart), strings.Index(body, globalEnd)
	if (start >= 0) != (end >= 0) || (start >= 0 && end < start) {
		return "", fmt.Errorf("%s: corrupt agnostic-ai managed block", path)
	}
	if start >= 0 {
		end += len(globalEnd)
		body = strings.TrimSpace(body[:start] + body[end:])
	}
	body = strings.TrimSpace(body)
	if strings.TrimSpace(managed) == globalStart+"\n\n"+globalEnd {
		return body, nil
	}
	if body != "" {
		body += "\n\n"
	}
	return body + managed + "\n", nil
}

func mergeGlobalHooks(path, target string, entries []spec.Entry, previous map[string][]any, next map[string][]any) ([]byte, error) {
	doc := map[string]any{}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	hooks, _ := doc["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	for event, oldEntries := range previous {
		current, _ := hooks[event].([]any)
		for _, oldEntry := range oldEntries {
			var found bool
			current, found = removeEqual(current, oldEntry)
			if !found {
				return nil, fmt.Errorf("%s: managed hook ownership is corrupt for %s", path, event)
			}
		}
		if len(current) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = current
		}
	}
	for _, entry := range entries {
		event, _ := entry.Meta["event"].(string)
		if event == "" {
			continue
		}
		for _, command := range globalHookCommands(entry.Meta["command"]) {
			var item any
			if target == "claude" {
				commandHook := map[string]any{"type": "command", "command": command}
				for _, key := range []string{"timeout", "statusMessage", "async", "asyncRewake", "shell", "if", "once"} {
					if value, ok := entry.Meta[key]; ok {
						commandHook[key] = value
					}
				}
				matcher, _ := entry.Meta["matcher"].(string)
				item = map[string]any{"matcher": matcher, "hooks": []any{commandHook}}
			} else {
				cursorHook := map[string]any{"command": command}
				for _, key := range []string{"matcher", "timeout", "loop_limit", "failClosed"} {
					if value, ok := entry.Meta[key]; ok {
						cursorHook[key] = value
					}
				}
				item = cursorHook
			}
			current, _ := hooks[event].([]any)
			hooks[event] = append(current, item)
			next[event] = append(next[event], item)
		}
	}
	doc["hooks"] = hooks
	if target == "cursor" {
		doc["version"] = float64(1)
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal %s: %w", path, err)
	}
	return append(out, '\n'), nil
}

func removeEqual(items []any, want any) ([]any, bool) {
	for i, item := range items {
		if reflect.DeepEqual(item, want) {
			return append(items[:i], items[i+1:]...), true
		}
	}
	return items, false
}

func globalHookCommands(raw any) []string {
	switch value := raw.(type) {
	case string:
		if value != "" {
			return []string{value}
		}
	case []any:
		var out []string
		for _, item := range value {
			if command, ok := item.(string); ok && command != "" {
				out = append(out, command)
			}
		}
		return out
	case []string:
		return value
	}
	return nil
}
func jsonString(s string) string { raw, _ := json.Marshal(s); return string(raw) }
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

func globalUserHome() (string, error) {
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return home, nil
}

func cursorGlobalBridge(base, body string) (path, command, script string, mode fs.FileMode) {
	payload := `{"additional_context":` + jsonString(body) + `}`
	if runtime.GOOS == "windows" {
		path = filepath.Join(base, "hooks", "agnostic-ai-global-context.ps1")
		command = `powershell -NoProfile -ExecutionPolicy Bypass -File "` + strings.ReplaceAll(path, `"`, `\"`) + `"`
		script = "$payload = '" + strings.ReplaceAll(payload, "'", "''") + "'\r\n[Console]::Out.WriteLine($payload)\r\n"
		return path, command, script, 0o644
	}
	path = filepath.Join(base, "hooks", "agnostic-ai-global-context.sh")
	return path, path, "#!/bin/sh\nprintf '%s\\n' " + shellQuote(payload) + "\n", 0o755
}

func preflightGlobalWrites(writes []globalWrite, old, next globalState) error {
	owned := map[string]bool{}
	for _, p := range old.Files {
		owned[p] = true
	}
	for _, w := range writes {
		if strings.Contains(w.path, string(filepath.Separator)+"skills"+string(filepath.Separator)) && !owned[w.path] {
			if filepath.Base(w.path) == "SKILL.md" {
				if _, err := os.Stat(filepath.Dir(w.path)); err == nil {
					return fmt.Errorf("%s: unmanaged global skill collision", filepath.Dir(w.path))
				}
			}
			if _, err := os.Stat(w.path); err == nil {
				return fmt.Errorf("%s: unmanaged global skill collision", w.path)
			}
		}
		if info, err := os.Lstat(w.path); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s: refusing to replace symlink", w.path)
		}
	}
	return nil
}

func cloneHookState(in map[string][]any) map[string][]any {
	out := map[string][]any{}
	for k, v := range in {
		out[k] = append([]any(nil), v...)
	}
	return out
}
func removePathPrefix(paths []string, prefix string) []string {
	out := paths[:0]
	for _, p := range paths {
		if !strings.HasPrefix(p, prefix) {
			out = append(out, p)
		}
	}
	return out
}

func removedGlobalFiles(old, next []string) []string {
	keep := map[string]bool{}
	for _, p := range next {
		keep[p] = true
	}
	var out []string
	for _, p := range old {
		if !keep[p] {
			out = append(out, p)
		}
	}
	return out
}

func applyGlobalChanges(writes []globalWrite, removals []string, backup bool) error {
	type prior struct {
		path   string
		data   []byte
		mode   fs.FileMode
		absent bool
	}
	var done []prior
	rollback := func() {
		for i := len(done) - 1; i >= 0; i-- {
			p := done[i]
			if p.absent {
				_ = os.Remove(p.path)
			} else {
				_ = os.WriteFile(p.path, p.data, p.mode)
			}
		}
	}
	for _, w := range writes {
		old, err := os.ReadFile(w.path)
		absent := os.IsNotExist(err)
		if err != nil && !absent {
			rollback()
			return fmt.Errorf("read %s: %w", w.path, err)
		}
		if !absent && reflect.DeepEqual(old, w.data) {
			continue
		}
		mode := fs.FileMode(0o644)
		if info, statErr := os.Stat(w.path); statErr == nil {
			mode = info.Mode()
		}
		done = append(done, prior{w.path, old, mode, absent})
		if err := os.MkdirAll(filepath.Dir(w.path), 0o755); err != nil {
			rollback()
			return fmt.Errorf("create directory for %s: %w", w.path, err)
		}
		if backup && !absent {
			if err := os.WriteFile(w.path+".bak", old, mode); err != nil {
				rollback()
				return fmt.Errorf("backup %s: %w", w.path, err)
			}
		}
		tmp, err := os.CreateTemp(filepath.Dir(w.path), ".agnostic-ai-global-*")
		if err != nil {
			rollback()
			return fmt.Errorf("create temporary file for %s: %w", w.path, err)
		}
		tmpName := tmp.Name()
		writeErr := func() error {
			defer func() { _ = os.Remove(tmpName) }()
			if _, err := tmp.Write(w.data); err != nil {
				return err
			}
			if err := tmp.Chmod(w.mode); err != nil {
				return err
			}
			if err := tmp.Close(); err != nil {
				return err
			}
			return os.Rename(tmpName, w.path)
		}()
		if writeErr != nil {
			_ = tmp.Close()
			rollback()
			return fmt.Errorf("write %s: %w", w.path, writeErr)
		}
	}
	for _, path := range removals {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			rollback()
			return fmt.Errorf("read managed %s: %w", path, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			rollback()
			return fmt.Errorf("stat managed %s: %w", path, err)
		}
		done = append(done, prior{path: path, data: data, mode: info.Mode()})
		if err := os.Remove(path); err != nil {
			rollback()
			return fmt.Errorf("remove managed %s: %w", path, err)
		}
	}
	return nil
}
