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
// worktrees.json keys Cursor documents: `setup-worktree-unix` for macOS
// and Linux, `setup-worktree-windows` for Windows. Each takes a list of
// commands. The keys stay out of environment.json, which is the cloud
// agent's file.
var worktreeSetupKeys = []struct{ field, key string }{
	{"setup", "setup-worktree-unix"},
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
			if cmds := setupCommands(m[k.field]); len(cmds) > 0 {
				doc[k.key] = cmds
			}
		}
		for _, k := range worktreeNativeKeys {
			if v, ok := m[k]; ok && v != nil {
				doc[k] = v
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
