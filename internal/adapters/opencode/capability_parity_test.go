package opencode

import (
	"io/fs"
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
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"opencode": {
				RulesFile:            "AGENTS-rules.md",
				EmitSkillsAsCommands: true,
			},
		},
	}
	if err := New().Emit(kitSinkBundle(), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	paths := walkRel(t, dir)
	type expect struct {
		kind     spec.Kind
		matchers []string
	}
	cases := []expect{
		{spec.KindAgent, []string{".opencode/commands/alpha.md", ".opencode/commands/beta.md", ".opencode/commands/gamma.md"}},
		{spec.KindSkill, []string{".opencode/commands/skill-uno.md", ".opencode/commands/skill-dos.md", ".opencode/commands/skill-tres.md"}},
		{spec.KindRule, []string{"AGENTS-rules.md"}},
		{spec.KindMCP, []string{"opencode.json"}},
		{spec.KindCommand, []string{".opencode/commands/deploy.md"}},
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
			"opencode": {
				RulesFile:            "AGENTS-rules.md",
				EmitSkillsAsCommands: true,
			},
		},
	}
	if err := New().Emit(kitSinkBundle(), cfg, false); err != nil {
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
		{Kind: spec.KindHook, Name: "fmt-go", Meta: map[string]any{"event": "PostToolUse", "command": "gofmt -w"}},
		{Kind: spec.KindSettings, Name: "defaults", Meta: map[string]any{"model": "x"}},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{OnUnsupported: "warn"}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if got := emit.PendingCapabilityWarningsCount(); got != 2 {
		t.Errorf("expected 2 capability warnings (hook/settings), got %d", got)
	}
}

func walkRel(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
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
