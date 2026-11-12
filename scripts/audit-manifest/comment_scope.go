package main

import (
	"bytes"
	"fmt"
	"go/parser"
	"go/scanner"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

type sourceToken struct {
	Kind token.Token
	Text string
	Line int
}

func commentTokens(body []byte) ([]sourceToken, bool) {
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, "source.go", body, parser.ParseComments)
	if err != nil {
		return nil, false
	}
	for _, i := range parsed.Imports {
		if i.Path.Value == `"C"` {
			return nil, false
		}
	}
	for _, group := range parsed.Comments {
		for _, c := range group.List {
			if strings.Contains(c.Text, "go:") || strings.HasPrefix(c.Text, "//line ") || strings.HasPrefix(c.Text, "/*line ") || strings.HasPrefix(c.Text, "//export ") || strings.Contains(c.Text, "+build") {
				return nil, false
			}
		}
	}
	var scan scanner.Scanner
	file := fset.AddFile("tokens.go", -1, len(body))
	scan.Init(file, body, nil, scanner.ScanComments)
	var tokens []sourceToken
	for {
		pos, kind, literal := scan.Scan()
		if kind == token.EOF {
			break
		}
		if kind != token.COMMENT {
			tokens = append(tokens, sourceToken{kind, literal, file.Line(pos)})
		}
	}
	return tokens, scan.ErrorCount == 0
}

func commentOnly(before, after []byte) bool {
	if bytes.Equal(before, after) {
		return false
	}
	old, ok := commentTokens(before)
	if !ok {
		return false
	}
	current, ok := commentTokens(after)
	return ok && slices.Equal(old, current) && !sameComments(before, after)
}

func sameComments(before, after []byte) bool {
	comments := func(body []byte) []string {
		f, _ := parser.ParseFile(token.NewFileSet(), "source.go", body, parser.ParseComments)
		var result []string
		for _, group := range f.Comments {
			for _, line := range group.List {
				result = append(result, line.Text)
			}
		}

		return result
	}
	return slices.Equal(comments(before), comments(after))
}

func (a app) commentSource(baseline, path string) ([]byte, []byte, error) {
	if !strings.HasPrefix(path, "internal/cli/") || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.ContainsAny(path, "\r\n\t\\") || filepath.Clean(path) != path {
		return nil, nil, fmt.Errorf("%s: outside comment-only scope", path)
	}
	full := filepath.Join(a.root, path)
	info, err := os.Lstat(full)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0111 != 0 {
		return nil, nil, fmt.Errorf("%s: changed file mode", path)
	}
	oldmode, err := a.command("git", "ls-tree", baseline, "--", path)
	if err != nil || !strings.HasPrefix(oldmode, "100644 blob ") {
		return nil, nil, fmt.Errorf("%s: missing regular baseline file", path)
	}
	index, err := a.command("git", "ls-files", "--stage", "--", path)
	if fields := strings.Fields(index); err != nil || len(fields) != 4 || fields[0] != "100644" || fields[2] != "0" {
		return nil, nil, fmt.Errorf("%s: changed index mode", path)
	}
	cmd := exec.Command("git", "show", baseline+":"+path)
	cmd.Dir = a.root
	before, err := cmd.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("read %s at %s: %w", path, baseline, err)
	}
	after, err := os.ReadFile(full)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	if !commentOnly(before, after) {
		return nil, nil, fmt.Errorf("%s: runtime tokens, directives or positions differ", path)
	}
	return before, after, nil
}

func (a app) commentPaths(baseline string, paths []byte) string {
	var out strings.Builder
	for _, path := range strings.Split(string(paths), "\x00") {
		if path == "" {
			continue
		}
		if _, _, err := a.commentSource(baseline, path); err == nil {
			out.WriteString(path + "\n")
		}
	}
	return out.String()
}
