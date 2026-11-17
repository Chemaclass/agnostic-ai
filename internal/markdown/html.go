package markdown

import (
	"regexp"
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
// CSS; and when the conversion fails or comes out empty. Images, link
// titles and code classes other than a language are left out, since the
// person never saw them.
func FromHTML(source, text string) string {
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return ""
	}
	dropUnseen(doc)
	if !hasStructure(doc) || !wordsWithin(doc, text) {
		return ""
	}
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

func unseen(n *html.Node) bool {
	if n.Type == html.CommentNode {
		return true
	}
	switch n.DataAtom {
	case atom.Head, atom.Script, atom.Style, atom.Template, atom.Noscript, atom.Iframe, atom.Textarea,
		atom.Img, atom.Picture, atom.Svg, atom.Object, atom.Embed, atom.Input, atom.Select:
		return true
	}
	return false
}

// codeLanguage is the only class Markdown keeps, as a fence's language.
var codeLanguage = regexp.MustCompile(`^(?:language|lang)-[a-z0-9+#-]{1,20}$`)

// dropUnseen removes what a page holds but does not show as text, so the
// word check and the conversion read the same tree.
func dropUnseen(n *html.Node) {
	kept := n.Attr[:0]
	for _, attr := range n.Attr {
		switch attr.Key {
		case "alt", "title":
			continue
		case "class":
			var classes []string
			for _, class := range strings.Fields(attr.Val) {
				if codeLanguage.MatchString(class) {
					classes = append(classes, class)
				}
			}
			if len(classes) == 0 {
				continue
			}
			attr.Val = strings.Join(classes, " ")
		}
		kept = append(kept, attr)
	}
	n.Attr = kept
	for child := n.FirstChild; child != nil; {
		next := child.NextSibling
		if unseen(child) {
			n.RemoveChild(child)
		} else {
			dropUnseen(child)
		}
		child = next
	}
}

// hasStructure reports whether Markdown would keep something plain text
// loses.
func hasStructure(n *html.Node) bool {
	if n.Type == html.ElementNode {
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
// at least as often. Inline elements join their words, as a browser's
// plain text does.
func wordsWithin(doc *html.Node, text string) bool {
	left := map[string]int{}
	for _, word := range wordsOf(text) {
		left[word]++
	}
	var shown strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			shown.WriteString(n.Data)
		}
		block := n.Type == html.ElementNode && !inline[n.DataAtom]
		if block {
			shown.WriteByte(' ')
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if block {
			shown.WriteByte(' ')
		}
	}
	walk(doc)
	for _, word := range wordsOf(shown.String()) {
		if left[word] == 0 {
			return false
		}
		left[word]--
	}
	return true
}

// inline are the elements whose text runs on into its neighbours'.
var inline = map[atom.Atom]bool{
	atom.A: true, atom.Abbr: true, atom.B: true, atom.Bdi: true, atom.Bdo: true, atom.Cite: true, atom.Code: true,
	atom.Data: true, atom.Del: true, atom.Dfn: true, atom.Em: true, atom.Font: true, atom.I: true, atom.Ins: true,
	atom.Kbd: true, atom.Label: true, atom.Mark: true, atom.Q: true, atom.S: true, atom.Samp: true, atom.Small: true,
	atom.Span: true, atom.Strike: true, atom.Strong: true, atom.Sub: true, atom.Sup: true, atom.Time: true,
	atom.Tt: true, atom.U: true, atom.Var: true, atom.Wbr: true,
}

// wordsOf splits text at white space and punctuation, so digits and
// symbols count as words too.
func wordsOf(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	})
}
