package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A kind whose adapter names no folder still shows where its specs land.
func TestSync_FirstRunNamesWhereEachKindLands(t *testing.T) {
	for tool, want := range map[string]string{
		"codex":  "AGENTS.md, services/api/AGENTS.md",
		"cursor": ".cursor/rules/",
		"cline":  ".clinerules/",
		"kiro":   ".kiro/steering/",
	} {
		t.Run(tool, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			isolateGit(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+tool+"]\n")
			mustWriteFile(t, ".agnostic-ai/rules/tone.md", "---\nname: tone\ndescription: Tone.\nalwaysApply: true\n---\nBe kind.\n")
			mustWriteFile(t, ".agnostic-ai/rules/api.md", "---\nname: api\ndescription: API.\nscope: services/api\n---\nReturn typed errors.\n")
			mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", "---\nname: review\ndescription: Review a PR.\n---\nReview.\n")
			log := captureLog(t)

			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			for _, line := range strings.Split(log.String(), "\n") {
				if !strings.HasPrefix(line, "    ") || len(line) < 21 || strings.HasPrefix(line, "    instructions") {
					continue
				}
				rest := strings.TrimSpace(line[21:])
				if strings.Contains(line, " rules ") && strings.TrimSpace(strings.TrimSuffix(rest, "api, tone")) != want {
					t.Errorf("rules land in %q, want %q:\n%s", rest, want, log.String())
				}
				if rest == "review" || rest == "api, tone" {
					t.Errorf("a kind lists no location: %q\n%s", line, log.String())
				}
			}
		})
	}
}
