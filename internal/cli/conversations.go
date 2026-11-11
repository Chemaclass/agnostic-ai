package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

// briefReadLimit bounds what is read of a brief; Start says when the
// brief is longer than it takes.
const briefReadLimit = 1 << 20

// startConversation and runBrief reach the process table, the tools'
// stores and the terminal; tests replace them.
var (
	startConversation = func(ctx context.Context, req conversations.StartRequest) (conversations.StartReply, error) {
		starter := conversations.Starter{
			Stores:    conversations.DefaultStores(),
			Processes: conversations.SystemProcesses{},
			Launcher:  conversations.SystemLauncher(),
			LookPath:  exec.LookPath,
			Getenv:    os.Getenv,
			BriefsDir: conversations.DefaultBriefsDir(),
			Now:       time.Now,
		}
		return starter.Start(ctx, req)
	}
	runBrief = func(id string) error {
		runner := conversations.BriefRunner{
			BriefsDir: conversations.DefaultBriefsDir(),
			LookPath:  exec.LookPath,
			Exec:      conversations.ExecTool,
			Environ:   os.Environ,
		}
		return runner.Run(id)
	}
)

func newConversationsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "conversations", Short: "Start a conversation linked to the one this runs in"}
	cmd.AddCommand(newConversationsStartCmd(), newConversationsRunCmd())
	return cmd
}

func newConversationsStartCmd() *cobra.Command {
	var tool, briefFile string
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a new conversation in a new terminal tab, linked to this one",
		Long: `Start a new conversation in a new terminal tab, in this folder, with the brief as its first prompt.

Run it from inside a Claude Code or Codex session. The new conversation's first line names that session, so the Conversations page can show where it came from. Pass the brief on standard input or with --brief-file.`,
		Example: `  agnostic-ai conversations start <<'EOF'
  Write the release notes for the parser change.
  EOF
  agnostic-ai conversations start --tool codex --brief-file brief.md`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			brief, err := readBrief(cmd.InOrStdin(), briefFile)
			if err != nil {
				return err
			}
			folder, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("could not read the current folder: %w", err)
			}
			self, err := os.Executable()
			if err != nil {
				return fmt.Errorf("could not find this agnostic-ai binary: %w", err)
			}
			reply, err := startConversation(cmd.Context(), conversations.StartRequest{
				Tool: conversations.Tool(tool), Folder: folder, Brief: brief, Self: self, PID: os.Getpid(),
			})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), reply.Message)
			return err
		},
	}
	cmd.Flags().StringVar(&tool, "tool", "", "tool for the new conversation: claude or codex (default: the tool this runs in)")
	cmd.Flags().StringVar(&briefFile, "brief-file", "", "read the brief from this file instead of standard input")
	return cmd
}

// readBrief reads the brief from path, or from in when path is empty.
// A terminal on standard input means no brief was piped.
func readBrief(in io.Reader, path string) (string, error) {
	if path != "" {
		file, err := os.Open(path)
		if err != nil {
			return "", fmt.Errorf("could not read the brief: %w", err)
		}
		defer func() { _ = file.Close() }()
		in = file
	} else if stdinIsTerminal(in) {
		return "", errors.New("no brief: pass it on standard input or with --brief-file")
	}
	data, err := io.ReadAll(io.LimitReader(in, briefReadLimit))
	if err != nil {
		return "", fmt.Errorf("could not read the brief: %w", err)
	}
	return string(data), nil
}

func newConversationsRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "run <brief-id>",
		Short:  "Run a one-time brief in the tab that start opened",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runBrief(args[0])
		},
	}
}
