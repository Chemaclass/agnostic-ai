package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

// memoryContextLimit keeps the session-start context under every hook
// target's cap: Codex keeps 2,500 tokens by default and Claude Code
// 10,000 characters.
const memoryContextLimit = 8000

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
			root := guardProjectRoot()
			if root == "" {
				return nil
			}
			index, err := os.ReadFile(filepath.Join(root, adapters.ProjectMemoryIndexPath))
			if err != nil || strings.TrimSpace(string(index)) == "" {
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
	keep := text[:memoryContextLimit-len(cut)]
	if i := strings.LastIndex(keep, "\n"); i >= 0 {
		keep = keep[:i+1]
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
