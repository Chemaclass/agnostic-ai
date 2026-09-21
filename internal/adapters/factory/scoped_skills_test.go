package factory

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

func TestEmit_PreservesSkillScopes(t *testing.T) {
	testutil.TempCwd(t)
	entries := []spec.Entry{}
	for _, scope := range []string{"", "backend", "frontend"} {
		entries = append(entries, spec.Entry{Kind: spec.KindSkill, Name: "review", Scope: scope, Body: "Review " + scope})
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"", "backend", "frontend"} {
		dir := ".agents/skills"
		if scope != "" {
			dir = filepath.Join(scope, ".factory/skills")
		}
		raw, err := os.ReadFile(filepath.Join(dir, "review/SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), strings.TrimSpace("Review "+scope)) {
			t.Errorf("wrong skill: %s", raw)
		}
	}
}

func TestEmit_ScopedSkillOverrideKeepsAssetsAndUnmanagedFiles(t *testing.T) {
	testutil.TempCwd(t)
	source := ".agnostic-ai/skills/backend/review"
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "guide.md"), []byte("asset"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := "backend/custom/skills/review"
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dest, "SKILL.md")
	if err := os.WriteFile(path, []byte("manual"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess := emit.NewSession()
	sess.SetUnmanaged([]string{filepath.ToSlash(path)})
	entry := spec.Entry{Kind: spec.KindSkill, Name: "review", Scope: "backend", Path: filepath.Join(source, "SKILL.md"), Body: "Generated"}
	cfg := &config.Config{Outputs: map[string]config.Output{"factory": {SkillsDir: "custom/skills"}}}
	if err := New().Emit(sess, spec.NewBundle([]spec.Entry{entry}), cfg, false); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "manual" {
		t.Errorf("manual file changed: %s", got)
	}
	if got := readFile(t, filepath.Join(dest, "guide.md")); got != "asset" {
		t.Errorf("asset changed: %s", got)
	}
}
