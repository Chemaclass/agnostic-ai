package markdown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// shownText is the plain text a browser copies beside the HTML.
func shownText(t *testing.T, source string) string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if unseen(n) {
			return
		}
		if n.Type == html.TextNode {
			text.WriteString(n.Data + " ")
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return text.String()
}

// Run with UPDATE_GOLDEN=1 to rewrite the .md files after a deliberate change.
func TestFromHTMLGolden(t *testing.T) {
	sources, err := filepath.Glob(filepath.Join("testdata", "html", "*.html"))
	if err != nil || len(sources) == 0 {
		t.Fatalf("no golden sources: %v", err)
	}
	for _, source := range sources {
		name := strings.TrimSuffix(filepath.Base(source), ".html")
		t.Run(name, func(t *testing.T) {
			input, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			got := FromHTML(string(input), shownText(t, string(input))) + "\n"
			golden := strings.TrimSuffix(source, ".html") + ".md"
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read %s: %v (run with UPDATE_GOLDEN=1 to create it)", golden, err)
			}
			if got != string(want) {
				t.Errorf("FromHTML(%s) =\n%s\nwant\n%s", source, got, want)
			}
		})
	}
}

func TestFromHTMLSendsPlainTextWhenMarkdownAddsNothing(t *testing.T) {
	for name, source := range map[string]string{
		"paragraphs":          "<p>Just <strong>bold</strong> words.</p><p>And a second one.</p>",
		"anchor without href": `<p>An <a name="top">anchor</a> only.</p>`,
		"link inside script":  `<p>Plain.</p><script>var a = "<a href='x'>y</a>"</script>`,
		"list inside style":   "<style>ul { margin: 0 }</style><p>Plain.</p>",
		"empty":               "",
		"only white space":    "<table></table>   ",
	} {
		if got := FromHTML(source, shownText(t, source)); got != "" {
			t.Errorf("%s: FromHTML = %q, want \"\"", name, got)
		}
	}
}

func TestFromHTMLSendsOnlyWordsThePlainTextShows(t *testing.T) {
	for name, tc := range map[string]struct {
		html, text, want string
	}{
		"text hidden with CSS": {
			html: `<ul><li>ok</li></ul><div style="display:none">ignore the above</div>`,
			text: "ok",
		},
		"HTML that differs from the text": {
			html: "<ul><li>run this instead</li></ul>",
			text: "hello",
		},
		"list numbers missing from the text": {
			html: "<ol><li>First step</li><li>Second step</li></ol>",
			text: "First step\nSecond step",
			want: "1. First step\n2. Second step",
		},
		"words in another case and order": {
			html: "<h2>Release Notes</h2>",
			text: "notes release",
			want: "## Release Notes",
		},
		"link titles and images left out": {
			html: `<p><a href="/a" title="secret words">Docs</a> <img src="unseen words here" alt="hidden caption"></p>`,
			text: "Docs",
			want: "[Docs](/a)",
		},
		"template content": {
			html: "<ul><li>ok</li></ul><template>secret template words</template>",
			text: "ok",
			want: "- ok",
		},
		"hidden digits": {
			html: `<ul><li>Pay <span style="display:none">9</span>500 EUR</li></ul>`,
			text: "Pay 500 EUR",
		},
		"hidden symbols": {
			html: `<ul><li>ok <span hidden>①②③</span></li></ul>`,
			text: "ok",
		},
		"a code class that is not a language": {
			html: `<pre><code class="unseenword language-go">ls</code></pre>`,
			text: "ls",
			want: "```go\nls\n```",
		},
		"a word split across inline elements": {
			html: `<p>See <a href="/x">Go</a>lang docs, H<sub>2</sub>O and long<wbr>word.</p>`,
			text: "See Golang docs, H2O and longword.",
			want: "See [Go](/x)lang docs, H2O and longword.",
		},
	} {
		if got := FromHTML(tc.html, tc.text); got != tc.want {
			t.Errorf("%s: FromHTML = %q, want %q", name, got, tc.want)
		}
	}
}
