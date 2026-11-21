package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// The values --scope takes: where the specs a command works on live.
const (
	specScopeUser    = "user"
	specScopeProject = "project"
	specScopeLocal   = "local"
)

const (
	scopeFlagName        = "scope"
	legacyGlobalFlagName = "global"
)

// scopeFlag holds what --scope and the deprecated --global were given.
// Each flag updates *user as it is parsed, so an Args check that runs
// before the command already sees the scope.
type scopeFlag struct {
	user   *bool
	scope  string
	legacy bool
}

func (f *scopeFlag) apply() { *f.user = f.legacy || f.scope == specScopeUser }

type scopeValue struct{ flag *scopeFlag }

func (v scopeValue) String() string { return v.flag.scope }
func (v scopeValue) Type() string   { return "string" }
func (v scopeValue) Set(s string) error {
	switch s {
	case specScopeUser, specScopeProject, specScopeLocal:
	default:
		return fmt.Errorf("must be %s, %s, or %s", specScopeUser, specScopeProject, specScopeLocal)
	}
	v.flag.scope = s
	v.flag.apply()
	return nil
}

type legacyGlobalValue struct{ flag *scopeFlag }

func (v legacyGlobalValue) String() string   { return strconv.FormatBool(v.flag.legacy) }
func (v legacyGlobalValue) Type() string     { return "bool" }
func (v legacyGlobalValue) IsBoolFlag() bool { return true }
func (v legacyGlobalValue) Set(s string) error {
	on, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	v.flag.legacy = on
	v.flag.apply()
	return nil
}

// addScopeFlag registers --scope and the deprecated --global, which
// means --scope user. It sets *user while the flags are parsed. userHelp
// says what the command does at user level.
func addScopeFlag(cmd *cobra.Command, user *bool, userHelp string) {
	state := &scopeFlag{user: user, scope: specScopeProject}
	flags := cmd.Flags()
	flags.Var(scopeValue{state}, scopeFlagName, "Where the specs live: project or user. user: "+userHelp)
	flags.Var(legacyGlobalValue{state}, legacyGlobalFlagName, "Same as --scope user.")
	flags.Lookup(legacyGlobalFlagName).NoOptDefVal = "true"
	// Not MarkDeprecated: cobra prints that notice on stdout, where it
	// would corrupt --json output.
	_ = flags.MarkHidden(legacyGlobalFlagName)
	_ = cmd.RegisterFlagCompletionFunc(scopeFlagName, func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		// No command runs at local scope yet, so it is not offered.
		return []string{specScopeUser, specScopeProject}, cobra.ShellCompDirectiveNoFileComp
	})

	before := cmd.PreRunE
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if err := checkScopeFlag(cmd.CommandPath(), state.scope, flags.Changed(scopeFlagName), state.legacy); err != nil {
			return err
		}
		if flags.Changed(legacyGlobalFlagName) && verbosity >= levelDefault {
			instead := specScopeProject
			if *state.user {
				instead = specScopeUser
			}
			cmd.PrintErrf("warning: --global is deprecated; use --scope %s\n", instead)
		}
		if before != nil {
			return before(cmd, args)
		}
		return nil
	}
}

// checkScopeFlag rejects a scope the command cannot run at. command
// names it in the error. explicit says --scope was passed; legacy says
// --global was.
func checkScopeFlag(command, scope string, explicit, legacy bool) error {
	if legacy && explicit && scope != specScopeUser {
		return errs.Coded(errs.CodeFlagConflict, "--global means --scope %s and cannot be combined with --scope %s", specScopeUser, scope)
	}
	if scope == specScopeLocal {
		return fmt.Errorf("%s does not support --scope %s; use %s or %s", command, specScopeLocal, specScopeProject, specScopeUser)
	}
	return nil
}

// userScopeSet reports whether cmd was asked to run at user scope, with
// either spelling. False for a command without the flag.
func userScopeSet(cmd *cobra.Command) bool {
	f := cmd.Flags().Lookup(scopeFlagName)
	if f == nil {
		return false
	}
	v, ok := f.Value.(scopeValue)
	return ok && *v.flag.user
}
