package zed

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

func TestEmit_CapabilityMatrixCoversEveryDeclaredKind(t *testing.T) {
	dir := testutil.TempCwd(t)
	// Rules and agents are only observable in adapter output through the
	// legacy merged document; by default sync delivers rules via the
	// shared AGENTS.md entry-point, which this per-adapter test cannot see.
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"zed": {TasksFile: ".zed/tasks.json", RulesFile: ".rules"},
		},
	}
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	paths := testutil.WalkRel(t, dir)
	rulesBody, _ := os.ReadFile(filepath.Join(dir, ".rules"))
	body := string(rulesBody)

	type expect struct {
		kind     spec.Kind
		matchers []string
		inBody   []string
	}
	cases := []expect{
		{spec.KindRule, []string{".rules"}, nil},
		{spec.KindAgent, nil, []string{"### Agent: alpha", "### Agent: beta", "### Agent: gamma"}},
		{spec.KindSkill, []string{".agents/skills/uno/SKILL.md"}, nil},
		{spec.KindMCP, []string{".zed/settings.json"}, nil},
		{spec.KindHook, []string{".zed/tasks.json"}, nil},
	}
	for _, k := range caps.Supports {
		found := false
		for _, c := range cases {
			if c.kind != k {
				continue
			}
			for _, m := range c.matchers {
				if pathSetContains(paths, m) {
					found = true
					break
				}
			}
			for _, s := range c.inBody {
				if strings.Contains(body, s) {
					found = true
					break
				}
			}
		}
		if !found {
			t.Errorf("declared kind %q in caps.Supports has no observable output (paths: %v)", k, paths)
		}
	}
}

func TestEmit_NoCapabilityWarningsForKitSinkBundle(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"zed": {TasksFile: ".zed/tasks.json"},
		},
	}
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if got := emit.PendingCapabilityWarningsCount(); got != 0 {
		t.Errorf("expected no capability warnings, got %d", got)
	}
}

func TestEmit_UnsupportedKindsWarn(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)

	entries := []spec.Entry{
		{Kind: spec.KindCommand, Name: "cmd-one", Path: "commands/cmd-one.md", Body: "cmd body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{OnUnsupported: "warn"}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if got := emit.PendingCapabilityWarningsCount(); got != 1 {
		t.Errorf("expected 1 capability warning (command), got %d", got)
	}
}

func pathSetContains(paths []string, needle string) bool {
	needle = filepath.ToSlash(needle)
	for _, p := range paths {
		if strings.Contains(p, needle) {
			return true
		}
	}
	return false
}
