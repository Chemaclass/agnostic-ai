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
	user *bool
	// local is nil for a command that cannot run at local scope.
	local  *bool
	scope  string
	legacy bool
}

func (f *scopeFlag) apply() {
	*f.user = f.legacy || f.scope == specScopeUser
	if f.local != nil {
		*f.local = !*f.user && f.scope == specScopeLocal
	}
}

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
	registerScopeFlag(cmd, &scopeFlag{user: user, scope: specScopeProject}, "Where the specs live: project or user. user: "+userHelp)
}

// addScopeFlagWithLocal is addScopeFlag for a command that also runs at
// local scope, the personal layer of a project. It sets *local for it.
func addScopeFlagWithLocal(cmd *cobra.Command, user, local *bool, help string) {
	registerScopeFlag(cmd, &scopeFlag{user: user, local: local, scope: specScopeProject}, "Where the specs live: project, local, or user. "+help)
}

func registerScopeFlag(cmd *cobra.Command, state *scopeFlag, help string) {
	flags := cmd.Flags()
	flags.Var(scopeValue{state}, scopeFlagName, help)
	flags.Var(legacyGlobalValue{state}, legacyGlobalFlagName, "Same as --scope user.")
	flags.Lookup(legacyGlobalFlagName).NoOptDefVal = "true"
	// Not MarkDeprecated: cobra prints that notice on stdout, where it
	// would corrupt --json output.
	_ = flags.MarkHidden(legacyGlobalFlagName)
	_ = cmd.RegisterFlagCompletionFunc(scopeFlagName, func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		// local is offered only where the command runs at it.
		if state.local != nil {
			return []string{specScopeProject, specScopeLocal, specScopeUser}, cobra.ShellCompDirectiveNoFileComp
		}
		return []string{specScopeUser, specScopeProject}, cobra.ShellCompDirectiveNoFileComp
	})

	checkScopeBeforeArgs(cmd, func(cmd *cobra.Command) error {
		return checkScopeFlag(cmd.CommandPath(), state.scope, flags.Changed(scopeFlagName), state.legacy, state.local != nil)
	})
	before := cmd.PreRunE
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
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
// --global was; local says the command runs at local scope.
func checkScopeFlag(command, scope string, explicit, legacy, local bool) error {
	if legacy && explicit && scope != specScopeUser {
		return errs.Coded(errs.CodeFlagConflict, "--global means --scope %s and cannot be combined with --scope %s", specScopeUser, scope)
	}
	if scope == specScopeLocal && !local {
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

// addProjectOnlyScopeFlag registers --scope on a command that works on
// a project's specs only, so it takes the flag every command takes.
// userInstead names what does the same job at user scope and is shown
// when the flag asks for it.
func addProjectOnlyScopeFlag(cmd *cobra.Command, userInstead string) {
	var user bool
	state := &scopeFlag{user: &user, scope: specScopeProject}
	flags := cmd.Flags()
	flags.Var(scopeValue{state}, scopeFlagName, "Where the specs live. This command reads a project: only project is accepted.")
	_ = cmd.RegisterFlagCompletionFunc(scopeFlagName, func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{specScopeProject}, cobra.ShellCompDirectiveNoFileComp
	})
	checkScopeBeforeArgs(cmd, func(cmd *cobra.Command) error {
		switch state.scope {
		case specScopeUser:
			return fmt.Errorf("%s works on a project and does not support --scope %s; %s", cmd.CommandPath(), specScopeUser, userInstead)
		case specScopeLocal:
			return fmt.Errorf("%s works on a whole project and does not support --scope %s; `agnostic-ai list --scope %s` shows your personal specs", cmd.CommandPath(), specScopeLocal, specScopeLocal)
		}
		return nil
	})
}

// checkScopeBeforeArgs runs check ahead of the command's own argument
// check, which cobra runs before any PreRun. A scope the command cannot
// take is then reported as that, not as a missing argument.
func checkScopeBeforeArgs(cmd *cobra.Command, check func(*cobra.Command) error) {
	args := cmd.Args
	cmd.Args = func(cmd *cobra.Command, positional []string) error {
		if err := check(cmd); err != nil {
			return err
		}
		if args != nil {
			return args(cmd, positional)
		}
		return nil
	}
}
