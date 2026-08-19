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
