package amp

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

// TestEmit_CapabilityMatrixCoversEveryDeclaredKind enforces the
// invariant that the amp adapter actually emits something for every
// spec kind it declares in caps.Supports. A future refactor that
// drops support for, say, KindMCP would either need to remove the
// kind from Supports (forcing the warning channel) or fix the emit
// path.
func TestEmit_CapabilityMatrixCoversEveryDeclaredKind(t *testing.T) {
	dir := testutil.TempCwd(t)
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"amp": {
				RulesFile:            "AGENTS.md",
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
		{spec.KindAgent, []string{".agents/commands/alpha.md", ".agents/commands/beta.md", ".agents/commands/gamma.md"}},
		{spec.KindSkill, []string{".agents/commands/skill-uno.md", ".agents/commands/skill-dos.md", ".agents/commands/skill-tres.md"}},
		{spec.KindRule, []string{"AGENTS.md"}},
		{spec.KindMCP, []string{".amp/settings.json"}},
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

// TestEmit_NoCapabilityWarningsForKitSinkBundle asserts that emitting
// every declared kind does not buffer any "unsupported" warning. If
// the adapter starts silently dropping a kind without removing it
// from Supports, the warning channel would stay empty (wrong) AND
// ReportUnsupported would too; this test catches the regression by
// checking the warning buffer.
func TestEmit_NoCapabilityWarningsForKitSinkBundle(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"amp": {
				RulesFile:            "AGENTS.md",
				EmitSkillsAsCommands: true,
			},
		},
	}
	if err := New().Emit(kitSinkBundle(), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if got := emit.PendingCapabilityWarningsCount(); got != 0 {
		t.Errorf("expected no capability warnings for a kit-sink bundle, got %d", got)
	}
}

// TestEmit_UnsupportedKindsWarn asserts ReportUnsupported fires for
// every kind amp does not declare in caps.Supports (Hook, Command).
// A future caps.Supports expansion needs to delete the matching row
// here and demonstrate the emit path that backs the new claim.
func TestEmit_UnsupportedKindsWarn(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "fmt-go", Meta: map[string]any{"event": "PostToolUse", "command": "gofmt -w"}},
		{Kind: spec.KindCommand, Name: "cmd-one", Path: "commands/cmd-one.md", Body: "cmd body"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{OnUnsupported: "warn"}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if got := emit.PendingCapabilityWarningsCount(); got != 2 {
		t.Errorf("expected 2 capability warnings (hook/command), got %d", got)
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
