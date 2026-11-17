package markdown

import (
	"strings"
	"unicode"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/strikethrough"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// FromHTML turns copied HTML into Markdown. text is the plain text
// copied with it, the one the person saw. It returns "" when the HTML
// holds no link, table, code block, heading or list, since text says the
// same; when its words are not all in text, such as content hidden with
// CSS; and when the conversion fails or comes out empty. Image
// descriptions and link titles are left out, since the person never saw
// them.
func FromHTML(source, text string) string {
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil || !hasStructure(doc) || !wordsWithin(doc, text) {
		return ""
	}
	dropUnseen(doc)
	conv := converter.NewConverter(converter.WithPlugins(
		base.NewBasePlugin(),
		commonmark.NewCommonmarkPlugin(),
		strikethrough.NewStrikethroughPlugin(),
		table.NewTablePlugin(),
	))
	out, err := conv.ConvertNode(doc)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}

func hidden(n *html.Node) bool {
	switch n.DataAtom {
	case atom.Head, atom.Script, atom.Style, atom.Template, atom.Noscript, atom.Iframe, atom.Textarea:
		return true
	}
	return false
}

// hasStructure reports whether Markdown would keep something plain text
// loses.
func hasStructure(n *html.Node) bool {
	if n.Type == html.ElementNode {
		if hidden(n) {
			return false
		}
		switch n.DataAtom {
		case atom.Table, atom.Pre, atom.Ul, atom.Ol, atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
			return true
		case atom.A:
			for _, attr := range n.Attr {
				if attr.Key == "href" && strings.TrimSpace(attr.Val) != "" {
					return true
				}
			}
		}
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if hasStructure(child) {
			return true
		}
	}
	return false
}

// wordsWithin reports whether every word the HTML shows appears in text
// at least as often. Numbers are left out: browsers differ on whether
// list numbers reach the plain text.
func wordsWithin(doc *html.Node, text string) bool {
	left := map[string]int{}
	for _, word := range wordsOf(text) {
		left[word]++
	}
	within := true
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if !within {
			return
		}
		if n.Type == html.ElementNode && hidden(n) {
			return
		}
		var shown string
		if n.Type == html.TextNode {
			shown = n.Data
		}
		for _, word := range wordsOf(shown) {
			if left[word] == 0 {
				within = false
				return
			}
			left[word]--
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return within
}

func wordsOf(text string) []string {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	kept := words[:0]
	for _, word := range words {
		if strings.IndexFunc(word, unicode.IsLetter) >= 0 {
			kept = append(kept, word)
		}
	}
	return kept
}

func dropUnseen(n *html.Node) {
	kept := n.Attr[:0]
	for _, attr := range n.Attr {
		if attr.Key != "alt" && attr.Key != "title" {
			kept = append(kept, attr)
		}
	}
	n.Attr = kept
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		dropUnseen(child)
	}
}
