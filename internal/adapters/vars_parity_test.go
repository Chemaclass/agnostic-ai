package adapters

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// A declared variable must name the place the adapter actually writes
// that kind. Without this, targetVarPaths is a second copy of every
// adapter's path constants and drifts the first time one moves.
func TestTargetVarPaths_MatchRealEmission(t *testing.T) {
	probes := map[string]spec.Entry{
		emit.VarSkillsDir:   {Kind: spec.KindSkill, Name: "probe", Path: "skills/probe/SKILL.md", Body: "b"},
		emit.VarAgentsDir:   {Kind: spec.KindAgent, Name: "probe", Path: "agents/probe.md", Body: "b"},
		emit.VarCommandsDir: {Kind: spec.KindCommand, Name: "probe", Path: "commands/probe.md", Body: "b"},
		emit.VarRulesDir:    {Kind: spec.KindRule, Name: "probe", Path: "rules/probe.md", Body: "b"},
		emit.VarMCPFile:     {Kind: spec.KindMCP, Name: "probe", Meta: map[string]any{"command": "x"}},
	}

	for target, declared := range targetVarPaths {
		adapter, err := Resolve(target)
		if err != nil {
			t.Errorf("%s: declared in targetVarPaths but not a registered target", target)
			continue
		}
		for name, want := range declared {
			sess := NewSession()
			sess.StartCapture()
			bundle := spec.NewBundle([]spec.Entry{probes[name]})
			if err := EmitWithProvenance(sess, adapter, bundle, &config.Config{}, true); err != nil {
				sess.StopCapture()
				t.Errorf("%s/%s: emit failed: %v", target, name, err)
				continue
			}
			var paths []string
			for _, f := range sess.StopCapture() {
				paths = append(paths, f.Path)
			}
			if !writesUnder(paths, name, want) {
				t.Errorf("%s declares %s=%q but emits that kind to %v", target, name, want, paths)
			}
		}
	}
}

// writesUnder reports whether want is where the kind actually landed:
// the containing directory for a *_DIR variable, the file itself for
// MCP_FILE.
func writesUnder(paths []string, name, want string) bool {
	for _, p := range paths {
		if name == emit.VarMCPFile {
			if p == want {
				return true
			}
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(p))
		// A skill folder is <dir>/<name>/SKILL.md, one level deeper.
		if dir == want || strings.HasPrefix(dir+"/", want+"/") {
			return true
		}
	}
	return false
}

// A target that honors `outputs.<target>.dir` must resolve its declared
// variables under the configured dir, or a spec body names a directory
// the sync no longer writes to (#849).
func TestTargetVarPaths_FollowDirOverride(t *testing.T) {
	const moved = "vendor/.claude"
	cfg := &config.Config{Outputs: map[string]config.Output{"claude": {Dir: moved}}}

	vars := varsFor(cfg, "claude")
	if len(vars) == 0 {
		t.Fatal("claude resolved no variables")
	}
	for name, got := range vars {
		if name == emit.VarMCPFile {
			continue
		}
		if !strings.HasPrefix(got, moved+"/") {
			t.Errorf("%s = %q, want it under %q", name, got, moved)
		}
	}

	probes := map[string]spec.Entry{
		emit.VarSkillsDir:   {Kind: spec.KindSkill, Name: "probe", Path: "skills/probe/SKILL.md", Body: "b"},
		emit.VarAgentsDir:   {Kind: spec.KindAgent, Name: "probe", Path: "agents/probe.md", Body: "b"},
		emit.VarCommandsDir: {Kind: spec.KindCommand, Name: "probe", Path: "commands/probe.md", Body: "b"},
		emit.VarRulesDir:    {Kind: spec.KindRule, Name: "probe", Path: "rules/probe.md", Body: "b"},
	}
	adapter, err := Resolve("claude")
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range vars {
		probe, ok := probes[name]
		if !ok {
			continue
		}
		sess := NewSession()
		sess.StartCapture()
		if err := EmitWithProvenance(sess, adapter, spec.NewBundle([]spec.Entry{probe}), cfg, true); err != nil {
			sess.StopCapture()
			t.Fatalf("%s: emit failed: %v", name, err)
		}
		var paths []string
		for _, f := range sess.StopCapture() {
			paths = append(paths, f.Path)
		}
		if !writesUnder(paths, name, want) {
			t.Errorf("claude resolves %s=%q under the dir override but emits that kind to %v", name, want, paths)
		}
	}
}

// Every registered target needs an entry, even an empty one, so adding
// an adapter forces a decision about its variables instead of silently
// leaving every variable unresolved there.
func TestTargetVarPaths_CoverEveryRegisteredTarget(t *testing.T) {
	for _, target := range Names() {
		if _, ok := targetVarPaths[target]; !ok {
			t.Errorf("%s has no targetVarPaths entry; add one (empty is fine when the target has no per-kind dirs)", target)
		}
	}
}
