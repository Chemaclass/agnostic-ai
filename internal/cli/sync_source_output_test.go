package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const sourceNote = "# My project\n\nHand-written note.\n"

func sourceOutputProject(t *testing.T, outputs string) string {
	t.Helper()
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\nsources:\n  skills: specs/skills\noutputs:\n  codex:\n"+outputs)
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), sourceNote)
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\n---\nUse tabs.\n")
	writeFile(t, filepath.Join(dir, "specs", "skills", "review", "SKILL.md"), "---\nname: review\ndescription: Review.\n---\nReview.\n")
	return dir
}

// An output path that names the source note overwrote it with no backup
// on a case-insensitive disk (#3740).
func TestSync_RefusesAnOutputPathInTheSourceFolder(t *testing.T) {
	for _, tc := range []struct {
		name, outputs, key string
		caseOnly           bool
		link               string
	}{
		{name: "other letter case", outputs: "    file: .agnostic-ai/agnostic_ai.md\n", key: "outputs.codex.file", caseOnly: true},
		{name: "dot-dot segment", outputs: "    file: .agnostic-ai/rules/../AGNOSTIC_AI.md\n", key: "outputs.codex.file"},
		{name: "configured spec folder", outputs: "    file: specs/skills/AGENTS.md\n", key: "outputs.codex.file"},
		{name: "folder setting", outputs: "    skills-dir: .agnostic-ai/skills\n", key: "outputs.codex.skills-dir"},
		{name: "linked folder", outputs: "    skills-dir: lnk/skills\n", key: "outputs.codex.skills-dir", link: ".agnostic-ai"},
		{name: "dot-dot after a link", outputs: "    file: lnk/../AGNOSTIC_AI.md\n", key: "outputs.codex.file", link: filepath.Join(".agnostic-ai", "rules")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := sourceOutputProject(t, tc.outputs)
			if tc.caseOnly && !caseInsensitiveFS(".") {
				t.Skip("letter case names distinct files here")
			}
			if tc.link != "" {
				if err := os.Symlink(tc.link, "lnk"); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			}

			out, err := runCLI(t, "sync", "--gitignore=off")

			if err == nil {
				t.Fatalf("sync wrote an output into the source folder:\n%s", out)
			}
			if !strings.Contains(err.Error(), tc.key) {
				t.Errorf("error does not name %s: %v", tc.key, err)
			}
			if got := readFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md")); got != sourceNote {
				t.Errorf("source note = %q, want it kept byte for byte", got)
			}

			out, err = runCLI(t, "validate")
			if err == nil || !strings.Contains(out+err.Error(), tc.key) {
				t.Errorf("validate does not report %s: %v\n%s", tc.key, err, out)
			}
		})
	}
}
