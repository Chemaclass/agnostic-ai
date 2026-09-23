package opencode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func emitHookSpec(t *testing.T, e spec.Entry) (dir string) {
	t.Helper()
	dir = testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{e}), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	return dir
}

// PreToolUse maps onto the `tool.execute.before` key the vendor's own
// .env protection example returns from the plugin function, and the
// matcher becomes the `input.tool` guard that example writes by hand.
func TestEmit_Hook_PreToolUseWritesToolExecuteBefore(t *testing.T) {
	dir := emitHookSpec(t, spec.Entry{
		Kind: spec.KindHook, Name: "guard-bash",
		Meta: map[string]any{"event": "PreToolUse", "matcher": "bash", "command": "./scripts/guard.sh"},
	})

	got := readFile(t, filepath.Join(dir, ".opencode/plugins/guard-bash.ts"))
	for _, want := range []string{
		`import type { Plugin } from "@opencode-ai/plugin"`,
		`export const GuardBashPlugin: Plugin = async ({ $ }) => {`,
		`"tool.execute.before": async (input) => {`,
		`const MATCHER = new RegExp("^(?:bash)$")`,
		`if (!MATCHER.test(input.tool)) return`,
		"const r1 = await $`${{ raw: \"./scripts/guard.sh\" }}`.nothrow()",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// PostToolUse is the other half of the pair, and a hook with no matcher
// declares no handler parameters rather than two it never reads.
func TestEmit_Hook_PostToolUseWritesToolExecuteAfter(t *testing.T) {
	dir := emitHookSpec(t, spec.Entry{
		Kind: spec.KindHook, Name: "fmt-go",
		Meta: map[string]any{"event": "PostToolUse", "command": "gofmt -w ."},
	})

	got := readFile(t, filepath.Join(dir, ".opencode/plugins/fmt-go.ts"))
	for _, want := range []string{
		`"tool.execute.after": async () => {`,
		"await $`${{ raw: \"gofmt -w .\" }}`.nothrow()",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "new RegExp") {
		t.Errorf("matcher-less hook must emit no tool guard:\n%s", got)
	}
}

// A documented event-bus type rides the single `event` hook with an
// `event.type` guard, the shape the vendor's notification example uses
// for session.idle. It is not a key on the returned object.
func TestEmit_Hook_BusEventWritesEventHandler(t *testing.T) {
	dir := emitHookSpec(t, spec.Entry{
		Kind: spec.KindHook, Name: "notify-idle",
		Meta: map[string]any{"event": "session.idle", "command": "echo done"},
	})

	got := readFile(t, filepath.Join(dir, ".opencode/plugins/notify-idle.ts"))
	for _, want := range []string{
		`event: async ({ event }) => {`,
		`if (event.type !== "session.idle") return`,
		"await $`${{ raw: \"echo done\" }}`.nothrow()",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"session.idle":`) {
		t.Errorf("a bus event must not become a key on the hook object:\n%s", got)
	}
}

// The native spellings must survive unchanged, so a spec authored
// straight from the vendor doc is not reported as unmappable.
func TestEmit_Hook_NativeEventNamePassesThrough(t *testing.T) {
	dir := emitHookSpec(t, spec.Entry{
		Kind: spec.KindHook, Name: "native",
		Meta: map[string]any{"event": "tool.execute.before", "command": "echo hi"},
	})

	got := readFile(t, filepath.Join(dir, ".opencode/plugins/native.ts"))
	if !strings.Contains(got, `"tool.execute.before": async () => {`) {
		t.Errorf("native event name must map to itself:\n%s", got)
	}
}

// Every command in a list runs in the order it was authored.
func TestEmit_Hook_CommandListAwaitsEachInOrder(t *testing.T) {
	dir := emitHookSpec(t, spec.Entry{
		Kind: spec.KindHook, Name: "chain",
		Meta: map[string]any{"event": "PostToolUse", "command": []any{"first", "second"}},
	})

	got := readFile(t, filepath.Join(dir, ".opencode/plugins/chain.ts"))
	first := strings.Index(got, "await $`${{ raw: \"first\" }}`.nothrow()")
	second := strings.Index(got, "await $`${{ raw: \"second\" }}`.nothrow()")
	if first < 0 || second < 0 {
		t.Fatalf("both commands must emit:\n%s", got)
	}
	if first > second {
		t.Errorf("commands emitted out of order:\n%s", got)
	}
}

// A backtick, `${`, or backslash in the command reaches the shell
// exactly as authored: the command rides a `{ raw }` value, so nothing
// is escaped on the way.
func TestEmit_Hook_EscapesTemplateLiteralSyntax(t *testing.T) {
	dir := emitHookSpec(t, spec.Entry{
		Kind: spec.KindHook, Name: "tricky",
		Meta: map[string]any{"event": "PostToolUse", "command": "echo `id` ${HOME} \\d"},
	})

	got := readFile(t, filepath.Join(dir, ".opencode/plugins/tricky.ts"))
	if !strings.Contains(got, "await $`${{ raw: \"echo `id` ${HOME} \\\\d\" }}`.nothrow()") {
		t.Errorf("command not passed through verbatim:\n%s", got)
	}
}

// Claude spells "every tool" as `*`, which is not a valid regex on its
// own: `new RegExp("*")` throws and OpenCode never loads the module.
func TestEmit_Hook_WildcardMatcherEmitsNoGuard(t *testing.T) {
	dir := emitHookSpec(t, spec.Entry{
		Kind: spec.KindHook, Name: "all-tools",
		Meta: map[string]any{"event": "PreToolUse", "matcher": "*", "command": "echo hi"},
	})

	got := readFile(t, filepath.Join(dir, ".opencode/plugins/all-tools.ts"))
	if strings.Contains(got, "new RegExp") {
		t.Errorf("wildcard matcher must not compile into a regex guard:\n%s", got)
	}
}

// An event outside OpenCode's own vocabulary has no plugin hook to
// attach to, so it writes nothing and says so.
func TestEmit_Hook_UnmappableEventNotesTheGap(t *testing.T) {
	testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{
		{Kind: spec.KindHook, Name: "on-start", Meta: map[string]any{"event": "SessionStart", "command": "echo hi"}},
	}), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if _, err := os.Stat(".opencode/plugins/on-start.ts"); !os.IsNotExist(err) {
		t.Errorf("an unmappable event must write no plugin, stat err = %v", err)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "no plugin hook to attach to") {
		t.Errorf("expected a coverage note, got: %s", buf.String())
	}
}

// shell.env and experimental.session.compacting exist to rewrite the
// output object, not to run a command, so neither is forced into the
// generic shape.
func TestEmit_Hook_MutationOnlyEventNotesTheGap(t *testing.T) {
	testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{
		{Kind: spec.KindHook, Name: "inject", Meta: map[string]any{"event": "shell.env", "command": "echo hi"}},
	}), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if _, err := os.Stat(".opencode/plugins/inject.ts"); !os.IsNotExist(err) {
		t.Errorf("a mutation-only hook must write no plugin, stat err = %v", err)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "cannot express") {
		t.Errorf("expected a coverage note, got: %s", buf.String())
	}
}

// A Claude-cased matcher compiles and then matches nothing, because
// OpenCode's own tool names are lowercase. Emit the guard the author
// asked for, and name the trap.
func TestEmit_Hook_ClaudeCasedMatcherNotesTheNoOp(t *testing.T) {
	testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{
		{Kind: spec.KindHook, Name: "fmt", Meta: map[string]any{"event": "PostToolUse", "matcher": "Edit", "command": "echo hi"}},
	}), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	got := readFile(t, ".opencode/plugins/fmt.ts")
	if !strings.Contains(got, `new RegExp("^(?:Edit)$")`) {
		t.Errorf("the authored matcher must still emit:\n%s", got)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "lowercase") {
		t.Errorf("expected a matcher no-op note, got: %s", buf.String())
	}
}

// A bus event handler gets no tool name in its payload, so a matcher
// there has nowhere to go.
func TestEmit_Hook_MatcherOnBusEventNotesTheNoOp(t *testing.T) {
	testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{
		{Kind: spec.KindHook, Name: "idle", Meta: map[string]any{"event": "session.idle", "matcher": "bash", "command": "echo hi"}},
	}), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	got := readFile(t, ".opencode/plugins/idle.ts")
	if strings.Contains(got, "new RegExp") {
		t.Errorf("a bus event must carry no tool guard:\n%s", got)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "carries no tool name") {
		t.Errorf("expected a matcher no-op note, got: %s", buf.String())
	}
}

// A matcher that does not compile would throw at module load and stop
// OpenCode loading the plugin at all, so the guard is dropped instead.
func TestEmit_Hook_InvalidMatcherDropsGuardAndNotes(t *testing.T) {
	testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{
		{Kind: spec.KindHook, Name: "broken", Meta: map[string]any{"event": "PreToolUse", "matcher": "bash(", "command": "echo hi"}},
	}), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	got := readFile(t, ".opencode/plugins/broken.ts")
	if strings.Contains(got, "new RegExp") {
		t.Errorf("an uncompilable matcher must not reach the module:\n%s", got)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "JavaScript reads the same way") {
		t.Errorf("expected an invalid-matcher note, got: %s", buf.String())
	}
}

// A hook timeout has no counterpart: OpenCode awaits the handler with
// no deadline.
func TestEmit_Hook_TimeoutNotesTheNoOp(t *testing.T) {
	testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{
		{Kind: spec.KindHook, Name: "slow", Meta: map[string]any{"event": "PreToolUse", "command": "echo hi", "timeout": 30}},
	}), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "no deadline") {
		t.Errorf("expected a timeout no-op note, got: %s", buf.String())
	}
}

// outputs.opencode.hooks-dir moves the plugin directory.
func TestEmit_Hook_HooksDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)
	cfg := &config.Config{Outputs: map[string]config.Output{
		"opencode": {HooksDir: "custom/plugins"},
	}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{
		{Kind: spec.KindHook, Name: "moved", Meta: map[string]any{"event": "PreToolUse", "command": "echo hi"}},
	}), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/plugins/moved.ts")); err != nil {
		t.Errorf("hooks-dir override did not move the plugin: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".opencode/plugins/moved.ts")); !os.IsNotExist(err) {
		t.Errorf("plugin still written to the default dir, stat err = %v", err)
	}
}

// A spec missing either half of the pair produces no module rather than
// one that runs nothing.
func TestEmit_Hook_IncompleteSpecWritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{
		{Kind: spec.KindHook, Name: "no-command", Meta: map[string]any{"event": "PreToolUse"}},
		{Kind: spec.KindHook, Name: "no-event", Meta: map[string]any{"command": "echo hi"}},
	}), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".opencode/plugins")); !os.IsNotExist(err) {
		t.Errorf("expected no plugin directory, stat err = %v", err)
	}
}

// A module in the plugin directory always runs, so a disabled hook
// writes no module at all.
func TestEmit_Hook_DisabledWritesNothing(t *testing.T) {
	dir := emitHookSpec(t, spec.Entry{
		Kind: spec.KindHook, Name: "off",
		Meta: map[string]any{"event": "PostToolUse", "command": "echo hi", "disabled": true},
	})

	if _, err := os.Stat(filepath.Join(dir, ".opencode/plugins/off.ts")); !os.IsNotExist(err) {
		t.Errorf("a disabled hook must write no module, stat err = %v", err)
	}
}
