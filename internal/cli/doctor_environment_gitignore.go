package cli

import (
	"cmp"
	"path"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/gitquery"
)

// gitignoreEdit is one change to a hand-written ignore file that makes
// ignored cloud files trackable. With Replace set, line Line, which holds
// Replace, becomes the Add lines (none deletes it). Otherwise the Add
// lines go after line Line, or at the end of Source when Line is 0.
type gitignoreEdit struct {
	Source  string   `json:"source"`
	Line    int      `json:"line,omitempty"`
	Pattern string   `json:"pattern,omitempty"`
	Replace bool     `json:"replace,omitempty"`
	Add     []string `json:"add"`
}

// planGitignoreEdits returns the edits that stop the ignore files at root
// from ignoring files, slash paths from the root. For each file it finds,
// through `git check-ignore -v`, the line that ignores the file or its
// shallowest ignored folder, because Git cannot re-include a file whose
// folder is ignored. A line that names exactly that folder becomes
// `/dir/*`, so the folder's other files stay ignored; any other line
// stays, followed by `!/dir/` and `/dir/*`. Each folder below gets the
// same pair, then `!/file` re-includes the file. A line that names
// exactly the file goes away. Lines outside the repository's .gitignore
// files, such as .git/info/exclude or a global excludes file, are
// overridden at the end of the root .gitignore, which Git reads after
// them.
func planGitignoreEdits(root string, files []string) ([]gitignoreEdit, error) {
	var queries []string
	for _, file := range files {
		for _, p := range pathChain(file) {
			if !slices.Contains(queries, p) {
				queries = append(queries, p)
			}
		}
	}
	rules, err := gitquery.IgnoreRules(root, queries)
	if err != nil {
		return nil, err
	}
	type key struct {
		source string
		line   int
	}
	edits := map[key]*gitignoreEdit{}
	var order []key
	for _, file := range files {
		var ignored string
		var rule gitquery.IgnoreRule
		for _, p := range pathChain(file) {
			if r, ok := rules[p]; ok && r.Excludes() {
				ignored, rule = p, r
				break
			}
		}
		if ignored == "" {
			continue
		}
		source, line, base := rule.Source, rule.Line, path.Dir(rule.Source)
		if path.Base(source) != ".gitignore" || path.IsAbs(source) || strings.HasPrefix(source, "../") || strings.HasPrefix(source, ".git/") {
			source, line, base = ".gitignore", 0, "."
		}
		k := key{source, line}
		edit, ok := edits[k]
		if !ok {
			edit = &gitignoreEdit{Source: source, Line: line, Add: []string{}}
			if line != 0 {
				edit.Pattern = rule.Pattern
			}
			edits[k] = edit
			order = append(order, k)
		}
		rel := func(p string) string {
			if base == "." {
				return p
			}
			return strings.TrimPrefix(p, base+"/")
		}
		exact := line != 0 && namesExactly(rule.Pattern, rel(ignored))
		var lines []string
		switch {
		case ignored == file && exact:
			edit.Replace = true
		case ignored == file:
			lines = append(lines, "!/"+rel(file))
		default:
			if exact {
				edit.Replace = true
			} else {
				lines = append(lines, "!/"+rel(ignored)+"/")
			}
			lines = append(lines, "/"+rel(ignored)+"/*")
			for _, dir := range pathChain(file)[strings.Count(ignored, "/")+1:] {
				if dir == file {
					break
				}
				lines = append(lines, "!/"+rel(dir)+"/", "/"+rel(dir)+"/*")
			}
			lines = append(lines, "!/"+rel(file))
		}
		for _, l := range lines {
			if !slices.Contains(edit.Add, l) {
				edit.Add = append(edit.Add, l)
			}
		}
	}
	out := make([]gitignoreEdit, 0, len(order))
	for _, k := range order {
		out = append(out, *edits[k])
	}
	slices.SortStableFunc(out, func(a, b gitignoreEdit) int {
		if a.Source != b.Source {
			return cmp.Compare(a.Source, b.Source)
		}
		// Appends (line 0) go last.
		if (a.Line == 0) != (b.Line == 0) {
			return cmp.Compare(b.Line, a.Line)
		}
		return cmp.Compare(a.Line, b.Line)
	})
	return out, nil
}

// pathChain returns file's folders, shallowest first, then file.
func pathChain(file string) []string {
	parts := strings.Split(file, "/")
	chain := make([]string, 0, len(parts))
	for i := range parts {
		chain = append(chain, strings.Join(parts[:i+1], "/"))
	}
	return chain
}

// namesExactly reports whether the gitignore pattern matches only p, a
// slash path from the pattern's folder: a literal path anchored there by
// a leading slash or an inner one, with an optional trailing slash.
func namesExactly(pattern, p string) bool {
	if strings.ContainsAny(pattern, `*?[\!`) {
		return false
	}
	trimmed := strings.TrimSuffix(pattern, "/")
	if strings.HasPrefix(trimmed, "/") {
		return trimmed == "/"+p
	}
	return strings.Contains(trimmed, "/") && trimmed == p
}
