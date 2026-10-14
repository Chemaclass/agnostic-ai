package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func builtinClaudeProject(t *testing.T, usableCache bool) string {
	t.Helper()
	silence(t)
	t.Cleanup(func() {
		if err := cleanupBuiltinLayers(); err != nil {
			t.Error(err)
		}
	})
	dir := testutil.TempCwd(t)
	t.Setenv(envUserGlobalRoot, t.TempDir())
	builtinCache(t, usableCache)
	writeFile(t, filepath.Join(dir, config.ConfigFileName), "version: 1\ntargets: [claude]\nbuiltins: [memory]\n")
	return dir
}

func readRawState(t *testing.T, dir string) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(stateFilePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

// stateWithoutTimestamp is the ledger a no-op sync must leave alone.
func stateWithoutTimestamp(t *testing.T, dir string) string {
	t.Helper()
	raw := readRawState(t, dir)
	delete(raw, "synced_at")
	out, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func syncOK(t *testing.T, args ...string) string {
	t.Helper()
	out, err := runBuiltinCLI(t, append([]string{"sync", "--gitignore=off"}, args...)...)
	if err != nil {
		t.Fatalf("sync %v: %v\n%s", args, err, out)
	}
	return out
}

func TestSyncState_BuiltinSpecsNeverKeyedByMaterializedPath(t *testing.T) {
	for name, usable := range map[string]bool{"cache": true, "temp fallback": false} {
		t.Run(name, func(t *testing.T) {
			dir := builtinClaudeProject(t, usable)
			syncOK(t)
			syncOK(t)
			first := stateWithoutTimestamp(t, dir)
			if strings.Contains(first, "agnostic-ai-builtin") || strings.Contains(first, "builtins/") {
				t.Errorf("state names a materialized built-in path:\n%s", first)
			}
			syncOK(t)
			if got := stateWithoutTimestamp(t, dir); got != first {
				t.Errorf("no-op syncs changed the state:\nfirst: %s\nlater: %s", first, got)
			}
		})
	}
}

func TestSyncState_UpgradeDropsMaterializedPathKeys(t *testing.T) {
	dir := builtinClaudeProject(t, false)
	syncOK(t)
	old := filepath.Join(t.TempDir(), "agnostic-ai-builtin-memory-123456")
	oldSpec := filepath.Join(old, "hooks", "memory-subagent-start.yaml")
	writeFile(t, oldSpec, "name: memory-subagent-start\n")
	state := readStateFile(dir)
	state.SpecFileSums = map[string]specFileSum{
		filepath.ToSlash(oldSpec): {Sum: sha256Hex([]byte("name: memory-subagent-start\n")), By: specSumBySync, Targets: []string{"claude", "codex"}},
	}
	state.Outputs = append(state.Outputs, "AGENTS.md")
	cfgPath := filepath.Join(dir, config.ConfigFileName)
	writeFile(t, cfgPath, "version: 1\ntargets: [claude, codex]\nbuiltins: [memory]\n")
	if err := os.WriteFile(stateFilePath(dir), mustJSON(t, state), 0o644); err != nil {
		t.Fatal(err)
	}
	syncOK(t, "--target", "claude")
	if got := stateWithoutTimestamp(t, dir); strings.Contains(got, "agnostic-ai-builtin") {
		t.Errorf("old key survived the upgrade sync:\n%s", got)
	}
	syncOK(t)
	if out, err := runBuiltinCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check after upgrade: %v\n%s", err, out)
	}
	syncOK(t)
	settled := stateWithoutTimestamp(t, dir)
	syncOK(t)
	if got := stateWithoutTimestamp(t, dir); got != settled {
		t.Errorf("state kept changing after the upgrade settled:\n%s\n%s", settled, got)
	}
}

func TestSyncSummary_NamesOnlySpecsAConfiguredTargetEmits(t *testing.T) {
	dir := builtinClaudeProject(t, true)
	syncOK(t)
	state := readStateFile(dir)
	if _, ok := state.SpecSums["hook memory-cline-task-start"]; ok {
		t.Fatalf("baseline holds a spec no configured target emits: %v", state.SpecSums)
	}
	state.SpecSums = map[string]string{"rule shared-memory-policy": "stale", "hook memory-cline-task-start": "stale", "rule antigravity-memory-index": "stale"}
	if err := os.WriteFile(stateFilePath(dir), mustJSON(t, state), 0o644); err != nil {
		t.Fatal(err)
	}
	buf := captureLogOut(t)
	syncOK(t, "-vv")
	out := buf.String()
	if !strings.Contains(out, "~ rule shared-memory-policy") {
		t.Errorf("summary misses the claude rule change:\n%s", out)
	}
	for _, other := range []string{"memory-cline-task-start", "antigravity-memory-index", "memory-windsurf-session-start", "memory-trae-session-start", "gemini-memory-tiers", "kiro-memory-index"} {
		if strings.Contains(out, other) {
			t.Errorf("summary names %s, which no configured target receives:\n%s", other, out)
		}
	}
}

func TestDiffSpecSums_OldBaselineKeepsUnemittedSpecsQuiet(t *testing.T) {
	prev := specBaseline{sums: map[string]string{"rule a": "1", "rule b": "1", "rule c": "1"}}
	cur := specBaseline{sums: map[string]string{"rule a": "1"}}
	got := diffSpecSums(prev, cur, map[string]bool{"rule a": true, "rule b": true})
	if len(got) != 1 || got[0] != (specChange{"-", "rule c"}) {
		t.Errorf("got %v, want only the removed rule c", got)
	}
}

func TestDiffSpecSums_NamesSpecWhoseTargetsChanged(t *testing.T) {
	prev := specBaseline{sums: map[string]string{"rule a": "1", "rule b": "1"}, targets: map[string][]string{"rule a": {"claude"}, "rule b": {"claude"}}}
	cur := specBaseline{sums: map[string]string{"rule a": "1"}, targets: map[string][]string{"rule a": {"claude", "codex"}}}
	got := diffSpecSums(prev, cur, map[string]bool{"rule a": true, "rule b": true})
	want := []specChange{{"~", "rule a"}, {"~", "rule b"}}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSyncSummary_NamesAgentEditOnTargetThatWritesAgentsAsSkills(t *testing.T) {
	dir := builtinClaudeProject(t, true)
	writeFile(t, filepath.Join(dir, config.ConfigFileName), "version: 1\ntargets: [crush]\noutputs:\n  crush:\n    agents: skill\n")
	agent := filepath.Join(dir, ".agnostic-ai", "agents", "helper.md")
	writeFile(t, agent, "---\nname: helper\ndescription: Helps\n---\nOne.\n")
	syncOK(t)
	writeFile(t, agent, "---\nname: helper\ndescription: Helps\n---\nTwo.\n")
	buf := captureLogOut(t)
	syncOK(t, "-vv")
	if out := buf.String(); !strings.Contains(out, "~ agent helper") {
		t.Errorf("summary misses the agent edit:\n%s", out)
	}
}

func TestSyncSummary_OneOffTargetOutsideConfigAddsNothingEveryRun(t *testing.T) {
	dir := builtinClaudeProject(t, true)
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\n---\nBe brief.\n")
	syncOK(t)
	for i := range 2 {
		buf := captureLogOut(t)
		syncOK(t, "--target", "gemini", "-vv")
		if out := buf.String(); strings.Contains(out, "+ rule gemini-memory-tiers") || strings.Contains(out, "+ rule style") {
			t.Errorf("run %d names specs the stored baseline cannot hold:\n%s", i+1, out)
		}
	}
}

func TestSyncSummary_NamesRuleMovedToUnconfiguredTool(t *testing.T) {
	dir := builtinClaudeProject(t, true)
	rule := filepath.Join(dir, ".agnostic-ai", "rules", "style.md")
	writeFile(t, rule, "---\nname: style\n---\nBe brief.\n")
	syncOK(t)
	writeFile(t, rule, "---\nname: style\ntargets: [gemini]\n---\nBe brief.\n")
	buf := captureLogOut(t)
	syncOK(t, "-vv")
	if out := buf.String(); !strings.Contains(out, "~ rule style") {
		t.Errorf("summary misses the retargeted rule:\n%s", out)
	}
}

func TestSyncSummary_AddedTargetNamesConfigNotEverySpec(t *testing.T) {
	dir := builtinClaudeProject(t, true)
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\n---\nBe brief.\n")
	syncOK(t)
	writeFile(t, filepath.Join(dir, config.ConfigFileName), "version: 1\ntargets: [claude, cursor]\nbuiltins: [memory]\n")
	buf := captureLogOut(t)
	syncOK(t, "-vv")
	out := buf.String()
	if !strings.Contains(out, "~ config") {
		t.Errorf("summary misses the config change:\n%s", out)
	}
	for _, spec := range []string{"~ rule style", "~ rule shared-memory-policy", "~ skill shared-memory"} {
		if strings.Contains(out, spec) {
			t.Errorf("summary lists %q for an added target:\n%s", spec, out)
		}
	}
}

func TestMaterializedBuiltinPath(t *testing.T) {
	hash := strings.Repeat("a1", 32)
	for key, want := range map[string]bool{
		"/tmp/agnostic-ai-builtin-memory-123456/hooks/x.yaml":                   true,
		"/var/folders/T/agnostic-ai-builtin-handoff-hook-9/skills/a/SKILL.md":   true,
		"C:/Users/me/AppData/Local/Temp/agnostic-ai-builtin-rtk-77/rules/a.md":  true,
		"/Users/me/Library/Caches/agnostic-ai/builtins/" + hash + "/rules/a.md": true,
		".agnostic-ai/skills/agnostic-ai-builtin-notes/SKILL.md":                false,
		"/work/proj/.agnostic-ai/skills/agnostic-ai-builtin-notes/SKILL.md":     false,
		"/work/agnostic-ai/builtins/notes/a.md":                                 false,
		"/tmp/agnostic-ai-builtin-memory-12x/hooks/x.yaml":                      false,
	} {
		if got := materializedBuiltinPath(key); got != want {
			t.Errorf("materializedBuiltinPath(%q) = %v, want %v", key, got, want)
		}
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
