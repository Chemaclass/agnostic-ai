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
			scopes := []memoryIndex{
				// Personal first: it is short, and a long project index must
				// not push the user's own corrections out of the limit.
				personalMemoryIndex(root),
				{name: "Project memory", path: adapters.ProjectMemoryIndexPath},
			}
			var indexes []memoryIndex
			for _, scope := range scopes {
				if text, ok := readMemoryIndex(root, scope.path); ok {
					scope.text = text
					indexes = append(indexes, scope)
				}
			}
			if len(indexes) == 0 {
				return nil
			}
			reply, err := memoryHookReply(target, memoryContext(indexes, scopes))
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

// memoryIndex is one scope's index as the hook prints it. path is
// relative to the project root, or absolute for the repo store.
type memoryIndex struct{ name, path, text string }

// personalMemoryIndex returns the personal index of the project at root,
// in the checkout unless its config sets memory.personal: repo.
func personalMemoryIndex(root string) memoryIndex {
	index := memoryIndex{name: "Personal memory", path: adapters.PersonalMemoryIndexPath}
	cfg, err := config.Load(root)
	if err != nil || !cfg.RepoPersonalMemory() {
		return index
	}
	if dir, err := adapters.PersonalMemoryDir(cfg, root); err == nil {
		index.path = filepath.ToSlash(filepath.Join(dir, "MEMORY.md"))
	}
	return index
}

// memoryContext frames the indexes for the model and cuts them at a
// whole line to stay under memoryContextLimit. The cut note names every
// scope's index.
func memoryContext(indexes, scopes []memoryIndex) string {
	paths := make([]string, len(scopes))
	for i, index := range scopes {
		paths[i] = index.path
	}
	cut := "\n(More facts are in " + strings.Join(paths, " and ") + ".)\n"
	text := "## Shared memory\n\nOpen a fact's file, in the folder of its index, when its line is relevant.\n"
	head := 0
	for i, index := range indexes {
		text += "\n" + index.name + ", `" + index.path + "`:\n\n"
		if i == 0 {
			head = len(text)
		}
		text += strings.TrimRight(index.text, "\n") + "\n"
	}
	if len(text) <= memoryContextLimit {
		return text
	}
	keep := text[:memoryContextLimit-len(cut)-1]
	if i := strings.LastIndex(keep, "\n"); i >= head {
		keep = keep[:i+1]
	} else {
		for len(keep) > head && !utf8.RuneStart(text[len(keep)]) {
			keep = keep[:len(keep)-1]
		}
		keep += "\n"
	}
	return keep + cut
}

// memoryHookReply wraps text in the reply each target reads as context.
func memoryHookReply(target, text string) (string, error) {
	var reply any
	switch target {
	case "gemini":
		reply = map[string]map[string]string{"hookSpecificOutput": {"hookEventName": "SessionStart", "additionalContext": text}}
	case "cursor":
		reply = map[string]string{"additional_context": text}
	case "copilot":
		reply = map[string]string{"additionalContext": text}
	default:
		return text, nil
	}
	data, err := json.Marshal(reply)
	if err != nil {
		return "", fmt.Errorf("encode %s reply: %w", target, err)
	}
	return string(data) + "\n", nil
}

// hookProjectDirEnv names the variable a target sets to the workspace
// when its hooks run elsewhere, such as Cursor's user hooks in ~/.cursor/.
var hookProjectDirEnv = map[string]string{
	"gemini":  "GEMINI_PROJECT_DIR",
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
func readMemoryIndex(root, indexPath string) (string, bool) {
	want := filepath.Join(canonicalDir(root), indexPath)
	if filepath.IsAbs(indexPath) {
		want = filepath.Join(canonicalDir(filepath.Dir(indexPath)), filepath.Base(indexPath))
	}
	real, err := filepath.EvalSymlinks(want)
	if err != nil {
		return "", false
	}
	if real != want {
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
