package mdlink

import (
	"strings"
	"testing"
)

func TestLocal_FindsInlineAndReferenceLinks(t *testing.T) {
	doc := strings.Join([]string{
		"---",
		"name: deploy",
		"description: see [x](frontmatter.md)",
		"---",
		"Read [setup](references/setup.md) first.",
		"Then ![diagram](img/flow.png \"Flow\") and [spaced](<my notes.md>).",
		"Encoded [enc](my%20file.md) and [frag](guide.md#install).",
		"[label]: refs/label.md \"Title\"",
		"  [angle]: <refs/angle file.md>",
		"[^note]: footnote text",
	}, "\n")

	got := Local(doc)
	want := []Link{
		{Line: 5, Dest: "references/setup.md"},
		{Line: 6, Dest: "img/flow.png"},
		{Line: 6, Dest: "my notes.md"},
		{Line: 7, Dest: "my file.md"},
		{Line: 7, Dest: "guide.md"},
		{Line: 8, Dest: "refs/label.md"},
		{Line: 9, Dest: "refs/angle file.md"},
	}
	assertLinks(t, got, want)
}

func TestLocal_IgnoresCodeAndNonLocalDestinations(t *testing.T) {
	doc := strings.Join([]string{
		"Inline `[code](inline.md)` span and ``[double](double.md)``.",
		"```md",
		"[fenced](fenced.md)",
		"```",
		"~~~~",
		"[tilde](tilde.md)",
		"```",
		"[still fenced](tilde2.md)",
		"~~~~",
		"",
		"    [indented](indented.md)",
		"",
		"[web](https://example.com/x.md) [mail](mailto:a@b.c)",
		"[proto](//cdn.example.com/x.md) [abs](/etc/passwd)",
		"[frag](#section) [empty]() [query](?x=1)",
		"[ref]: https://example.com/ref.md",
		"[kept](kept.md)",
	}, "\n")

	assertLinks(t, Local(doc), []Link{{Line: 17, Dest: "kept.md"}})
}

func TestLocal_ChecksIndentedLinesInsideLists(t *testing.T) {
	doc := "- step one\n\n    see [nested](nested.md)\n"

	assertLinks(t, Local(doc), []Link{{Line: 3, Dest: "nested.md"}})
}

func TestLocal_KeepsParenthesesBalancedInDestination(t *testing.T) {
	doc := "See [v](docs/guide(v2).md) now.\n"

	assertLinks(t, Local(doc), []Link{{Line: 1, Dest: "docs/guide(v2).md"}})
}

func TestRewriteLocal_ReplacesPathsAndKeepsEverythingElse(t *testing.T) {
	doc := strings.Join([]string{
		"---",
		"description: see [x](references/a.md)",
		"---",
		"Read [a](references/a.md#install) and `[code](references/a.md)`.",
		"Then [b]( <references/b file.md> ) and [web](https://example.com/references/a.md).",
		"```",
		"[fenced](references/a.md)",
		"```",
		"  [ref]: references/c.md \"Title\"",
		"[other](other.md)",
	}, "\r\n")

	got := RewriteLocal(doc, func(l Link) (string, bool) {
		if !strings.HasPrefix(l.Dest, "references/") {
			return "", false
		}
		return "../skills/deploy/" + l.Raw, true
	})

	want := strings.Join([]string{
		"---",
		"description: see [x](references/a.md)",
		"---",
		"Read [a](../skills/deploy/references/a.md#install) and `[code](references/a.md)`.",
		"Then [b]( <../skills/deploy/references/b file.md> ) and [web](https://example.com/references/a.md).",
		"```",
		"[fenced](references/a.md)",
		"```",
		"  [ref]: ../skills/deploy/references/c.md \"Title\"",
		"[other](other.md)",
	}, "\r\n")
	if got != want {
		t.Errorf("RewriteLocal =\n%q\nwant\n%q", got, want)
	}
}

func TestRewriteLocal_LeavesDocumentUntouchedWhenNothingMatches(t *testing.T) {
	doc := "No links, only [text] and `code`.\n"

	if got := RewriteLocal(doc, func(Link) (string, bool) { return "x", true }); got != doc {
		t.Errorf("RewriteLocal changed a document without links: %q", got)
	}
}

func assertLinks(t *testing.T, got, want []Link) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d links %+v, want %d %+v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i].Line != want[i].Line || got[i].Dest != want[i].Dest {
			t.Errorf("link %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
