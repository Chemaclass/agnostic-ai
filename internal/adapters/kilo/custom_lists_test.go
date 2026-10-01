package kilo

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

// An x-kilo list merges into the kept instructions and the skills paths
// sync writes, so neither the user's entries nor sync's go missing.
func TestEmit_XKiloListsMergeWithKeptAndGeneratedEntries(t *testing.T) {
	for _, tc := range []struct {
		name     string
		existing string
		custom   map[string]any
		cfg      *config.Config
		entries  []spec.Entry
		inlined  bool
		want     []string
	}{
		{
			name:     "instructions",
			existing: `{"instructions": [".kilo/rules/r1.md", "docs/team.md"]}`,
			custom:   map[string]any{"instructions": []any{"docs/custom.md"}},
			cfg:      &config.Config{},
			inlined:  true,
			want:     []string{`"docs/team.md"`, `"docs/custom.md"`},
		},
		{
			name:    "skills paths",
			custom:  map[string]any{"skills": map[string]any{"paths": []any{"custom-x"}}},
			cfg:     &config.Config{Outputs: map[string]config.Output{"kilo": {SkillsDir: "docs/team-skills"}}},
			entries: []spec.Entry{{Kind: spec.KindSkill, Name: "demo", Body: "skill body"}},
			want:    []string{`"custom-x"`, `"docs/team-skills"`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			if tc.existing != "" {
				if err := os.WriteFile(filepath.Join(dir, "kilo.jsonc"), []byte(tc.existing), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			sess := emit.NewSession()
			if tc.inlined {
				sess.SetInlinedRules([]spec.Entry{{Kind: spec.KindRule, Name: "r1"}})
			}
			entries := append(tc.entries, spec.Entry{Kind: spec.KindSettings, Name: "s", Meta: map[string]any{"x-kilo": tc.custom}})
			if err := New().Emit(sess, spec.NewBundle(entries), tc.cfg, false); err != nil {
				t.Fatal(err)
			}
			got := readFile(t, filepath.Join(dir, "kilo.jsonc"))
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("missing %s:\n%s", want, got)
				}
			}
		})
	}
}
