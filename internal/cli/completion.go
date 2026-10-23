package cli

import (
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/spf13/cobra"
)

func registerTargetCompletion(cmd *cobra.Command) {
	completeTargets := func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		cfg, err := config.Load(".")
		if err != nil {
			return allTargetNames(), cobra.ShellCompDirectiveNoFileComp
		}
		return cfg.Targets, cobra.ShellCompDirectiveNoFileComp
	}
	_ = cmd.RegisterFlagCompletionFunc("target", completeTargets)
	_ = cmd.RegisterFlagCompletionFunc("only", completeTargets)
	_ = cmd.RegisterFlagCompletionFunc("except", completeTargets)
}
