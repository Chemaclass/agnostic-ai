package cli

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/hookrun"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// claudeProjectDirEnv is the project root Claude Code gives every hook.
const claudeProjectDirEnv = "CLAUDE_PROJECT_DIR"

func newHookRunCmd() *cobra.Command {
	var only []string
	var in hookrun.Input
	var payloadFile, expect string
	cmd := &cobra.Command{
		Use:   "run <hook>",
		Short: "Run a hook spec with each target's payload and report what each target decides",
		Long: "Runs the hook spec named <hook> once for every target it reaches, with the payload, " +
			"environment, shell, and timeout that target gives it, from the project root. " +
			"Builds payloads for " + strings.Join(hookrun.Targets(), " and ") + "; other targets are listed as not run. " +
			"Prints each command's decision (allow, block, error, or timeout), exit code, time, stdout, and stderr. " +
			"Exits 1 when a command times out or errors, when targets decide differently, or when a decision differs from --expect. " +
			"Run sync first: commands run the scripts sync copied into each target's hook directory.",
		Example: `  # Check that one protect-files hook blocks on Claude Code and Codex alike
  agnostic-ai hook run protect-files --edit .github/workflows/tests.yml --expect block

  # A Bash guard, on Codex only
  agnostic-ai hook run guard --target codex --bash "git push --force"

  # An event with no payload builder
  agnostic-ai hook run on-stop --payload stop.json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if expect != "" && expect != string(hookrun.Allow) && expect != string(hookrun.Block) {
				return fmt.Errorf("--expect: expected allow or block, got %q", expect)
			}
			if payloadFile != "" {
				if in.Edit != "" || in.Bash != "" || in.Prompt != "" {
					return fmt.Errorf("--payload replaces --edit, --bash, and --prompt")
				}
				raw, err := os.ReadFile(payloadFile)
				if err != nil {
					return fmt.Errorf("%s: %w", payloadFile, err)
				}
				in.Raw = raw
			}
			cfg, b, err := loadProject(".")
			if err != nil {
				return err
			}
			hook, ok := findHook(b, args[0])
			if !ok {
				return fmt.Errorf("no hook named %s under hooks/", args[0])
			}
			targets := slices.DeleteFunc(slices.Clone(cfg.Targets), func(t string) bool { return !hook.EmitsTo(t) })
			for _, t := range only {
				if !slices.Contains(targets, t) {
					return fmt.Errorf("hook %s does not reach %s; it reaches %s", hook.Name, t, strings.Join(targets, ", "))
				}
			}
			if len(only) > 0 {
				targets = slices.DeleteFunc(targets, func(t string) bool { return !slices.Contains(only, t) })
			}
			root, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			runs, err := runHookTargets(cmd.OutOrStdout(), hook, targets, root, in)
			if err != nil {
				return err
			}
			return judgeHookRuns(hook.Name, runs, hookrun.Decision(expect))
		},
	}
	cmd.Flags().StringSliceVarP(&only, "target", "t", nil, "Run only for these targets (default: every target the hook reaches)")
	cmd.Flags().StringVar(&in.Edit, "edit", "", "Build a tool event that edits this path")
	cmd.Flags().StringVar(&in.Bash, "bash", "", "Build a tool event that runs this shell command")
	cmd.Flags().StringVar(&in.Prompt, "prompt", "", "Prompt text for a UserPromptSubmit event")
	cmd.Flags().StringVar(&payloadFile, "payload", "", "Send this JSON file to every target as the payload")
	cmd.Flags().StringVar(&expect, "expect", "", "Fail unless every target decides allow or block")
	_ = cmd.RegisterFlagCompletionFunc("target", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return hookrun.Targets(), cobra.ShellCompDirectiveNoFileComp
	})
	_ = cmd.RegisterFlagCompletionFunc("expect", cobra.FixedCompletions([]string{"allow", "block"}, cobra.ShellCompDirectiveNoFileComp))
	return cmd
}

func findHook(b spec.Bundle, name string) (spec.Entry, bool) {
	for _, h := range b.Hooks {
		if h.Name == name {
			return h, true
		}
	}
	return spec.Entry{}, false
}

// hookTargetRun is one target's combined decision. failed holds every
// timeout or error among its commands, even when another one blocked.
type hookTargetRun struct {
	target   string
	decision hookrun.Decision
	failed   []hookrun.Decision
}

func runHookTargets(w io.Writer, hook spec.Entry, targets []string, root string, in hookrun.Input) ([]hookTargetRun, error) {
	event, _ := hook.Meta["event"].(string)
	matcher, _ := hook.Meta["matcher"].(string)
	timeout := hookTimeout(hook.Meta)
	var runs []hookTargetRun
	for _, target := range targets {
		if !hookrun.Supported(target) {
			_, _ = fmt.Fprintf(w, "%s: not run (hook run builds no %s payload)\n", target, target)
			continue
		}
		handlers := adapters.HookHandlers(target, hook)
		if len(handlers) == 0 {
			_, _ = fmt.Fprintf(w, "%s: not run (no command handler; hook run runs command hooks only)\n", target)
			continue
		}
		payload, err := hookrun.Build(target, event, matcher, root, in)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", target, err)
		}
		if !payload.Fires {
			_, _ = fmt.Fprintf(w, "%s: allow (not run: matcher %q does not match %s)\n", target, matcher, payload.Trigger)
			runs = append(runs, hookTargetRun{target: target, decision: hookrun.Allow})
			continue
		}
		env := hookRunEnv(target, root)
		run := hookTargetRun{target: target, decision: hookrun.Allow}
		for _, h := range handlers {
			r := hookrun.Run(hookrun.Argv(target, runtime.GOOS, h), root, env, payload.Body, timeout)
			d := hookrun.Decide(event, r)
			printHookRun(w, target, event, payload.Trigger, h, r, d)
			run.decision = strongerDecision(run.decision, d)
			if d == hookrun.Timeout || d == hookrun.Error {
				run.failed = append(run.failed, d)
			}
		}
		if adapters.JudgesHooks(target) {
			if reason := adapters.AcceptsHook(target, hook.Meta); reason != "" {
				_, _ = fmt.Fprintf(w, "  note: %s\n", reason)
			}
		}
		runs = append(runs, run)
	}
	if len(runs) == 0 {
		return nil, fmt.Errorf("hook %s reaches no target hook run builds payloads for (%s)", hook.Name, strings.Join(hookrun.Targets(), ", "))
	}
	return runs, nil
}

// strongerDecision combines the handlers of one event: any block stops
// the event, and a timeout or error outranks a plain allow.
func strongerDecision(a, b hookrun.Decision) hookrun.Decision {
	rank := map[hookrun.Decision]int{hookrun.Allow: 0, hookrun.Error: 1, hookrun.Timeout: 2, hookrun.Block: 3}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

func printHookRun(w io.Writer, target, event, trigger string, h hookrun.Handler, r hookrun.Result, d hookrun.Decision) {
	elapsed := r.Elapsed.Round(time.Millisecond)
	switch {
	case r.TimedOut:
		_, _ = fmt.Fprintf(w, "%s: %s (after %s)\n", target, d, elapsed)
	case r.StartErr != nil:
		_, _ = fmt.Fprintf(w, "%s: %s (did not start: %v)\n", target, d, r.StartErr)
	default:
		_, _ = fmt.Fprintf(w, "%s: %s (exit %d, %s)\n", target, d, r.Exit, elapsed)
	}
	_, _ = fmt.Fprintf(w, "  event: %s (%s)\n", event, trigger)
	command := h.Command
	if runtime.GOOS == "windows" && h.CommandWindows != "" {
		command = h.CommandWindows
	}
	_, _ = fmt.Fprintf(w, "  command: %s\n", strings.Join(append([]string{command}, h.Args...), " "))
	for _, stream := range []struct{ name, text string }{{"stdout", r.Stdout}, {"stderr", r.Stderr}} {
		if text := strings.TrimSpace(stream.text); text != "" {
			_, _ = fmt.Fprintf(w, "  %s: %s\n", stream.name, strings.ReplaceAll(text, "\n", "\n          "))
		}
	}
	if hookrun.AddsContext(event, r) {
		_, _ = fmt.Fprintf(w, "  context: %s adds the output to the session\n", target)
	}
}

// hookRunEnv is the environment target gives a hook. The variables a
// session sets are dropped first, so a run from inside Claude Code does
// not hand them to Codex.
func hookRunEnv(target, root string) []string {
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool {
		key, _, _ := strings.Cut(kv, "=")
		return strings.EqualFold(key, adapters.HookTargetEnv) || strings.EqualFold(key, claudeProjectDirEnv)
	})
	if target == "claude" {
		env = append(env, adapters.HookTargetEnv+"=claude", claudeProjectDirEnv+"="+root)
	}
	return env
}

func hookTimeout(meta map[string]any) time.Duration {
	var seconds int
	switch v := meta["timeout"].(type) {
	case int:
		seconds = v
	case int64:
		seconds = int(v)
	case float64:
		seconds = int(v)
	}
	if seconds <= 0 {
		return hookrun.DefaultTimeout
	}
	return time.Duration(seconds) * time.Second
}

// judgeHookRuns fails on a command that timed out or errored, on a
// decision other than expect, and on targets that disagree.
func judgeHookRuns(name string, runs []hookTargetRun, expect hookrun.Decision) error {
	var timedOut, errored, summary []string
	for _, r := range runs {
		summary = append(summary, r.target+" "+string(r.decision))
		if slices.Contains(r.failed, hookrun.Timeout) {
			timedOut = append(timedOut, r.target)
		}
		if slices.Contains(r.failed, hookrun.Error) {
			errored = append(errored, r.target)
		}
	}
	if len(timedOut) > 0 {
		return fmt.Errorf("hook %s timed out on %s", name, strings.Join(timedOut, ", "))
	}
	if len(errored) > 0 {
		return fmt.Errorf("hook %s failed on %s", name, strings.Join(errored, ", "))
	}
	if expect != "" {
		for _, r := range runs {
			if r.decision != expect {
				return fmt.Errorf("hook %s: expected %s, got %s", name, expect, strings.Join(summary, ", "))
			}
		}
	}
	for _, r := range runs[1:] {
		if r.decision != runs[0].decision {
			return fmt.Errorf("hook %s: targets decide differently: %s", name, strings.Join(summary, ", "))
		}
	}
	return nil
}
