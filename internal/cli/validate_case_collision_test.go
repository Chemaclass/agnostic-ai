package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Outputs that differ only in letter case validated clean, then sync
// failed with AAI-102 where case does not count (#3733).
func TestValidate_ReportsOutputsThatDifferOnlyInCase(t *testing.T) {
	for _, tc := range []struct {
		name  string
		yaml  string
		files map[string]string
		paths []string
	}{
		{
			name: "rule names",
			yaml: "version: 1\ntargets: [claude]\n",
			files: map[string]string{
				"rules/b.md":  "---\nname: b\n---\nLower.\n",
				"rules/B2.md": "---\nname: B\n---\nUpper.\n",
			},
			paths: []string{".claude/rules/B.md", ".claude/rules/b.md"},
		},
		{
			name: "rule scopes",
			yaml: "version: 1\ntargets: [codex]\n",
			files: map[string]string{
				"rules/one.md": "---\nname: one\nscope: src\n---\nOne.\n",
				"rules/two.md": "---\nname: two\nscope: Src\n---\nTwo.\n",
			},
			paths: []string{"Src/AGENTS.md", "src/AGENTS.md"},
		},
		{
			name: "configured skills folder",
			yaml: "version: 1\ntargets: [claude, codex]\noutputs:\n  claude:\n    skills-dir: .Agents/skills\n",
			files: map[string]string{
				"skills/x/SKILL.md": "---\nname: x\ndescription: X.\n---\nX.\n",
			},
			paths: []string{".Agents/skills/x/SKILL.md", ".agents/skills/x/SKILL.md"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			if !caseInsensitiveFS(".") {
				t.Skip("letter case names distinct files here")
			}
			writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), tc.yaml)
			writeFile(t, filepath.Join(dir, "src", ".keep"), "")
			for path, body := range tc.files {
				writeFile(t, filepath.Join(dir, ".agnostic-ai", filepath.FromSlash(path)), body)
			}

			out, err := runCLI(t, "validate")

			if err == nil {
				t.Fatalf("validate passed a case-only output collision:\n%s", out)
			}
			for _, want := range append([]string{"AAI-102"}, tc.paths...) {
				if !strings.Contains(out+err.Error(), want) {
					t.Errorf("validate output does not name %q:\n%s\n%v", want, out, err)
				}
			}
		})
	}
}

func TestValidate_PassesRulesWithDistinctOutputs(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "a.md"), "---\nname: a\n---\nOne.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "b.md"), "---\nname: b\n---\nTwo.\n")

	if out, err := runCLI(t, "validate"); err != nil {
		t.Fatalf("validate: %v\n%s", err, out)
	}
}
