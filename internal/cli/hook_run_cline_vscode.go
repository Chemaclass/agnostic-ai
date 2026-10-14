package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/cline"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/hookrun"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func runClineVSCodeTaskStart(cfg *config.Config, source spec.Entry, hooks []spec.Entry, root string, in hookrun.Input, show func(hookTargetRun)) ([]hookTargetRun, error) {
	hook, reason := source.NativeHook("cline")
	hook = adapters.TargetHook("cline", hook)
	event, _ := hook.Meta["event"].(string)
	if path := cline.HookScriptPath(cfg, event); path != "" {
		event = filepath.Base(path)
	}
	if event != "TaskStart" {
		return nil, fmt.Errorf("runtime vscode models only Cline TaskStart, got %q", event)
	}
	run := hookTargetRun{Target: "cline", Event: event, Trigger: "task", Decision: notRun, Commands: []hookCommandRun{}, Notes: []string{}, Warnings: []string{}, Assumptions: []hookrun.Assumption{}}
	finish := func() ([]hookTargetRun, error) {
		show(run)
		return []hookTargetRun{run}, nil
	}
	if reason != "" {
		run.Reason = reason
		return finish()
	}
	if runtime.GOOS == "windows" {
		run.Reason = "Cline VS Code probes several PowerShell installations; hook run does not yet model its Windows launcher resolver"
		return finish()
	}
	path := cline.HookScriptPath(cfg, event)
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	stat, err := os.Stat(path)
	if err != nil {
		run.Reason = fmt.Sprintf("%s: %v; run agnostic-ai sync", path, err)
		return finish()
	}
	if !stat.Mode().IsRegular() || stat.Mode().Perm()&0o111 == 0 {
		run.Reason = "Cline VS Code discovers only an executable regular TaskStart file on this platform"
		return finish()
	}
	payload, err := hookrun.BuildClineVSCodeTaskStart(root, in)
	if err != nil {
		return nil, fmt.Errorf("cline vscode TaskStart: %w", err)
	}
	handlers, err := adapters.HookHandlers(cfg, "cline", hook)
	if err != nil {
		return nil, fmt.Errorf("cline vscode TaskStart: %w", err)
	}
	if len(handlers) == 0 {
		run.Reason = "no command handler; hook run runs command hooks only"
		return finish()
	}
	run.Warnings = hookFileWarnings(cfg, "cline", hook.Name, event, "", root, handlers)
	if siblings := adapters.HookScriptSiblings(cfg, "cline", spec.Bundle{Hooks: hooks}.HooksFor("cline"), hook); len(siblings) > 0 {
		run.sharedScript = true
		run.uncounted = hookrun.ClineSharedScript(siblings)
		run.Notes = append(run.Notes, "not counted: "+run.uncounted)
	}
	env := hookRunEnv("cline", root, hookEnvContext{event: event}, hookrun.Handler{})
	r := hookrun.Run([]string{"/bin/sh", "-c", `"$1"`, "agnostic-ai-cline-vscode", path}, root, env, payload.Body, hookrun.ClineVSCodeTimeout)
	decision, context, notes := hookrun.ClineVSCodeTaskStartResult(r)
	run.Decision = decision
	run.Notes = append(run.Notes, notes...)
	run.Commands = append(run.Commands, newHookCommandRun(path, decision, r, context))
	if decision == hookrun.Error || decision == hookrun.Timeout {
		run.failed = append(run.failed, decision)
	}
	return finish()
}
