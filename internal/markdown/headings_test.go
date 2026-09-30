package markdown

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer/html"
)

func TestNestHeadings_PreservesRenderedContent(t *testing.T) {
	for _, body := range []string{
		"[foo]: /url\nbar\n===\n\nUse [foo].\n",
		"[foo]:\n /url\n \"title\"\nbar\n===\n\nUse [foo].\n",
		"[foo]: /url\n===\n\nUse [foo].\n",
		"Title  \nSecond\n---\n",
		"Title\\\nSecond\n---\n",
		"`Title  \nSecond`\n---\n",
		"`Title  \n  Second`\n---\n",
		"Title #\n---\n",
		"#hashtag\n---\n",
		"Title\n    continued\n---\n",
		"Title  \n---\n",
		"- ```markdown\n  # Literal\n  ```\n\n### Outside\n",
		"> ```markdown\n> # Literal\n> ```\n\n### Outside\n",
		"###\n\n#\n",
		"Title  \r\nSecond\r\n---\r\n",
	} {
		t.Run(body, func(t *testing.T) {
			got, _ := NestHeadings(body, 3)
			if before, after := renderedContent(t, body), renderedContent(t, got); before != after {
				t.Errorf("heading rewrite changed rendered content:\nsource: %q\nemitted: %q\nbefore: %s\nafter: %s", body, got, before, after)
			}
		})
	}
}

func renderedContent(t *testing.T, body string) string {
	t.Helper()
	var buf bytes.Buffer
	md := goldmark.New(goldmark.WithRendererOptions(html.WithUnsafe()))
	if err := md.Convert([]byte(body), &buf); err != nil {
		t.Fatal(err)
	}
	content := regexp.MustCompile(`<(\/?)h[1-6]>`).ReplaceAllString(buf.String(), "<${1}h>")
	content = strings.ReplaceAll(content, "<br>\n", "<br>")
	return strings.ReplaceAll(content, "\n", " ")
}

func TestLeavesBlockOpen(t *testing.T) {
	for _, tc := range []struct {
		body string
		want bool
	}{
		{"", false},
		{"```sh\necho open\n\n", true},
		{"```\n", true},
		{"```sh\necho closed\n```\n\n", false},
		{"- ```\n  code\n\n", false},
		{"<!-- draft\n\n", true},
		{"<!-- draft -->\n\n", false},
		{"<!--\ndraft\n-->\n\n", false},
		{"<?php\n\n", true},
		{"<?php ?>\n\n", false},
		{"<script>\n\n", true},
		{"<script></script>\n\n", false},
		{"<![CDATA[\n\n", true},
		{"<![CDATA[text]]>\n\n", false},
		{"<div>\ntext\n\n", false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			if got := LeavesBlockOpen(tc.body); got != tc.want {
				t.Errorf("LeavesBlockOpen(%q) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}
