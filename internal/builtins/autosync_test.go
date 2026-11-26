package builtins_test

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/builtins"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func loadAutosyncHook(t *testing.T) spec.Entry {
	t.Helper()
	cacheForTest(t)
	root, cleanup, err := builtins.Materialize("autosync")
	if err != nil {
		t.Fatalf("materialize autosync: %v", err)
	}
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Errorf("cleanup %s: %v", root, err)
		}
	})
	bundle, err := spec.LoadLayered([]spec.Layer{{Name: "builtin", Root: root, Sources: config.Sources{Hooks: "hooks"}}})
	if err != nil {
		t.Fatalf("load builtin layer: %v", err)
	}
	if len(bundle.All()) != 1 || len(bundle.Hooks) != 1 {
		t.Fatalf("autosync entries = %v, want one hook", bundle.All())
	}
	return bundle.Hooks[0]
}

func hookTargets(hook spec.Entry) []string {
	var out []string
	list, _ := hook.Meta["targets"].([]any)
	for _, item := range list {
		out = append(out, fmt.Sprint(item))
	}
	return out
}

// The list in the spec must follow the adapters: a target that gains or
// loses a session-start event fails here until the spec catches up.
func TestAutosync_TargetsEveryTargetWithASessionStartEvent(t *testing.T) {
	hook := loadAutosyncHook(t)
	want := spec.PortableEventTargets("session-start")
	got := slices.Sorted(slices.Values(hookTargets(hook)))
	for _, target := range spec.PortableHookTargets() {
		if !slices.Contains(want, target) && hook.EmitsTo(target) {
			t.Errorf("autosync reaches %s, which has no session-start event", target)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("autosync targets = %v, want %v", got, want)
	}
	if on, _ := hook.Meta["on"].(string); on != "session-start" {
		t.Errorf("on = %q, want session-start", on)
	}
}

func TestAutosync_EachTargetRunsTheHiddenCommandForItself(t *testing.T) {
	hook := loadAutosyncHook(t)
	for _, target := range hookTargets(hook) {
		command, _ := hook.Meta["command"].(string)
		if target != "claude" {
			override, _ := hook.Meta["x-"+target].(map[string]any)
			command, _ = override["command"].(string)
		}
		want := fmt.Sprintf(`"$bin" project -- hook autosync --target %s >/dev/null; exit 0`, target)
		if !strings.Contains(command, want) {
			t.Errorf("%s command lacks %q:\n%s", target, want, command)
		}
		if !strings.Contains(command, `command -v "$bin" >/dev/null || exit 0`) {
			t.Errorf("%s command fails when the binary is missing:\n%s", target, command)
		}
	}
	windows := map[string]string{"codex": "", "copilot": "copilot"}
	for target, key := range windows {
		command, _ := hook.Meta["commandWindows"].(string)
		if key != "" {
			override, _ := hook.Meta["x-"+key].(map[string]any)
			command, _ = override["commandWindows"].(string)
		}
		want := fmt.Sprintf("& $bin project -- hook autosync --target %s > $null; exit 0", target)
		if !strings.Contains(command, want) {
			t.Errorf("%s Windows command lacks %q:\n%s", target, want, command)
		}
	}
	for _, banned := range []string{"\u2013", "\u2014"} {
		if desc, _ := hook.Meta["description"].(string); strings.Contains(desc, banned) {
			t.Errorf("description has a long dash: %q", desc)
		}
	}
}
