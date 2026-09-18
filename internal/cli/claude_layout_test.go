package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func movedClaudeConfig(dir string) *config.Config {
	return &config.Config{Outputs: map[string]config.Output{"claude": {Dir: dir}}}
}

// With no overrides the layout is the default tree, so every existing
// caller keeps reading exactly where it always did.
func TestClaudeLayoutFor_DefaultsUnchanged(t *testing.T) {
	if got := claudeLayoutFor(&config.Config{}); got != defaultClaudeLayout() {
		t.Errorf("claudeLayoutFor(empty) = %+v, want %+v", got, defaultClaudeLayout())
	}
	if got := claudeLayoutFor(nil); got != defaultClaudeLayout() {
		t.Errorf("claudeLayoutFor(nil) = %+v, want %+v", got, defaultClaudeLayout())
	}
}

// `outputs.claude.dir` moves the whole tool directory, and the layout
// has to follow it or import reads an empty default path (#852).
func TestClaudeLayoutFor_FollowsTheDirOverride(t *testing.T) {
	got := claudeLayoutFor(movedClaudeConfig("vendor/.claude"))
	want := claudeLayout{
		dir:      filepath.FromSlash("vendor/.claude"),
		rules:    filepath.FromSlash("vendor/.claude/rules"),
		commands: filepath.FromSlash("vendor/.claude/commands"),
		agents:   filepath.FromSlash("vendor/.claude/agents"),
		skills:   filepath.FromSlash("vendor/.claude/skills"),
	}
	if got != want {
		t.Errorf("claudeLayoutFor(moved) = %+v, want %+v", got, want)
	}
}

// A per-kind key overrides that path on its own, and the layout reads
// it from the adapter rather than re-deriving the rule here.
func TestClaudeLayoutFor_FollowsPerKindOverrides(t *testing.T) {
	cfg := &config.Config{Outputs: map[string]config.Output{"claude": {
		Dir: "vendor/.claude", CommandsDir: "tools/cmds", AgentsDir: "tools/agents",
	}}}
	got := claudeLayoutFor(cfg)
	if got.commands != filepath.FromSlash("tools/cmds") {
		t.Errorf("commands = %q, want tools/cmds", got.commands)
	}
	if got.agents != filepath.FromSlash("tools/agents") {
		t.Errorf("agents = %q, want tools/agents", got.agents)
	}
	if got.rules != filepath.FromSlash("vendor/.claude/rules") {
		t.Errorf("rules = %q, want it under the moved dir", got.rules)
	}
}

// The acceptance scenario: specs written under a moved directory come
// back through import.
func TestImportFromClaude_ReadsAMovedDirectory(t *testing.T) {
	root := t.TempDir()
	moved := filepath.Join("vendor", ".claude")
	writeFile(t, filepath.Join(root, moved, "rules", "commits.md"), "Use Conventional Commits.\n")
	writeFile(t, filepath.Join(root, moved, "commands", "deploy.md"), "---\ndescription: Ship\n---\nShip it.\n")
	writeFile(t, filepath.Join(root, moved, "agents", "explorer.md"), "---\nname: explorer\n---\nExplore.\n")
	writeFile(t, filepath.Join(root, moved, "skills", "review", "SKILL.md"), "---\nname: review\ndescription: Review\n---\nReview.\n")

	if err := importFromClaude(root, rootSources(), claudeLayoutFor(movedClaudeConfig("vendor/.claude"))); err != nil {
		t.Fatalf("import: %v", err)
	}

	for _, rel := range []string{
		filepath.Join("rules", "commits.md"),
		filepath.Join("commands", "deploy.md"),
		filepath.Join("agents", "explorer.md"),
		filepath.Join("skills", "review", "SKILL.md"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Errorf("import did not reconstruct %s from the moved directory: %v", rel, err)
		}
	}
}

// doctor has to report an unmanaged file under the moved commands
// directory, not just under the default one.
func TestFindUnmanagedConfig_FollowsTheMovedClaudeDir(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "vendor", ".claude", "commands", "hand.md"), "hand written\n")

	got, err := findUnmanagedConfig(root, movedClaudeConfig("vendor/.claude"))
	if err != nil {
		t.Fatalf("findUnmanagedConfig: %v", err)
	}
	var found bool
	for _, f := range got {
		if f.Path == "vendor/.claude/commands/hand.md" && f.Target == "claude" {
			found = true
		}
	}
	if !found {
		t.Errorf("unmanaged file under the moved dir went unreported; got %+v", got)
	}
}
