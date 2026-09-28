package cursor

import (
	"fmt"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// defaultWorktreesFile is where Cursor reads the commands it runs in a new
// worktree (cursor.com/docs/configuration/worktrees).
const defaultWorktreesFile = ".cursor/worktrees.json"

// worktreeSetupKeys maps an environment spec's setup fields to the
// worktrees.json keys Cursor documents: `setup-worktree` for every OS and
// `setup-worktree-windows`, which Cursor runs instead on Windows. Each
// takes a list of commands. The keys stay out of environment.json, which
// is the cloud agent's file.
var worktreeSetupKeys = []struct{ field, key string }{
	{"setup", "setup-worktree"},
	{"setup-windows", "setup-worktree-windows"},
}

// worktreeNativeKeys are the worktrees.json keys an `x-cursor` block may
// set as written. A string value there is a script path relative to
// .cursor/worktrees.json, which a portable `setup` command cannot say.
var worktreeNativeKeys = []string{"setup-worktree", "setup-worktree-unix", "setup-worktree-windows"}

// emitWorktrees writes .cursor/worktrees.json from the environment specs'
// setup fields. The last spec that sets a field wins, as in
// environment.json.
func emitWorktrees(sess *emit.Session, envs []spec.Entry, dryRun bool) error {
	doc := map[string]any{}
	for _, e := range envs {
		m := emit.ResolveMeta(e.Meta, target)
		for _, k := range worktreeSetupKeys {
			if v, ok := m[k.field]; ok {
				setWorktreeKey(doc, k.key, setupCommands(v))
			}
		}
		for _, k := range worktreeNativeKeys {
			if v, ok := m[k]; ok {
				setWorktreeKey(doc, k, v)
			}
		}
	}
	if len(doc) == 0 {
		return nil
	}
	raw, err := emit.MarshalJSONIndent(doc)
	if err != nil {
		return fmt.Errorf("cursor worktrees: %w", err)
	}
	return sess.WriteFile(defaultWorktreesFile, string(raw)+"\n", dryRun)
}

// setWorktreeKey sets key to v, or clears it when a later spec sets the
// field empty, so the last spec wins as in environment.json.
func setWorktreeKey(doc map[string]any, key string, v any) {
	switch t := v.(type) {
	case nil:
		delete(doc, key)
	case string:
		if t == "" {
			delete(doc, key)
			return
		}
		doc[key] = t
	case []string:
		if len(t) == 0 {
			delete(doc, key)
			return
		}
		doc[key] = t
	default:
		doc[key] = v
	}
}

// setupCommands reads a setup field written as one command or a list.
func setupCommands(v any) []string {
	if s, ok := v.(string); ok {
		if s == "" {
			return nil
		}
		return []string{s}
	}
	return emit.StringSlice(v)
}

func isWorktreeSetupField(k string) bool {
	for _, s := range worktreeSetupKeys {
		if s.field == k {
			return true
		}
	}
	return slices.Contains(worktreeNativeKeys, k)
}
