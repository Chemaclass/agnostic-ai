package opencode

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// defaultPluginsDir is the project-level plugin directory OpenCode
// loads at startup: "Place JavaScript or TypeScript files in the plugin
// directory. `.opencode/plugins/` - Project-level plugins ... Files in
// these directories are automatically loaded at startup."
// (opencode.ai/docs/plugins/, re-read 2026-09-19).
const defaultPluginsDir = ".opencode/plugins"

// pluginHost adds OpenCode's TUI events to the shared bus vocabulary;
// Kilo does not list that group.
var pluginHost = func() emit.PluginHookHost {
	events := emit.PluginBusEvents()
	for _, e := range []string{"tui.prompt.append", "tui.command.execute", "tui.toast.show"} {
		events[e] = true
	}
	return emit.PluginHookHost{
		Target: target, Tool: "OpenCode", ImportModule: "@opencode-ai/plugin", BusEvents: events,
	}
}()

type hookPlan struct {
	v1, v2 []spec.Entry
}

func prepareHooks(hooks []spec.Entry) (hookPlan, error) {
	var plan hookPlan
	for _, hook := range hooks {
		api, err := hookPluginAPI(hook)
		if err != nil {
			return hookPlan{}, err
		}
		if api == "v2" {
			if err := validateV2Hook(hook); err != nil {
				return hookPlan{}, err
			}
			plan.v2 = append(plan.v2, hook)
		} else {
			plan.v1 = append(plan.v1, hook)
		}
	}
	return plan, nil
}

func emitHooks(sess *emit.Session, hooks []spec.Entry, dir string, dryRun bool) error {
	plan, err := prepareHooks(hooks)
	if err != nil {
		return err
	}
	return plan.emit(sess, dir, dryRun)
}

func (plan hookPlan) emit(sess *emit.Session, dir string, dryRun bool) error {
	if err := emit.EmitPluginHooks(sess, pluginHost, plan.v1, dir, dryRun); err != nil {
		return err
	}
	for _, hook := range plan.v2 {
		if err := emitV2Hook(sess, hook, dir, dryRun); err != nil {
			return err
		}
	}
	return nil
}

func hookPluginAPI(hook spec.Entry) (string, error) {
	meta, _ := hook.Meta["x-opencode"].(map[string]any)
	value, exists := meta["plugin-api"]
	if !exists {
		return "v1", nil
	}
	api, ok := value.(string)
	if ok && (api == "v1" || api == "v2") {
		return api, nil
	}
	return "", fmt.Errorf("%s: opencode plugin-api must be v1 or v2", hookLocation(hook))
}

func hookLocation(hook spec.Entry) string {
	if hook.Path != "" {
		return hook.Path
	}
	return hook.Name
}

// HookEvents lists every event a hook spec may name for this target, so
// `validate` reads the same vocabulary the emitter maps.
func HookEvents() []string {
	return pluginHost.Events()
}
