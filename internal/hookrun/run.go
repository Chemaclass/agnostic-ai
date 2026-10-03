package hookrun

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"time"
)

// DefaultTimeout is how long target waits for a command hook on event
// that sets no timeout: 600 seconds on Codex, 60 on Gemini CLI
// (gemini-cli c6bccb7 hookRunner.ts DEFAULT_HOOK_TIMEOUT), and on Claude
// Code 600, lowered to 30 on UserPromptSubmit, PreModelSwitch, and
// PostModelSwitch and to 10 on MessageDisplay (code.claude.com/docs/en/hooks).
func DefaultTimeout(target, event string) time.Duration {
	if d, ok := otherDefaultTimeouts[target]; ok {
		return d
	}
	switch {
	case target == "gemini":
		return 60 * time.Second
	case target != "claude":
		return 600 * time.Second
	case event == "MessageDisplay":
		return 10 * time.Second
	case event == "UserPromptSubmit" || event == "PreModelSwitch" || event == "PostModelSwitch":
		return 30 * time.Second
	}
	return 600 * time.Second
}

// Handler is one command a target runs for a hook, as sync writes it.
type Handler struct {
	Command string
	// Args switches a Claude Code hook to exec form.
	Args []string
	// Shell is Claude Code's `shell`; "powershell" leaves bash.
	Shell string
	// CommandWindows is what Codex runs on Windows.
	CommandWindows string
	// Env is what the handler's own env adds, as on Gemini CLI.
	Env map[string]string
	// Timeout is the timeout sync writes on the handler; zero when it
	// writes none and the target's default applies.
	Timeout time.Duration
	// If is Claude Code's `if` permission rule.
	If string
	// FailClosed is Goose's `on_failure: block`: a failed run blocks.
	FailClosed bool
}

// Argv returns the process target starts for h on goos. Claude Code runs
// shell-form hooks with bash (Git Bash on Windows). Codex runs command
// through a POSIX shell, and commandWindows through PowerShell.
func Argv(target, goos string, h Handler) []string {
	if argv, ok := otherArgv(target, goos, h); ok {
		return argv
	}
	switch {
	case target == "gemini" && goos == "windows":
		// shell-utils.ts falls back to Windows PowerShell, and
		// hookRunner.ts appends the exit code check.
		return []string{"powershell.exe", "-NoProfile", "-NonInteractive", "-Command", h.Command + "; if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }"}
	case target == "gemini":
		return []string{"bash", "-c", h.Command}
	case target == "codex" && goos == "windows":
		return []string{"powershell.exe", "-NoProfile", "-Command", h.CommandWindows}
	case target == "codex":
		return []string{"sh", "-c", h.Command}
	case len(h.Args) > 0:
		return append([]string{h.Command}, h.Args...)
	case h.Shell == "powershell" && goos == "windows":
		return []string{"powershell.exe", "-NoProfile", "-Command", h.Command}
	case h.Shell == "powershell":
		return []string{"pwsh", "-NoProfile", "-Command", h.Command}
	}
	return []string{"bash", "-c", h.Command}
}

// Result is what one hook process did. StartErr is set when it never
// started, such as a missing executable.
type Result struct {
	Exit     int
	Stdout   string
	Stderr   string
	Elapsed  time.Duration
	TimedOut bool
	StartErr error
	// FromPath names the commands a Crush hook ran from PATH that Crush
	// runs as its own Go programs.
	FromPath []string
}

// Run starts argv in dir with env and stdin, and kills it, with any
// child it started, once timeout passes.
func Run(argv []string, dir string, env []string, stdin []byte, timeout time.Duration) Result {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = bytes.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	killTree(cmd)
	// A grandchild that outlives the kill still holds the output pipes.
	cmd.WaitDelay = time.Second
	start := time.Now()
	err := cmd.Run()
	r := Result{Stdout: stdout.String(), Stderr: stderr.String(), Elapsed: time.Since(start)}
	reapTree(cmd)
	switch {
	case cmd.ProcessState == nil:
		r.StartErr = err
	// A hook that exited on its own before the deadline is no timeout,
	// even when a child it left in the background held the pipes past it.
	case !cmd.ProcessState.Exited() && errors.Is(ctx.Err(), context.DeadlineExceeded):
		r.TimedOut = true
	default:
		r.Exit = cmd.ProcessState.ExitCode()
	}
	return r
}

// Decision is what the target does with a hook's result.
type Decision string

// Allow lets the event proceed, Block stops it, Error is a failure the
// target reports and moves past, and Timeout is a hook it canceled.
const (
	Allow   Decision = "allow"
	Block   Decision = "block"
	Error   Decision = "error"
	Timeout Decision = "timeout"
)

// nonBlockingEvents show exit 2's stderr to the user but cannot stop
// anything, in Claude Code and Codex alike.
var nonBlockingEvents = []string{"SessionStart", "SessionEnd", "Notification", "PreCompact", "PostCompact"}

// Decide reads a result the way Claude Code and Codex do: exit 2 blocks,
// another non-zero exit is an error, and exit 0 may carry a JSON reply
// that denies, blocks, or stops.
func Decide(target, event string, r Result) Decision {
	if target == "gemini" {
		return decideGemini(event, r)
	}
	switch {
	case r.TimedOut:
		return Timeout
	case r.StartErr != nil:
		return Error
	case r.Exit == 2 && slices.Contains(nonBlockingEvents, event):
		return Error
	case r.Exit == 2:
		return Block
	case r.Exit != 0:
		return Error
	}
	reply, ok := readReply(r)
	if ok && (reply.HookSpecificOutput.PermissionDecision == "deny" || reply.Decision == "block" ||
		(reply.Continue != nil && !*reply.Continue)) {
		return Block
	}
	return Allow
}

// contextEvents add a hook's plain stdout on exit 0 to the session.
var contextEvents = []string{"SessionStart", "UserPromptSubmit"}

// AddsContext reports whether the target adds the hook's output to what
// the model sees: plain stdout on a context event, or a JSON reply's
// additionalContext.
func AddsContext(target, event string, r Result) bool {
	switch target {
	case "gemini":
		return geminiAddsContext(r)
	case "goose", "augment":
		return false
	case "crush":
		return crushAddsContext(r)
	case "cursor":
		var reply struct {
			AdditionalContext string `json:"additional_context"`
		}
		return !r.TimedOut && r.Exit == 0 && json.Unmarshal([]byte(strings.TrimSpace(r.Stdout)), &reply) == nil && reply.AdditionalContext != ""
	case "openhands":
		var reply struct {
			AdditionalContext string `json:"additionalContext"`
		}
		return !r.TimedOut && json.Unmarshal([]byte(strings.TrimSpace(r.Stdout)), &reply) == nil && reply.AdditionalContext != ""
	case "trae":
		reply, ok := readReply(r)
		return ok && r.Exit == 0 && reply.HookSpecificOutput.AdditionalContext != ""
	}
	if r.TimedOut || r.StartErr != nil || r.Exit != 0 {
		return false
	}
	if reply, ok := readReply(r); ok {
		return reply.HookSpecificOutput.AdditionalContext != ""
	}
	return slices.Contains(contextEvents, event) && strings.TrimSpace(r.Stdout) != ""
}

type hookReply struct {
	Continue           *bool  `json:"continue"`
	Decision           string `json:"decision"`
	HookSpecificOutput struct {
		PermissionDecision string `json:"permissionDecision"`
		AdditionalContext  string `json:"additionalContext"`
		Decision           string `json:"decision"`
	} `json:"hookSpecificOutput"`
}

func readReply(r Result) (hookReply, bool) {
	var reply hookReply
	out := strings.TrimSpace(r.Stdout)
	if !strings.HasPrefix(out, "{") || json.Unmarshal([]byte(out), &reply) != nil {
		return reply, false
	}
	return reply, true
}
