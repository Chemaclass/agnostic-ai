package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func setupHookCompareFixture(t *testing.T) string {
	t.Helper()
	dir := newProject(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "targets: [claude, cursor]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/hooks/guard.yaml"), `name: guard
on: before-tool
match: shell
command: echo hook-check
timeout: 15
failClosed: true
`)
	writeFile(t, filepath.Join(dir, ".agnostic-ai/hooks/claude-only.yaml"), `name: claude-only
on: before-tool
match: shell
command: echo private-check
target-exclude: cursor
`)
	return dir
}

func TestCompare_ReportsHookFieldsAndExclusions(t *testing.T) {
	testutil.Chdir(t, setupHookCompareFixture(t))
	silence(t)
	out := compareJSON(t, "claude", "cursor")
	for _, field := range []string{"on", "match", "command", "timeout", "failClosed"} {
		for _, target := range []string{"claude", "cursor"} {
			result := findCompareResult(t, out, ".agnostic-ai/hooks/guard.yaml", field, target)
			if len(result.Paths) == 0 {
				t.Errorf("%s %s must identify its emitted output: %+v", target, field, result)
			}
		}
	}
	result := findCompareResult(t, out, ".agnostic-ai/hooks/claude-only.yaml", "command", "cursor")
	if result.Status != statusExcluded {
		t.Errorf("excluded hook = %+v", result)
	}
}

func TestCompare_HookMappingsUseWrittenConfiguration(t *testing.T) {
	testutil.Chdir(t, setupHookCompareFixture(t))
	silence(t)
	out := compareJSON(t, "claude", "cursor")
	path := ".agnostic-ai/hooks/guard.yaml"
	for _, target := range []string{"claude", "cursor"} {
		for _, field := range []string{"on", "match", "command"} {
			r := findCompareResult(t, out, path, field, target)
			if r.Status != statusTranslated || strings.Contains(r.Reason, "cannot emit") {
				t.Errorf("%s %s = %+v, want translated from valid emission", target, field, r)
			}
		}
		if r := findCompareResult(t, out, path, "timeout", target); r.Status != statusPreserved {
			t.Errorf("%s timeout = %+v", target, r)
		}
	}
	for target, event := range map[string]string{"claude": "PreToolUse", "cursor": "preToolUse"} {
		r := findCompareResult(t, out, path, "on", target)
		if !strings.Contains(r.Reason, event) {
			t.Errorf("%s event must name actual mapping: %+v", target, r)
		}
	}
}

func TestCompare_HookMissingMappingAndIgnoredFailurePolicy(t *testing.T) {
	dir := setupHookCompareFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, ".agnostic-ai/hooks/stop.yaml", "name: stop\non: stop\ncommand: echo stop\nfailClosed: true\n")
	out := compareJSON(t, "claude", "cursor")
	missing := findCompareResult(t, out, ".agnostic-ai/hooks/stop.yaml", "on", "cursor")
	if missing.Status != statusUnsupported || !strings.Contains(missing.Reason, "stop") {
		t.Errorf("missing portable mapping = %+v", missing)
	}
	ignored := findCompareResult(t, out, ".agnostic-ai/hooks/stop.yaml", "failClosed", "claude")
	if ignored.Status != statusUnsupported || !strings.Contains(ignored.Reason, "ignores onFailure") {
		t.Errorf("retained native policy must keep its no-op reason: %+v", ignored)
	}
	written := findCompareResult(t, out, ".agnostic-ai/hooks/guard.yaml", "failClosed", "claude")
	if written.Status != statusTranslated {
		t.Errorf("synchronous PreToolUse policy = %+v", written)
	}
	writeFile(t, ".agnostic-ai/hooks/async.yaml", "name: async\nevent: PreToolUse\ncommand: echo async\nasync: true\nfailClosed: true\n")
	out = compareJSON(t, "claude", "cursor")
	if r := findCompareResult(t, out, ".agnostic-ai/hooks/async.yaml", "failClosed", "claude"); r.Status != statusUnsupported || !strings.Contains(r.Reason, "async") {
		t.Errorf("async failure policy = %+v", r)
	}
}

func TestCompare_HookOverridesUseNativeEvidenceWithoutChangingSpecs(t *testing.T) {
	testutil.Chdir(t, setupHookCompareFixture(t))
	silence(t)
	writeFile(t, ".agnostic-ai/hooks/native.yaml", `name: native
event: PreToolUse
matcher: Bash
command: echo base
args: [base]
x-cursor:
  event: preToolUse
  matcher: ^Shell$
  command: echo override
  args: [changed]
`)
	cfg, bundle, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	out, err := compareTargets(cfg, bundle, []string{"claude", "cursor"})
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("comparison mutated input metadata")
	}
	if !reflect.DeepEqual(out.Targets, []string{"claude", "cursor"}) {
		t.Errorf("target pair = %v", out.Targets)
	}
	for _, field := range []string{"event", "matcher", "command", "args"} {
		r := findCompareResult(t, out, ".agnostic-ai/hooks/native.yaml", field, "cursor")
		if len(r.Paths) == 0 || r.Status == statusUnknown || r.Status == statusUnsupported {
			t.Errorf("override %s = %+v", field, r)
		}
	}
	event := findCompareResult(t, out, ".agnostic-ai/hooks/native.yaml", "event", "cursor")
	if !strings.Contains(event.Reason, "preToolUse") {
		t.Errorf("event override = %+v", event)
	}
}

func TestCompare_HookOptInExplainsMissingAndWrittenOutput(t *testing.T) {
	dir := newProject(t)
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "targets: [claude, zed]\n")
	writeFile(t, ".agnostic-ai/hooks/worktree.yaml", "name: worktree\nevent: WorktreeCreate\ncommand: echo worktree\n")
	out := compareJSON(t, "claude", "zed")
	r := findCompareResult(t, out, ".agnostic-ai/hooks/worktree.yaml", "command", "zed")
	if r.Status != statusExcluded || !strings.Contains(r.Next, "outputs.zed.tasks-file") {
		t.Errorf("missing opt-in = %+v", r)
	}
	writeFile(t, "agnostic-ai.yaml", "targets: [claude, zed]\noutputs:\n  zed:\n    tasks-file: .zed/tasks.json\n")
	out = compareJSON(t, "claude", "zed")
	r = findCompareResult(t, out, ".agnostic-ai/hooks/worktree.yaml", "command", "zed")
	if r.Status == statusExcluded || len(r.Paths) == 0 {
		t.Errorf("enabled task output = %+v", r)
	}
}

func TestCompare_HookTextJSONAgreeAndNeverRunOrWrite(t *testing.T) {
	dir := setupHookCompareFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, ".agnostic-ai/hooks/not-run.yaml", "name: not-run\nevent: SessionStart\ncommand: echo executed > hook-was-run\n")
	writeFile(t, ".claude/settings.json", `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo hand-written"}]}]}}`)
	before := snapshotTree(t, dir)
	out := compareJSON(t, "claude", "cursor")
	text, err := runCompare(t, "claude", "cursor")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Caveat, "a hook ran") || !strings.Contains(text, out.Caveat) {
		t.Errorf("missing execution caveat: %q\n%s", out.Caveat, text)
	}
	for _, s := range out.Specs {
		if !strings.Contains(text, s.Path) {
			t.Errorf("text omits %s", s.Path)
		}
		for _, f := range s.Fields {
			for _, r := range f.Results {
				if !strings.Contains(text, string(r.Status)) || r.Reason != "" && !strings.Contains(text, r.Reason) {
					t.Errorf("text omits JSON result %+v", r)
				}
			}
		}
	}
	second, err := runCompare(t, "claude", "cursor")
	if err != nil {
		t.Fatal(err)
	}
	if text != second {
		t.Error("hook comparison output is unstable")
	}
	if after := snapshotTree(t, dir); !reflect.DeepEqual(before, after) {
		t.Error("hook comparison changed project files")
	}
	if _, err := os.Stat("hook-was-run"); !os.IsNotExist(err) {
		t.Error("hook was executed")
	}
}

func TestCompare_HookCommandEvidenceIgnoresUnchangedHandlers(t *testing.T) {
	testutil.Chdir(t, setupHookCompareFixture(t))
	silence(t)
	writeFile(t, ".agnostic-ai/hooks/native.yaml", "name: native\nevent: preToolUse\ncommand: echo base\nargs: [changed]\n")
	writeFile(t, ".cursor/hooks.json", `{"version":1,"hooks":{"sessionStart":[{"command":"echo base"}]}}`)
	out := compareJSON(t, "claude", "cursor")
	r := findCompareResult(t, out, ".agnostic-ai/hooks/native.yaml", "command", "cursor")
	if r.Status != statusTranslated {
		t.Errorf("an unrelated unchanged command must not prove preservation: %+v", r)
	}
}

func TestCompare_HookCommandProbeDoesNotMatchSource(t *testing.T) {
	testutil.Chdir(t, setupHookCompareFixture(t))
	silence(t)
	writeFile(t, ".agnostic-ai/hooks/probe.yaml", "name: probe\nevent: SessionStart\ncommand: printf agnostic_ai_compare_command\n")
	out := compareJSON(t, "claude", "cursor")
	r := findCompareResult(t, out, ".agnostic-ai/hooks/probe.yaml", "command", "claude")
	if r.Status != statusPreserved {
		t.Errorf("existing probe text must still compare its command: %+v", r)
	}
}

func TestCompare_HookCommandProbeDoesNotMatchTargetOverride(t *testing.T) {
	testutil.Chdir(t, setupHookCompareFixture(t))
	silence(t)
	writeFile(t, ".agnostic-ai/hooks/probe-override.yaml", `name: probe-override
event: SessionStart
command: echo base
x-cursor:
  command: printf agnostic_ai_compare_command
`)
	out := compareJSON(t, "claude", "cursor")
	r := findCompareResult(t, out, ".agnostic-ai/hooks/probe-override.yaml", "command", "cursor")
	if r.Status != statusPreserved {
		t.Errorf("override matching the probe must still compare its command: %+v", r)
	}
}

func TestCompare_HookCommandProbeKeepsArgumentFormValid(t *testing.T) {
	testutil.Chdir(t, setupHookCompareFixture(t))
	silence(t)
	writeFile(t, ".agnostic-ai/hooks/probe-args.yaml", `name: probe-args
event: SessionStart
command: printf
args: [agnostic_ai_compare_command]
`)
	out := compareJSON(t, "claude", "cursor")
	for target, status := range map[string]compareStatus{"claude": statusPreserved, "cursor": statusTranslated} {
		r := findCompareResult(t, out, ".agnostic-ai/hooks/probe-args.yaml", "command", target)
		if r.Status != status || len(r.Paths) == 0 {
			t.Errorf("%s argument-form command = %+v, want %s with output", target, r, status)
		}
	}
}
