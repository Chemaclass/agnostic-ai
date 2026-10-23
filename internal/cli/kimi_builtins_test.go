package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func TestSyncKimi_NativeProjectWithMemoryBuiltinsPassesChecks(t *testing.T) {
	dir := builtinProject(t, "handoff, caveman, memory")
	writeFile(t, filepath.Join(dir, config.ConfigFileName), "version: 1\ntargets: [kimi]\nbuiltins: [handoff, caveman, memory]\n")
	nativeAgent := ".kimi-code/agents/reviewer.md"
	writeFile(t, nativeAgent, "---\nname: reviewer\ndescription: Review changes\ntools: []\n---\nReview changes.\n")
	for _, args := range [][]string{{"import", "kimi"}, {"validate"}, {"lint", "--strict"}, {"sync", "-t", "kimi"}, {"sync", "--check", "-t", "kimi"}, {"lint", "--strict"}} {
		if out, err := runBuiltinCLI(t, args...); err != nil {
			t.Fatalf("%v with Kimi and memory builtins: %v\n%s", args, err, out)
		}
	}
	if got := readFile(t, nativeAgent); !strings.Contains(got, "tools: []") || !strings.Contains(got, "Review changes.") {
		t.Errorf("native agent restrictions changed: %s", got)
	}
	if got := readFile(t, ".kimi-code/skills/shared-memory/SKILL.md"); !strings.Contains(got, "name: shared-memory") {
		t.Errorf("builtin memory skill missing: %s", got)
	}
	if got := readFile(t, ".kimi-code/AGENTS.md"); !strings.Contains(got, "shared-memory-policy") {
		t.Errorf("builtin memory rule missing: %s", got)
	}
}
