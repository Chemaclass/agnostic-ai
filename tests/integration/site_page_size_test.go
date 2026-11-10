package integration

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

const (
	siteDocsPageWordLimit      = 6000
	siteDocsTableCellWordLimit = 150
)

func TestSiteDocs_PagesAndTableCellsStayReadable(t *testing.T) {
	pages := 0
	err := filepath.WalkDir(siteDocsContentDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		pages++
		source := readBuiltFile(t, path)
		name, _ := filepath.Rel(siteDocsContentDir, path)

		if words := len(strings.Fields(source)); words > siteDocsPageWordLimit {
			t.Errorf("%s has %d words, limit %d: move its largest command or topic to its own page", name, words, siteDocsPageWordLimit)
		}

		inFence := false
		for number, line := range strings.Split(source, "\n") {
			if strings.HasPrefix(line, "```") {
				inFence = !inFence
			}
			if inFence || !strings.HasPrefix(strings.TrimSpace(line), "|") {
				continue
			}
			for _, cell := range tableCells(line) {
				if words := len(strings.Fields(cell)); words > siteDocsTableCellWordLimit {
					t.Errorf("%s:%d has a table cell of %d words, limit %d: give the row its own heading", name, number+1, words, siteDocsTableCellWordLimit)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk documentation pages: %v", err)
	}
	if pages == 0 {
		t.Fatal("no documentation pages found")
	}
}

// tableCells splits a Markdown table row on the pipes that separate cells,
// not on escaped pipes or pipes inside inline code.
func tableCells(row string) []string {
	var cells []string
	var cell strings.Builder
	inCode, escaped := false, false
	for _, r := range row {
		switch {
		case escaped:
			escaped = false
		case r == '\\':
			escaped = true
		case r == '`':
			inCode = !inCode
		case r == '|' && !inCode:
			cells = append(cells, cell.String())
			cell.Reset()
			continue
		}
		cell.WriteRune(r)
	}
	return append(cells, cell.String())
}
