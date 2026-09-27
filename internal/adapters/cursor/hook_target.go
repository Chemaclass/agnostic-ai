package cursor

import "github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"

// HookTargetEvent is the event of the managed hook that names the target
// for every other hook. Cursor runs hook commands through PowerShell on
// Windows and `$SHELL` elsewhere, so no command prefix works on both. A
// sessionStart hook's `env` output instead reaches "all subsequent hook
// executions within that session" (cursor.com/docs/hooks), including
// the Claude Code hooks Cursor loads. The sessionStart hooks themselves,
// and any hook that fires before it returns, do not see the variable.
const HookTargetEvent = "sessionStart"

// HookTargetCommand prints that `env` output. Single quotes keep the JSON
// literal in POSIX shells, fish, and PowerShell alike.
const HookTargetCommand = `echo '{"env":{"` + emit.HookTargetEnv + `":"` + target + `"}}'`
