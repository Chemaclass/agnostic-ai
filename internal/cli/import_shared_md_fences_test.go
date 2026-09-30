package cli

import (
	"strings"
	"testing"
)

func TestImport_H3ChildrenKeepFencedHeadingsInTheirBody(t *testing.T) {
	for _, code := range []string{
		"~~~markdown\n### Code heading\n~~~",
		"```markdown\n### Code heading\n```",
		"````markdown\n### Code heading\n```\n### Still fenced\n````",
		"~~~markdown\n### Code heading\n~~~not-a-close\n### Still fenced\n~~~",
	} {
		t.Run(code, func(t *testing.T) {
			body := "### content\n\n" + code + "\n\n### following\n\nOutside code.\n"
			merged, ok := unwrapMergedH3Children(body, map[string]int{})
			if !ok || len(merged) != 2 {
				t.Errorf("merged children = %+v, want content and following", merged)
			} else if merged[0].slug != "content" || merged[1].slug != "following" || !strings.Contains(merged[0].body, code) {
				t.Errorf("merged split changed fenced body: %+v", merged)
			}
			codex := unwrapH3(body)
			if len(codex) != 2 {
				t.Errorf("Codex children = %+v, want content and following", codex)
			} else if codex[0].slug != "content" || codex[1].slug != "following" || !strings.Contains(codex[0].body, code) {
				t.Errorf("Codex split changed fenced body: %+v", codex)
			}
		})
	}
}
