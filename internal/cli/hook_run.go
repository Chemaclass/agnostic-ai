package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/hookrun"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// claudeProjectDirEnv is the project root Claude Code gives every hook.
const claudeProjectDirEnv = "CLAUDE_PROJECT_DIR"

// notRun is the decision of a target hook run cannot run the hook for.
const notRun hookrun.Decision = "not run"

func newHookRunCmd() *cobra.Command {
	var only []string
	var in hookrun.Input
	var payloadFile, expect, format string
	var includeAssumed bool
	cmd := &cobra.Command{
		Use:   "run <hook>",
		Short: "Run a hook spec with each target's payload and report what each target decides",
		Long: "Runs the hook spec named <hook> once for every target it reaches, with the payload, " +
			"environment, shell, and timeout that target gives it, from the project root. " +
			"Builds payloads for " + strings.Join(hookrun.Targets(), " and ") + "; other targets are listed as not run. " +
			"Prints each command's decision (allow, block, error, or timeout), exit code, time, stdout, and stderr, " +
			"and warns when the synced native file does not run the command the spec produces. " +
			"--format json prints one object per target instead. " +
			"A target whose docs leave out its shell, working directory, or default timeout runs on stated assumptions, marked (assumed: ...); " +
			"its result is shown but not counted unless --include-assumed is passed, and a disagreement prints a warning. " +
			"Exits 1 when a counted command times out or errors, when counted targets decide differently, or when a decision differs from --expect. " +
			"Run sync first: commands run the scripts sync copied into each target's hook directory.",
		Example: `  # Check that one protect-files hook blocks on Claude Code and Codex alike
  agnostic-ai hook run protect-files --edit .github/workflows/tests.yml --expect block

  # A Bash guard, on Codex only
  agnostic-ai hook run guard --target codex --bash "git push --force"

  # An event with no payload builder
  agnostic-ai hook run on-stop --payload stop.json

  # Per-target results for a CI job
  agnostic-ai hook run protect-files --edit .env --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if expect != "" && expect != string(hookrun.Allow) && expect != string(hookrun.Block) {
				return fmt.Errorf("--expect: expected allow or block, got %q", expect)
			}
			if format != "text" && format != "json" {
				return fmt.Errorf("--format: expected text or json, got %q", format)
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
			show := func(r hookTargetRun) { printHookTarget(cmd.OutOrStdout(), r) }
			if format == "json" {
				show = func(hookTargetRun) {}
			}
			runs, err := runHookTargets(cfg, hook, targets, root, in, show)
			if err != nil {
				return err
			}
			runs = countHookRuns(runs, hookrun.Decision(expect), includeAssumed)
			if format == "text" {
				printAssumedSummary(cmd.OutOrStdout(), runs, includeAssumed)
			}
			judged := judgeHookRuns(hook.Name, runs, hookrun.Decision(expect))
			if format == "json" {
				if err := printHookRunJSON(cmd.OutOrStdout(), hook.Name, runs, judged); err != nil {
					return err
				}
			}
			return judged
		},
	}
	cmd.Flags().StringSliceVarP(&only, "target", "t", nil, "Run only for these targets (default: every target the hook reaches)")
	cmd.Flags().StringVar(&in.Edit, "edit", "", "Build a tool event that edits this path")
	cmd.Flags().StringVar(&in.Bash, "bash", "", "Build a tool event that runs this shell command")
	cmd.Flags().StringVar(&in.Prompt, "prompt", "", "Prompt text for a UserPromptSubmit event")
	cmd.Flags().StringVar(&payloadFile, "payload", "", "Send this JSON file to every target as the payload")
	cmd.Flags().StringVar(&expect, "expect", "", "Fail unless every target decides allow or block")
	cmd.Flags().StringVar(&format, "format", "text", "Output format: text or json")
	cmd.Flags().BoolVar(&includeAssumed, "include-assumed", false, "Count results that rest on an assumed shell, working directory, or timeout in --expect and the comparison")
	_ = cmd.RegisterFlagCompletionFunc("target", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return hookrun.Targets(), cobra.ShellCompDirectiveNoFileComp
	})
	_ = cmd.RegisterFlagCompletionFunc("expect", cobra.FixedCompletions([]string{"allow", "block"}, cobra.ShellCompDirectiveNoFileComp))
	_ = cmd.RegisterFlagCompletionFunc("format", cobra.FixedCompletions([]string{"text", "json"}, cobra.ShellCompDirectiveNoFileComp))
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
// An async run is not judged: the target does not wait for its result.
type hookTargetRun struct {
	Target   string           `json:"target"`
	Decision hookrun.Decision `json:"decision"`
	Reason   string           `json:"reason,omitempty"`
	Event    string           `json:"event,omitempty"`
	Trigger  string           `json:"trigger,omitempty"`
	Async    bool             `json:"async"`
	Commands []hookCommandRun `json:"commands"`
	Notes    []string         `json:"notes"`
	Warnings []string         `json:"warnings"`
	// Assumptions are the contract items hook run filled in because the
	// target's docs leave them out.
	Assumptions []hookrun.Assumption `json:"assumptions"`
	// Counted is whether the result takes part in --expect and the
	// comparison.
	Counted bool `json:"counted"`
	failed  []hookrun.Decision
	// fireAndForget marks an event the target never waits on, whatever
	// the spec's async says.
	fireAndForget bool
	// disagreement is the warning for an uncounted result that differs
	// from the counted ones, printed after every target.
	disagreement string
	// uncounted is why the result is not counted even with
	// --include-assumed: the target does not document what it does with it.
	uncounted string
}

type hookCommandRun struct {
	Command     string           `json:"command"`
	Decision    hookrun.Decision `json:"decision"`
	ExitCode    *int             `json:"exit_code"`
	ElapsedMS   int64            `json:"elapsed_ms"`
	TimedOut    bool             `json:"timed_out"`
	StartError  string           `json:"start_error,omitempty"`
	Stdout      string           `json:"stdout"`
	Stderr      string           `json:"stderr"`
	AddsContext bool             `json:"adds_context"`
	result      hookrun.Result
}

func runHookTargets(cfg *config.Config, hook spec.Entry, targets []string, root string, in hookrun.Input, show func(hookTargetRun)) ([]hookTargetRun, error) {
	event, _ := hook.Meta["event"].(string)
	matcher, _ := hook.Meta["matcher"].(string)
	var runs []hookTargetRun
	add := func(r hookTargetRun) {
		show(r)
		runs = append(runs, r)
	}
	for _, target := range targets {
		run := hookTargetRun{Target: target, Decision: notRun, Commands: []hookCommandRun{}, Notes: []string{}, Warnings: []string{}, Assumptions: []hookrun.Assumption{}}
		if !hookrun.Supported(target) {
			run.Reason = fmt.Sprintf("hook run builds no %s payload", target)
			add(run)
			continue
		}
		if _, ok := hookEventsByTarget[target][event]; !ok {
			run.Reason = fmt.Sprintf("%s has no %s event", target, event)
			add(run)
			continue
		}
		handlers, err := adapters.HookHandlers(cfg, target, hook)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", target, err)
		}
		if len(handlers) == 0 {
			run.Reason = "no command handler; hook run runs command hooks only"
			add(run)
			continue
		}
		if target == "augment" && !slices.ContainsFunc(handlers, func(h hookrun.Handler) bool { return hookrun.AugmentRuns(h.Command) }) {
			run.Reason = "Augment runs only a hook command that is a .sh, .ps1, .cmd, or .bat script path"
			add(run)
			continue
		}
		payload, err := hookrun.Build(target, event, matcher, root, in)
		var unbuilt hookrun.Unbuilt
		if errors.As(err, &unbuilt) {
			run.Reason = unbuilt.Reason
			add(run)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", target, err)
		}
		run.Event, run.Trigger, run.Decision = event, payload.Trigger, hookrun.Allow
		run.Warnings = append(run.Warnings, hookFileWarnings(cfg, target, event, adapters.HookNativeMatcher(target, event, matcher), root, handlers)...)
		if !payload.Fires {
			run.Reason = fmt.Sprintf("matcher %q does not match %s", matcher, payload.Trigger)
			add(run)
			continue
		}
		// A matcher that does not fire runs nothing, so it assumes nothing.
		assumptions, reason := hookAssumptions(target, handlers)
		if reason != "" {
			run.Decision, run.Reason = notRun, reason
			add(run)
			continue
		}
		run.Assumptions = assumptions
		// Claude Code and Qoder write the spec's `if` on every handler.
		if rule := handlers[0].If; (target == "claude" || target == "qoder") && rule != "" {
			runs, err := hookrun.IfRuns(target, rule, event, payload.Body, root)
			if errors.As(err, &unbuilt) {
				run.Decision, run.Reason = notRun, unbuilt.Reason
				add(run)
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("%s: %w", target, err)
			}
			if !runs {
				run.Reason = fmt.Sprintf("if %q does not match this %s call", rule, payload.Trigger)
				add(run)
				continue
			}
		}
		// Gemini CLI has no async hooks; sync writes none there.
		run.Async = hookRunAsync(hook.Meta) && slices.Contains(asyncHookTargets, target)
		if hookrun.FireAndForget(target, event) {
			run.Async, run.fireAndForget = true, true
		}
		var crushResults []hookrun.Result
		if target == "crush" {
			handlers = hookrun.CrushDedupe(handlers)
			env := hookRunEnv(target, root, hookEnvContext{}, hookrun.Handler{})
			crushResults = hookrun.RunCrushHooks(handlers, root, hookrun.CrushEnv(env, root, payload.Body), payload.Body, hookTimeout(target, hook.Meta), runtime.GOOS)
		}
		for i, h := range handlers {
			timeout := h.Timeout
			if timeout <= 0 {
				timeout = hookTimeout(target, hook.Meta)
			}
			shown := shownHookCommand(h)
			h.Command = hookrun.ExpandCommand(target, runtime.GOOS, h.Command, root)
			if target == "augment" && !hookrun.AugmentRuns(h.Command) {
				continue
			}
			env := hookRunEnv(target, root, hookEnvContext{event: event, tool: hookrun.PayloadTool(payload.Body), pluginRoot: adapters.HookPluginRoot(cfg, target)}, h)
			dir := root
			if target == "copilot" {
				dir = hookrun.CopilotDir(root, h)
				h = hookrun.CopilotExec(root, h)
			}
			var r hookrun.Result
			if target == "crush" {
				r = crushResults[i]
				run.Assumptions = mergeAssumptions(run.Assumptions, hookrun.CrushAssumptions(r))
			} else {
				r = hookrun.Run(hookrun.Argv(target, runtime.GOOS, h), dir, env, payload.Body, timeout)
			}
			d := hookrun.DecideHandler(target, event, h, r)
			if target == "qoder" && d == hookrun.Block && hookrun.QoderPolicyChange(event, payload.Body) {
				d = hookrun.Error
				run.Notes = append(run.Notes, "Qoder enforces a policy_settings change; the hook runs for audit and cannot block it")
			}
			if target == "cursor" && hookrun.CursorAsks(event, r) {
				run.Notes = append(run.Notes, "replied ask: Cursor asks the user before the action runs; read as block")
			}
			if target == "crush" && hookrun.CrushHalts(r) {
				run.Notes = append(run.Notes, "halt: Crush ends the whole turn, not only this tool call")
			}
			if target == "factory" && hookrun.FactoryAsks(event, r) {
				run.Notes = append(run.Notes, "replied ask: Factory asks the user before the tool runs; read as block")
			}
			if target == "antigravity" {
				if note := hookrun.AntigravityNote(event, r); note != "" {
					run.Notes = append(run.Notes, note)
				}
				if reason := hookrun.AntigravityUncounted(event, r); reason != "" && run.uncounted == "" {
					run.uncounted = reason
					run.Notes = append(run.Notes, "not counted: "+reason)
				}
			}
			if target == "qoder" && hookrun.QoderAsks(event, r) {
				run.Notes = append(run.Notes, "replied ask: Qoder asks the user before the tool runs; read as block")
			}
			if target == "copilot" && hookrun.CopilotAsks(event, r) {
				run.Notes = append(run.Notes, "replied ask: Copilot CLI asks the user before the tool runs, and cloud agent denies it; read as block")
			}
			run.Commands = append(run.Commands, newHookCommandRun(shown, d, r, hookrun.AddsContext(target, event, r)))
			run.Decision = strongerDecision(run.Decision, d)
			if d == hookrun.Timeout || d == hookrun.Error {
				run.failed = append(run.failed, d)
			}
			if target == "copilot" && hookrun.CopilotErrored(event, r) {
				run.failed = append(run.failed, hookrun.Error)
				run.Notes = append(run.Notes, "the hook failed, and Copilot denies a preToolUse hook that fails; read as block")
			}
		}
		if target == "copilot" {
			copilotMergeDecision(&run, event)
		}
		if adapters.JudgesHooks(target) {
			if reason := adapters.AcceptsHook(target, hook.Meta); reason != "" {
				run.Notes = append(run.Notes, reason)
			}
		}
		add(run)
	}
	return runs, nil
}

func newHookCommandRun(command string, d hookrun.Decision, r hookrun.Result, addsContext bool) hookCommandRun {
	c := hookCommandRun{
		Command: command, Decision: d, ElapsedMS: r.Elapsed.Milliseconds(), TimedOut: r.TimedOut,
		Stdout: r.Stdout, Stderr: r.Stderr, AddsContext: addsContext, result: r,
	}
	switch {
	case r.StartErr != nil:
		c.StartError = r.StartErr.Error()
	case !r.TimedOut:
		exit := r.Exit
		c.ExitCode = &exit
	}
	return c
}

// shownHookCommand is the command the target starts on this platform,
// as the native file spells it.
func shownHookCommand(h hookrun.Handler) string {
	command := h.Command
	if runtime.GOOS == "windows" && h.CommandWindows != "" {
		command = h.CommandWindows
	}
	return strings.Join(append([]string{command}, h.Args...), " ")
}

// hookFileWarnings names each handler the synced native file of target
// does not run, so a run that passes cannot hide a stale file.
func hookFileWarnings(cfg *config.Config, target, event, matcher, root string, handlers []hookrun.Handler) []string {
	file := adapters.HookFile(cfg, target)
	path := file
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	shown := filepath.ToSlash(file)
	body, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return []string{shown + " does not exist; run agnostic-ai sync"}
	}
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", shown, err)}
	}
	covers := func(native, spec string) bool { return adapters.HookMatcherCovers(target, native, spec) }
	drift, err := hookrun.Drift(target, body, event, matcher, runtime.GOOS, handlers, covers)
	if err != nil {
		return []string{fmt.Sprintf("%s: parse: %v", shown, err)}
	}
	var warnings []string
	for _, d := range drift {
		warnings = append(warnings, fmt.Sprintf("%s %s; run agnostic-ai sync", shown, d.Reason))
	}
	return warnings
}

// copilotMergeDecision decides a Copilot permissionRequest from its
// merged output instead of the strongest command, since a later hook's
// behavior overrides an earlier one's.
func copilotMergeDecision(run *hookTargetRun, event string) {
	results := make([]hookrun.Result, 0, len(run.Commands))
	for _, c := range run.Commands {
		results = append(results, c.result)
	}
	blocks, ok := hookrun.CopilotMergedBlocks(event, results)
	if !ok || len(results) == 0 {
		return
	}
	run.Decision = hookrun.Allow
	for _, c := range run.Commands {
		if c.Decision != hookrun.Block {
			run.Decision = strongerDecision(run.Decision, c.Decision)
		}
	}
	if blocks {
		run.Decision = hookrun.Block
	}
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

// hookAssumptions merges what hook run assumes for each handler, or
// returns why one handler cannot run.
func hookAssumptions(target string, handlers []hookrun.Handler) ([]hookrun.Assumption, string) {
	out := []hookrun.Assumption{}
	for _, h := range handlers {
		assumed, reason := hookrun.Assumptions(target, runtime.GOOS, h)
		if reason != "" {
			return nil, reason
		}
		out = mergeAssumptions(out, assumed)
	}
	return out, ""
}

// mergeAssumptions adds each assumption whose item out does not list yet.
func mergeAssumptions(out, more []hookrun.Assumption) []hookrun.Assumption {
	for _, a := range more {
		if !slices.ContainsFunc(out, func(b hookrun.Assumption) bool { return b.Item == a.Item }) {
			out = append(out, a)
		}
	}
	return out
}

func assumedItems(run hookTargetRun) string {
	items := make([]string, 0, len(run.Assumptions))
	for _, a := range run.Assumptions {
		items = append(items, a.Item)
	}
	return strings.Join(items, ", ")
}

// countHookRuns marks which results take part in --expect and the
// comparison, and warns on each result left out that disagrees with
// them, so a skipped check is never silent.
func countHookRuns(runs []hookTargetRun, expect hookrun.Decision, includeAssumed bool) []hookTargetRun {
	reference := expect
	for i := range runs {
		r := &runs[i]
		r.Counted = r.Decision != notRun && !r.Async && r.uncounted == "" && (len(r.Assumptions) == 0 || includeAssumed)
		if r.Counted && reference == "" {
			reference = r.Decision
		}
	}
	for i := range runs {
		r := &runs[i]
		if r.Counted || r.Decision == notRun || r.Async || r.uncounted != "" || len(r.Assumptions) == 0 || reference == "" || r.Decision == reference {
			continue
		}
		r.disagreement = fmt.Sprintf("assumed result %s differs from %s and is not counted; pass --include-assumed to count it", r.Decision, reference)
		r.Warnings = append(r.Warnings, r.disagreement)
	}
	return runs
}

// printAssumedSummary counts the results that rested on assumptions, and
// repeats each disagreement, so CI logs show the reduced coverage.
func printAssumedSummary(w io.Writer, runs []hookTargetRun, includeAssumed bool) {
	checked, assumed, uncounted := 0, 0, 0
	for _, r := range runs {
		if r.Decision == notRun || r.Async {
			continue
		}
		if len(r.Assumptions) > 0 {
			assumed++
		}
		if r.Counted {
			checked++
		}
		if r.uncounted != "" {
			uncounted++
		}
	}
	if assumed == 0 {
		return
	}
	for _, r := range runs {
		if r.disagreement != "" {
			_, _ = fmt.Fprintf(w, "%s: warning: %s\n", r.Target, r.disagreement)
		}
	}
	state := "not counted; --include-assumed to count"
	if includeAssumed {
		state = "counted"
	}
	switch {
	case uncounted == 1:
		state += "; 1 undocumented result not counted"
	case uncounted > 1:
		state += fmt.Sprintf("; %d undocumented results not counted", uncounted)
	}
	_, _ = fmt.Fprintf(w, "\n%d checked, %d assumed (%s)\n", checked, assumed, state)
}

func printHookTarget(w io.Writer, run hookTargetRun) {
	switch {
	case run.Decision == notRun:
		_, _ = fmt.Fprintf(w, "%s: not run (%s)\n", run.Target, run.Reason)
	case run.Reason != "":
		_, _ = fmt.Fprintf(w, "%s: allow (not run: %s)\n", run.Target, run.Reason)
	}
	for _, c := range run.Commands {
		d := c.Decision
		if run.Async {
			d = "not judged"
		}
		printHookRun(w, run.Target, run.Event, run.Trigger, c, d, assumedItems(run))
		switch {
		case run.fireAndForget:
			_, _ = fmt.Fprintf(w, "  note: %s runs %s fire-and-forget; it does not wait for the result\n", run.Target, run.Event)
		case run.Async:
			_, _ = fmt.Fprintf(w, "  note: async hook; %s does not wait for its result\n", run.Target)
		}
	}
	for _, a := range run.Assumptions {
		_, _ = fmt.Fprintf(w, "  assumed %s: %s (%s)\n", a.Item, a.Value, a.Reason)
	}
	if docs := hookrun.ContractDocs(run.Target); docs != "" && len(run.Assumptions) > 0 {
		_, _ = fmt.Fprintf(w, "  docs: %s\n", docs)
	}
	for _, note := range run.Notes {
		_, _ = fmt.Fprintf(w, "  note: %s\n", note)
	}
	for _, warning := range run.Warnings {
		_, _ = fmt.Fprintf(w, "  warning: %s\n", warning)
	}
}

func printHookRun(w io.Writer, target, event, trigger string, c hookCommandRun, d hookrun.Decision, assumed string) {
	r := c.result
	elapsed := r.Elapsed.Round(time.Millisecond)
	suffix := ""
	if assumed != "" {
		suffix = " (assumed: " + assumed + ")"
	}
	switch {
	case r.TimedOut:
		_, _ = fmt.Fprintf(w, "%s: %s (after %s)%s\n", target, d, elapsed, suffix)
	case r.StartErr != nil:
		_, _ = fmt.Fprintf(w, "%s: %s (did not start: %v)%s\n", target, d, r.StartErr, suffix)
	default:
		_, _ = fmt.Fprintf(w, "%s: %s (exit %d, %s)%s\n", target, d, r.Exit, elapsed, suffix)
	}
	_, _ = fmt.Fprintf(w, "  event: %s (%s)\n", event, trigger)
	_, _ = fmt.Fprintf(w, "  command: %s\n", c.Command)
	for _, stream := range []struct{ name, text string }{{"stdout", r.Stdout}, {"stderr", r.Stderr}} {
		if text := strings.TrimSpace(stream.text); text != "" {
			_, _ = fmt.Fprintf(w, "  %s: %s\n", stream.name, strings.ReplaceAll(text, "\n", "\n          "))
		}
	}
	if c.AddsContext {
		_, _ = fmt.Fprintf(w, "  context: %s adds the output to the session\n", target)
	}
}

// printHookRunJSON prints every target's result and the reason the run
// fails, when it does.
func printHookRunJSON(w io.Writer, name string, runs []hookTargetRun, failure error) error {
	report := struct {
		Hook    string          `json:"hook"`
		Targets []hookTargetRun `json:"targets"`
		Error   string          `json:"error,omitempty"`
	}{Hook: name, Targets: runs}
	if report.Targets == nil {
		report.Targets = []hookTargetRun{}
	}
	if failure != nil {
		report.Error = failure.Error()
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(report)
}

// sessionEnvKeys are the variables a target session sets for its hooks.
// They are dropped first, so a run from inside one session does not hand
// them to another target.
var sessionEnvKeys = []string{
	adapters.HookTargetEnv, claudeProjectDirEnv, "GEMINI_PROJECT_DIR", "GEMINI_CWD", "GEMINI_SESSION_ID", "GEMINI_PLANS_DIR",
	"TRAE_PROJECT_DIR", "OPENHANDS_PROJECT_DIR", "OPENHANDS_SESSION_ID", "OPENHANDS_EVENT_TYPE", "OPENHANDS_TOOL_NAME",
	"PLUGIN_ROOT", "CURSOR_PROJECT_DIR", "CURSOR_VERSION", "CURSOR_USER_EMAIL", "CURSOR_TRANSCRIPT_PATH", "CURSOR_CODE_REMOTE", "FACTORY_PROJECT_DIR", "AUGMENT_PROJECT_DIR", "AUGMENT_CONVERSATION_ID", "AUGMENT_HOOK_EVENT", "AUGMENT_TOOL_NAME",
	"QODER_PROJECT_DIR", "QODER_PLUGIN_ROOT", "QODER_PLUGIN_DATA",
	"CRUSH_EVENT", "CRUSH_TOOL_NAME", "CRUSH_SESSION_ID", "CRUSH_CWD", "CRUSH_PROJECT_DIR", "CRUSH_TOOL_INPUT_COMMAND", "CRUSH_TOOL_INPUT_FILE_PATH",
}

// asyncHookTargets run an `async: true` hook in the background, so its
// result blocks nothing: Claude Code, Codex, OpenHands, and Qoder.
var asyncHookTargets = []string{"claude", "codex", "openhands", "qoder"}

// hookEnvContext is what a target's hook env names about the event.
type hookEnvContext struct {
	event, tool, pluginRoot string
}

// hookRunEnv is the environment target gives handler h.
func hookRunEnv(target, root string, ctx hookEnvContext, h hookrun.Handler) []string {
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool {
		key, _, _ := strings.Cut(kv, "=")
		return slices.ContainsFunc(sessionEnvKeys, func(k string) bool { return strings.EqualFold(k, key) })
	})
	switch target {
	case "claude":
		env = append(env, adapters.HookTargetEnv+"=claude", claudeProjectDirEnv+"="+root)
	case "gemini":
		// hookRunner.ts sets these, then spreads the handler's env over them.
		env = append(env, "GEMINI_PROJECT_DIR="+root, "GEMINI_CWD="+root,
			"GEMINI_SESSION_ID="+hookrun.SessionID, claudeProjectDirEnv+"="+root)
	case "trae":
		env = append(env, "TRAE_PROJECT_DIR="+root, claudeProjectDirEnv+"="+root)
	case "openhands":
		// executor.py sets these on every command hook.
		env = append(env, "OPENHANDS_PROJECT_DIR="+root, "OPENHANDS_SESSION_ID="+hookrun.SessionID, "OPENHANDS_EVENT_TYPE="+ctx.event)
		if ctx.tool != "" {
			env = append(env, "OPENHANDS_TOOL_NAME="+ctx.tool)
		}
	case "goose":
		env = append(env, "PLUGIN_ROOT="+filepath.Join(root, filepath.FromSlash(ctx.pluginRoot)))
	case "cursor":
		// The sessionStart entry sync adds sets AGNOSTIC_AI_TARGET for
		// every later hook in the session.
		env = append(env, "CURSOR_PROJECT_DIR="+root, "CURSOR_VERSION=", claudeProjectDirEnv+"="+root, adapters.HookTargetEnv+"=cursor")
	case "factory":
		env = append(env, "FACTORY_PROJECT_DIR="+root)
	case "qoder":
		env = append(env, "QODER_PROJECT_DIR="+root)
	case "augment":
		env = append(env, "AUGMENT_PROJECT_DIR="+root, "AUGMENT_CONVERSATION_ID="+hookrun.SessionID, "AUGMENT_HOOK_EVENT="+ctx.event)
		if ctx.tool != "" {
			env = append(env, "AUGMENT_TOOL_NAME="+ctx.tool)
		}
	}
	keys := make([]string, 0, len(h.Env))
	for k := range h.Env {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		env = append(env, k+"="+h.Env[k])
	}
	return env
}

// hookRunAsync reads the spec's async field: both targets run such a
// hook in the background, so its exit code blocks nothing.
func hookRunAsync(meta map[string]any) bool {
	switch v := meta["async"].(type) {
	case bool:
		return v
	case string:
		return v == "true"
	}
	return false
}

func hookTimeout(target string, meta map[string]any) time.Duration {
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
		event, _ := meta["event"].(string)
		return hookrun.DefaultTimeout(target, event)
	}
	return time.Duration(seconds) * time.Second
}

// judgeHookRuns fails when no target ran the hook, on a command that
// timed out or errored, on a decision other than expect, and on
// targets that disagree.
func judgeHookRuns(name string, runs []hookTargetRun, expect hookrun.Decision) error {
	var notRunReasons []string
	for _, r := range runs {
		if r.Decision == notRun && r.Reason != "" {
			notRunReasons = append(notRunReasons, r.Target+": "+r.Reason)
		}
	}
	runs = slices.DeleteFunc(slices.Clone(runs), func(r hookTargetRun) bool { return r.Decision == notRun })
	if len(runs) == 0 {
		if len(notRunReasons) > 0 {
			return fmt.Errorf("hook %s ran on no target (%s)", name, strings.Join(notRunReasons, "; "))
		}
		return fmt.Errorf("hook %s reaches no target hook run builds payloads for (%s)", name, strings.Join(hookrun.Targets(), ", "))
	}
	var assumedOnly, undocumented []string
	for _, r := range runs {
		switch {
		case r.Async || r.Counted:
		case r.uncounted != "":
			undocumented = append(undocumented, r.Target+": "+r.uncounted)
		case len(r.Assumptions) > 0:
			assumedOnly = append(assumedOnly, r.Target+": "+string(r.Decision))
		}
	}
	runs = slices.DeleteFunc(runs, func(r hookTargetRun) bool { return r.Async || !r.Counted })
	if len(runs) == 0 && len(assumedOnly) > 0 && expect != "" {
		return fmt.Errorf("hook %s: --expect checks nothing, since it ran only where hook run assumes part of the contract (%s); pass --include-assumed to count it", name, strings.Join(assumedOnly, ", "))
	}
	if len(runs) == 0 && len(undocumented) > 0 && expect != "" {
		return fmt.Errorf("hook %s: --expect checks nothing, since no result it got is one the target documents (%s)", name, strings.Join(undocumented, "; "))
	}
	if len(runs) == 0 {
		return nil
	}
	var timedOut, errored, summary []string
	for _, r := range runs {
		summary = append(summary, r.Target+" "+string(r.Decision))
		if slices.Contains(r.failed, hookrun.Timeout) {
			timedOut = append(timedOut, r.Target)
		}
		if slices.Contains(r.failed, hookrun.Error) {
			errored = append(errored, r.Target)
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
			if r.Decision != expect {
				return fmt.Errorf("hook %s: expected %s, got %s", name, expect, strings.Join(summary, ", "))
			}
		}
	}
	for _, r := range runs[1:] {
		if r.Decision != runs[0].Decision {
			return fmt.Errorf("hook %s: targets decide differently: %s", name, strings.Join(summary, ", "))
		}
	}
	return nil
}
