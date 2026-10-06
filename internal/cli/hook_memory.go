package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// memoryContextLimit keeps the session-start context under every hook
// target's cap: Codex keeps 2,500 tokens by default and Claude Code
// 10,000 characters. At 6,000 bytes, text over the Codex cap needs
// fewer than 2.4 bytes per token, which dense non-Latin text can reach.
const memoryContextLimit = 6000

func newHookMemoryCmd() *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "memory",
		Short: "Print the shared memory index from inside a session-start hook",
		Long: "Prints the project's shared memory index (.agnostic-ai/memory/MEMORY.md) in the reply " +
			"format the target adds to the model's context. Prints nothing when the project has no index, " +
			"so a missing store never disturbs a session.",
		Example: `  # Hook command that does nothing when the binary is missing
  command -v agnostic-ai >/dev/null 2>&1 || exit 0; agnostic-ai hook memory`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if target == "" {
				target = os.Getenv(adapters.HookTargetEnv)
			}
			root := memoryProjectRoot(target)
			if root == "" {
				return nil
			}
			index, ok := readMemoryIndex(root)
			if !ok {
				return nil
			}
			reply, err := memoryHookReply(target, memoryContext(string(index)))
			if err != nil {
				return nil
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), reply)
			return err
		},
	}
	cmd.Flags().StringVarP(&target, "target", "t", "", "Target that runs the hook (default $"+adapters.HookTargetEnv+")")
	return cmd
}

// memoryContext frames the index for the model and cuts it at a whole
// line to stay under memoryContextLimit.
func memoryContext(index string) string {
	const head = "## Shared memory\n\nIndex of `" + adapters.ProjectMemoryIndexPath + "`. Open a fact's file in that folder when its line is relevant.\n\n"
	const cut = "\n(The index continues in " + adapters.ProjectMemoryIndexPath + ".)\n"
	text := head + strings.TrimRight(index, "\n") + "\n"
	if len(text) <= memoryContextLimit {
		return text
	}
	keep := text[:memoryContextLimit-len(cut)-1]
	if i := strings.LastIndex(keep, "\n"); i >= len(head) {
		keep = keep[:i+1]
	} else {
		for len(keep) > len(head) && !utf8.RuneStart(text[len(keep)]) {
			keep = keep[:len(keep)-1]
		}
		keep += "\n"
	}
	return keep + cut
}

// memoryHookReply wraps text in the reply each target reads as context.
func memoryHookReply(target, text string) (string, error) {
	var key string
	switch target {
	case "cursor":
		key = "additional_context"
	case "copilot":
		key = "additionalContext"
	default:
		return text, nil
	}
	data, err := json.Marshal(map[string]string{key: text})
	if err != nil {
		return "", fmt.Errorf("encode %s reply: %w", target, err)
	}
	return string(data) + "\n", nil
}

// hookProjectDirEnv names the variable a target sets to the workspace
// when its hooks run elsewhere, such as Cursor's user hooks in ~/.cursor/.
var hookProjectDirEnv = map[string]string{
	"cursor":  "CURSOR_PROJECT_DIR",
	"qoder":   "QODER_PROJECT_DIR",
	"factory": "FACTORY_PROJECT_DIR",
}

// memoryProjectRoot finds the project the shared-memory skill uses: the
// nearest ancestor with a project config, never the global source root,
// or else the Git checkout, since a global install reaches projects with
// no config of their own.
func memoryProjectRoot(target string) string {
	wd := os.Getenv(hookProjectDirEnv[target])
	if wd == "" {
		var err error
		if wd, err = os.Getwd(); err != nil {
			return ""
		}
	}
	global := ""
	if source, err := globalSourceRoot(); err == nil {
		global = canonicalDir(source)
	}
	for dir := canonicalDir(wd); ; {
		if dir != global {
			if _, _, err := config.ResolveConfigPath(dir); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	top, err := gitOutput(wd, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(top)
}

// canonicalDir resolves symlinks in dir, or returns it absolute when it
// cannot.
func canonicalDir(dir string) string {
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		return real
	}
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

// memoryIndexMaxBytes bounds the read; the context keeps far less.
const memoryIndexMaxBytes = 1 << 20

// readMemoryIndex reads the project's memory index when it is a regular
// file at its own path under root. A hook reads it into the model's
// context unasked, so any symlink on the way, such as one a cloned
// checkout ships to .env or a credentials file, is skipped.
func readMemoryIndex(root string) (string, bool) {
	root = canonicalDir(root)
	real, err := filepath.EvalSymlinks(filepath.Join(root, adapters.ProjectMemoryIndexPath))
	if err != nil {
		return "", false
	}
	if real != filepath.Join(root, adapters.ProjectMemoryIndexPath) {
		return "", false
	}
	f, err := os.Open(real)
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	data, err := io.ReadAll(io.LimitReader(f, memoryIndexMaxBytes))
	if err != nil || strings.TrimSpace(string(data)) == "" {
		return "", false
	}
	return string(data), true
}
