package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeKiroHookFile drops one `.kiro/hooks/<name>.json` under root.
func writeKiroHookFile(t *testing.T, root, name, body string) {
	t.Helper()
	writeFile(t, filepath.Join(root, filepath.FromSlash(kiroHooksDir), name+".json"), body)
}

// importKiroHooksInto runs the hook importer against root and returns
// the hook spec directory it wrote into.
func importKiroHooksInto(t *testing.T, root string) (string, int) {
	t.Helper()
	dst := filepath.Join(root, "hooks")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	n, err := importKiroHooks(root, dst)
	if err != nil {
		t.Fatalf("importKiroHooks: %v", err)
	}
	return dst, n
}

// specNames lists the yaml stems written into a hook spec dir.
func specNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	return out
}

// A hook file with no `.kiro/hooks` directory imports nothing rather
// than failing, the same no-op every other hook importer applies to a
// missing source.
func TestImportKiroHooks_MissingDirectoryImportsNothing(t *testing.T) {
	dir := t.TempDir()
	_, n := importKiroHooksInto(t, dir)
	if n != 0 {
		t.Errorf("imported %d hooks from an empty project, want 0", n)
	}
}

// The plain command hook the kiro adapter emits comes back as a
// portable spec: `trigger` -> `event`, `action.command` -> `command`,
// `enabled: false` -> `disabled: true`.
func TestImportKiroHooks_ReadsPortableCommandHook(t *testing.T) {
	dir := t.TempDir()
	writeKiroHookFile(t, dir, "lint", `{
  "version": "v1",
  "hooks": [
    {
      "name": "lint",
      "description": "Lint edited files.",
      "trigger": "PostToolUse",
      "matcher": "Edit",
      "action": {"type": "command", "command": "npx eslint --fix"},
      "timeout": 30,
      "enabled": false
    }
  ]
}`)

	dst, n := importKiroHooksInto(t, dir)
	if n != 1 {
		t.Fatalf("imported %d hooks, want 1", n)
	}
	got := readFile(t, filepath.Join(dst, "lint.yaml"))
	for _, want := range []string{
		"name: lint",
		"event: PostToolUse",
		"matcher: Edit",
		"command: npx eslint --fix",
		"description: Lint edited files.",
		"timeout: 30",
		"disabled: true",
		"target: kiro",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("imported spec missing %q:\n%s", want, got)
		}
	}
}

// `timeout: 0` is the vendor's documented way to disable the timeout,
// so it has to survive as an explicit key rather than read as absent.
func TestImportKiroHooks_KeepsExplicitZeroTimeout(t *testing.T) {
	dir := t.TempDir()
	writeKiroHookFile(t, dir, "slow", `{
  "version": "v1",
  "hooks": [{"name": "slow", "trigger": "Stop",
    "action": {"type": "command", "command": "./long.sh"}, "timeout": 0}]
}`)

	dst, _ := importKiroHooksInto(t, dir)
	got := readFile(t, filepath.Join(dst, "slow.yaml"))
	if !strings.Contains(got, "timeout: 0") {
		t.Errorf("explicit zero timeout dropped:\n%s", got)
	}
}

// The grouping rule: entries sharing trigger, matcher and description
// and differing only in `action.command` recombine into one spec with a
// `command:` list, dropping the emitter's `-2`/`-3` name suffixes.
func TestImportKiroHooks_RecombinesCommandListIntoOneSpec(t *testing.T) {
	dir := t.TempDir()
	writeKiroHookFile(t, dir, "multi", `{
  "version": "v1",
  "hooks": [
    {"name": "multi", "trigger": "PostToolUse", "matcher": "Edit",
     "description": "Three steps.", "action": {"type": "command", "command": "one"}},
    {"name": "multi-2", "trigger": "PostToolUse", "matcher": "Edit",
     "description": "Three steps.", "action": {"type": "command", "command": "two"}},
    {"name": "multi-3", "trigger": "PostToolUse", "matcher": "Edit",
     "description": "Three steps.", "action": {"type": "command", "command": "three"}}
  ]
}`)

	dst, n := importKiroHooksInto(t, dir)
	if n != 1 {
		t.Fatalf("imported %d hooks, want 1 grouped spec: %v", n, specNames(t, dst))
	}
	got := readFile(t, filepath.Join(dst, "multi.yaml"))
	for _, want := range []string{"- one", "- two", "- three", "name: multi"} {
		if !strings.Contains(got, want) {
			t.Errorf("grouped spec missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "multi-2") {
		t.Errorf("grouped spec kept the emitter's name suffix:\n%s", got)
	}
}

// Entries differing only in trigger are separate hooks: a spec holds
// exactly one `event`, so one file with two triggers splits in two.
func TestImportKiroHooks_SplitsOneFileWithSeveralTriggers(t *testing.T) {
	dir := t.TempDir()
	writeKiroHookFile(t, dir, "pair", `{
  "version": "v1",
  "hooks": [
    {"name": "pair", "trigger": "PreToolUse", "action": {"type": "command", "command": "guard"}},
    {"name": "pair-after", "trigger": "PostToolUse", "action": {"type": "command", "command": "guard"}}
  ]
}`)

	dst, n := importKiroHooksInto(t, dir)
	if n != 2 {
		t.Fatalf("imported %d hooks, want 2: %v", n, specNames(t, dst))
	}
	pre := readFile(t, filepath.Join(dst, "pair.yaml"))
	if !strings.Contains(pre, "event: PreToolUse") {
		t.Errorf("first spec kept the wrong event:\n%s", pre)
	}
	post := readFile(t, filepath.Join(dst, "pair-after.yaml"))
	if !strings.Contains(post, "event: PostToolUse") {
		t.Errorf("second spec kept the wrong event:\n%s", post)
	}
}

// The vendor's own example names a hook "Lint on save". The filename
// slugs; the original stays on the spec's `name:`.
func TestImportKiroHooks_SlugsFilenameAndKeepsSpacedName(t *testing.T) {
	dir := t.TempDir()
	writeKiroHookFile(t, dir, "whatever", `{
  "version": "v1",
  "hooks": [{"name": "Lint on save", "trigger": "PostFileSave",
    "action": {"type": "command", "command": "npx eslint --fix"}}]
}`)

	dst, _ := importKiroHooksInto(t, dir)
	got := readFile(t, filepath.Join(dst, "lint-on-save.yaml"))
	if !strings.Contains(got, "name: Lint on save") {
		t.Errorf("spec lost the vendor's display name:\n%s", got)
	}
}

// Kiro hook filenames are free-form, so two files can declare the same
// hook name. Both must survive, the second under the deterministic
// hookSpecName every other hook importer falls back to.
func TestImportKiroHooks_KeepsBothHooksOnANameCollision(t *testing.T) {
	dir := t.TempDir()
	buf := captureSummary(t)
	writeKiroHookFile(t, dir, "a-first", `{
  "version": "v1",
  "hooks": [{"name": "lint", "trigger": "PostToolUse",
    "action": {"type": "command", "command": "first"}}]
}`)
	writeKiroHookFile(t, dir, "b-second", `{
  "version": "v1",
  "hooks": [{"name": "lint", "trigger": "PostToolUse",
    "action": {"type": "command", "command": "second"}}]
}`)

	dst, n := importKiroHooksInto(t, dir)
	if n != 2 {
		t.Fatalf("imported %d hooks, want 2: %v", n, specNames(t, dst))
	}
	names := specNames(t, dst)
	if len(names) != 2 {
		t.Fatalf("wrote %d spec files, want 2: %v", len(names), names)
	}
	fallback := hookSpecName("PostToolUse", "", []string{"second"})
	renamed := readFile(t, filepath.Join(dst, fallback+".yaml"))
	if !strings.Contains(renamed, "name: "+fallback) {
		t.Errorf("colliding spec did not take the deterministic name:\n%s", renamed)
	}
	if !strings.Contains(buf.String(), "lint") {
		t.Errorf("expected a warning naming the colliding hook, got:\n%s", buf.String())
	}
}

// A third file repeating a hook the deterministic fallback already
// covers is the same hook twice over, so it is skipped rather than
// written under a third name.
func TestImportKiroHooks_SkipsAThirdCopyOfTheSameHook(t *testing.T) {
	dir := t.TempDir()
	buf := captureSummary(t)
	for _, f := range []string{"a-first", "b-second", "c-third"} {
		writeKiroHookFile(t, dir, f, `{
  "version": "v1",
  "hooks": [{"name": "lint", "trigger": "PostToolUse",
    "action": {"type": "command", "command": "run"}}]
}`)
	}

	dst, n := importKiroHooksInto(t, dir)
	if n != 2 {
		t.Fatalf("imported %d hooks, want 2: %v", n, specNames(t, dst))
	}
	if !strings.Contains(buf.String(), "an earlier file declares the same hook") {
		t.Errorf("expected a duplicate warning, got:\n%s", buf.String())
	}
}

// `type: command` with no `command` and `type: agent` with no `prompt`
// are both "Cond." in the vendor table. Writing either would produce a
// spec that fails the next sync, so they are skipped with a warning.
func TestImportKiroHooks_SkipsConditionallyInvalidActions(t *testing.T) {
	dir := t.TempDir()
	buf := captureSummary(t)
	writeKiroHookFile(t, dir, "broken", `{
  "version": "v1",
  "hooks": [
    {"name": "no-command", "trigger": "Stop", "action": {"type": "command"}},
    {"name": "no-prompt", "trigger": "Stop", "action": {"type": "agent"}},
    {"name": "no-trigger", "action": {"type": "command", "command": "x"}},
    {"name": "ok", "trigger": "Stop", "action": {"type": "command", "command": "x"}}
  ]
}`)

	dst, n := importKiroHooksInto(t, dir)
	if n != 1 {
		t.Fatalf("imported %d hooks, want only the valid one: %v", n, specNames(t, dst))
	}
	out := buf.String()
	for _, want := range []string{"no-command", "no-prompt", "no-trigger"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected a warning naming %q, got:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "ok.yaml")); err != nil {
		t.Errorf("the valid hook alongside the broken ones was dropped: %v", err)
	}
}

// An agent action has no portable home: spec-format.md scopes the
// `type: prompt` handler to Claude Code, Cursor and Copilot. It lands
// under `x-kiro.action`, the same key the emitter reads back.
func TestImportKiroHooks_RoutesAgentActionToNativeMeta(t *testing.T) {
	dir := t.TempDir()
	writeKiroHookFile(t, dir, "review", `{
  "version": "v1",
  "hooks": [{"name": "review", "trigger": "Stop",
    "action": {"type": "agent", "prompt": "Summarize the changes."}}]
}`)

	dst, _ := importKiroHooksInto(t, dir)
	got := readFile(t, filepath.Join(dst, "review.yaml"))
	for _, want := range []string{"x-kiro:", "action:", "type: agent", "prompt: Summarize the changes."} {
		if !strings.Contains(got, want) {
			t.Errorf("agent action missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "\ncommand:") {
		t.Errorf("agent action must not become a portable command:\n%s", got)
	}
}

// No closed schema is published for `.kiro/hooks/*.json`, unlike
// Crush's `additionalProperties: false`, so an unknown key lands under
// `x-kiro` instead of being dropped. `confirm` rides the same route.
func TestImportKiroHooks_KeepsUnknownKeysUnderNativeMeta(t *testing.T) {
	dir := t.TempDir()
	writeKiroHookFile(t, dir, "submit", `{
  "version": "v1",
  "hooks": [{"name": "submit", "trigger": "Stop",
    "action": {"type": "command", "command": "./submit.sh"},
    "futureKey": "kept",
    "confirm": {
      "question": "Submit this session's results?",
      "confirmCommand": "./confirm-options.sh",
      "options": [{"id": "submit", "label": "Yes, submit", "run": true}]
    }}]
}`)

	dst, _ := importKiroHooksInto(t, dir)
	got := readFile(t, filepath.Join(dst, "submit.yaml"))
	for _, want := range []string{
		"x-kiro:", "confirm:", "confirmCommand: ./confirm-options.sh",
		"question: Submit this session's results?", "futureKey: kept",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("native passthrough missing %q:\n%s", want, got)
		}
	}
}

// `enabled: true` is the vendor default and the emitter writes no key
// for it, so it must not reach the spec as `disabled: false`.
func TestImportKiroHooks_DropsExplicitEnabledTrue(t *testing.T) {
	dir := t.TempDir()
	writeKiroHookFile(t, dir, "on", `{
  "version": "v1",
  "hooks": [{"name": "on", "trigger": "Stop", "enabled": true,
    "action": {"type": "command", "command": "x"}}]
}`)

	dst, _ := importKiroHooksInto(t, dir)
	got := readFile(t, filepath.Join(dst, "on.yaml"))
	if strings.Contains(got, "disabled") || strings.Contains(got, "enabled") {
		t.Errorf("explicit enabled:true should leave no key:\n%s", got)
	}
}

// A malformed hook file names itself in the error, so the user can find
// the file without a stack trace.
func TestImportKiroHooks_MalformedFileNamesItself(t *testing.T) {
	dir := t.TempDir()
	writeKiroHookFile(t, dir, "bad", "{not json")

	if _, err := importKiroHooks(dir, filepath.Join(dir, "hooks")); err == nil {
		t.Fatal("expected a parse error")
	} else if !strings.Contains(err.Error(), "bad.json") {
		t.Errorf("error does not name the file: %v", err)
	}
}

// `import kiro` reports hooks in its summary line now that it reads
// them, instead of claiming the kinds it covers without the new one.
func TestImportFromKiro_ReportsHooksInSummary(t *testing.T) {
	dir := t.TempDir()
	buf := captureSummary(t)
	writeKiroHookFile(t, dir, "lint", `{
  "version": "v1",
  "hooks": [{"name": "lint", "trigger": "PostToolUse",
    "action": {"type": "command", "command": "gofmt -l ."}}]
}`)

	if err := importFromKiro(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "1 hooks") {
		t.Errorf("summary does not report hooks:\n%s", buf.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "hooks", "lint.yaml")); err != nil {
		t.Errorf("hook spec not written: %v", err)
	}
}
