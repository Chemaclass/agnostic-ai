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
		"~~~markdown\n<div>\n### Code heading\n~~~",
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

func TestImport_H3ChildrenKeepHTMLHeadingsInTheirBody(t *testing.T) {
	for _, tc := range []struct{ name, html, separator string }{
		{"tag block", "<div>\n### Literal\n</div>\n", "\n"},
		{"indented opening", "   <div>\n### Literal\n</div>\n", "\n"},
		{"tag closing does not end block", "<div>\n### Literal\n</div>\n### Still literal\n", "\n"},
		{"indented blank ends block", "<div>\n### Literal\n</div>\n    \n", ""},
		{"tab blank ends block", "<div>\n### Literal\n</div>\n\t\n", ""},
		{"comment indented closing", "<!--\n\n### Literal\n    -->\n", ""},
		{"script indented closing", "<script>\n\n### Literal\n    </script>\n", ""},
		{"processing instruction", "<?example\n### Literal\n?>\n", ""},
		{"CDATA", "<![CDATA[\n### Literal\n]]>\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := "### content\n\n" + tc.html + tc.separator + "### following\n\nOutside HTML.\n"
			merged, ok := unwrapMergedH3Children(body, map[string]int{})
			if !ok || len(merged) != 2 {
				t.Errorf("merged children = %+v, want content and following", merged)
			} else if merged[0].slug != "content" || merged[1].slug != "following" || !strings.Contains(merged[0].body, strings.TrimSpace(tc.html)) {
				t.Errorf("merged split changed HTML body: %+v", merged)
			}
			codex := unwrapH3(body)
			if len(codex) != 2 {
				t.Errorf("Codex children = %+v, want content and following", codex)
			} else if codex[0].slug != "content" || codex[1].slug != "following" || !strings.Contains(codex[0].body, strings.TrimSpace(tc.html)) {
				t.Errorf("Codex split changed HTML body: %+v", codex)
			}
		})
	}
}

func TestImport_H3AfterIndentedCodeIsNotHTML(t *testing.T) {
	body := "### content\n\n    <div>\n### following\n\nOutside indented code.\n"
	if children := unwrapH3(body); len(children) != 2 || children[1].slug != "following" {
		t.Errorf("indented code swallowed a heading: %+v", children)
	}
	if children, ok := unwrapMergedH3Children(body, map[string]int{}); !ok || len(children) != 2 || children[1].slug != "following" {
		t.Errorf("indented code swallowed a merged heading: %+v", children)
	}
}
