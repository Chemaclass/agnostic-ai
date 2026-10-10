package cli

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func claudeFileItems(cfg *config.Config, b spec.Bundle, adapter adapters.Adapter, docs []instructionDoc, rel, projectRoot string, reached map[string]bool) ([]fileContextItem, error) {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", projectRoot, err)
	}
	root = resolveSymlinks(root)
	identity := func(p string) string {
		p = filepath.FromSlash(p)
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, p)
		}
		return resolveSymlinks(filepath.Clean(p))
	}
	discoveryPath := func(p string) string {
		relative, err := filepath.Rel(root, identity(p))
		if err != nil {
			return filepath.ToSlash(p)
		}
		return filepath.ToSlash(relative)
	}
	byPath := make(map[string]instructionDoc, len(docs))
	for _, d := range docs {
		byPath[identity(d.Path)] = d
	}
	entryPoint := identity(adapters.EntryPointPath(cfg, "claude"))
	imported := map[string]bool{}
	visitedDepth := map[string]int{}
	var visitImports func(instructionDoc, int)
	visitImports = func(d instructionDoc, depth int) {
		p := identity(d.Path)
		if previous, ok := visitedDepth[p]; ok && previous <= depth {
			return
		}
		visitedDepth[p] = depth
		if depth == 4 {
			return
		}
		for _, ref := range claudeImportRefs(d.Content) {
			p := filepath.FromSlash(ref)
			if !filepath.IsAbs(p) {
				p = filepath.Join(filepath.Dir(identity(d.Path)), p)
			}
			p = identity(p)
			if next, ok := byPath[p]; ok {
				imported[p] = true
				visitImports(next, depth+1)
			}
		}
	}
	for _, d := range docs {
		if path.Base(discoveryPath(d.Path)) == "CLAUDE.md" || identity(d.Path) == entryPoint {
			visitImports(d, 0)
		}
	}

	var items []fileContextItem
	seen := map[string]bool{}
	add := func(item fileContextItem, source, output string) {
		key := source + "\x00" + output
		if seen[key] {
			return
		}
		seen[key] = true
		item.Source, item.Output = source, output
		reached[source] = true
		items = append(items, item)
	}
	for _, d := range docs {
		p := identity(d.Path)
		nativePath := discoveryPath(d.Path)
		if path.Base(nativePath) != "CLAUDE.md" && p != entryPoint && !isClaudeRulePath(nativePath) && !imported[p] {
			continue
		}
		item := classifyClaudeOutput(d, nativePath, rel, imported[p])
		if d.CanonicalBody {
			if importsAgents(d.Content) {
				item = claudeImportItem()
			}
			add(item, adapters.AgnosticEntryPointPath, d.Path)
		}
		for _, marker := range sourceMarkerRE.FindAllStringSubmatch(d.Content, -1) {
			add(item, marker[1], d.Path)
		}
	}

	for _, r := range b.For("claude").Rules {
		files, err := captureEmit(adapter, singleEntryBundle(r), cfg)
		if err != nil {
			return nil, fmt.Errorf("claude rule %s: %w", adapters.EntrySourcePath(r), err)
		}
		for _, f := range files {
			p := identity(f.Path)
			d, ok := byPath[p]
			if !ok {
				continue
			}
			item := classifyClaudeOutput(d, discoveryPath(d.Path), rel, imported[p])
			add(item, filepath.ToSlash(adapters.EntrySourcePath(r)), d.Path)
		}
	}
	return items, nil
}

func claudeImportItem() fileContextItem {
	return fileContextItem{Status: contextUnknown, Selector: "@-import", Reason: "the planned document imports this instruction; its loading can depend on the session launch directory and import approval"}
}

func isClaudeRulePath(p string) bool {
	return strings.HasPrefix(p, ".claude/rules/") && path.Ext(p) == ".md"
}

func classifyClaudeOutput(d instructionDoc, nativePath, rel string, imported bool) fileContextItem {
	if isClaudeRulePath(nativePath) {
		item := classifyClaudePaths(d.Content, rel)
		if imported && item.Status == contextNoMatch {
			item.Status = contextUnknown
			item.Reason = "no paths pattern matches, but the planned document also imports this rule; loading through that import depends on the session launch directory and import approval"
		}
		return item
	}
	if path.Base(nativePath) == "CLAUDE.md" {
		return classifyClaudeDocument(nativePath, rel)
	}
	if imported {
		return claudeImportItem()
	}
	return fileContextItem{Status: contextUnknown, Reason: "the planned output is outside native CLAUDE.md and .claude/rules/ discovery; loading needs an import or session setting that this command cannot establish"}
}

func classifyClaudeDocument(output, rel string) fileContextItem {
	dir := path.Dir(output)
	if path.Base(dir) == ".claude" {
		dir = path.Dir(dir)
	}
	if dir == "." {
		return fileContextItem{Status: contextAlways, Selector: "project CLAUDE.md", Reason: "Claude Code loads project CLAUDE.md instructions at startup, with no file condition"}
	}
	selector := "directory: " + dir
	if strings.HasPrefix(rel, dir+"/") {
		return fileContextItem{Status: contextMatch, Selector: selector, Reason: "the file is under " + dir + "/; Claude Code loads nested CLAUDE.md instructions when it reads or edits a file there"}
	}
	return fileContextItem{Status: contextUnknown, Selector: selector, Reason: "the file is outside " + dir + "/; the session launch directory determines whether this CLAUDE.md is an ancestor loaded at startup"}
}

func classifyClaudePaths(content, rel string) fileContextItem {
	raw, _, hasFront := splitFrontmatter([]byte(content))
	var fm map[string]any
	if hasFront && yaml.Unmarshal(raw, &fm) != nil {
		return fileContextItem{Status: contextUnknown, Reason: "the planned rule has unreadable paths frontmatter"}
	}
	value, scoped := fm["paths"]
	if !scoped {
		return fileContextItem{Status: contextAlways, Selector: "no paths", Reason: "Claude Code discovers Markdown files under .claude/rules/ recursively; this rule has no file condition"}
	}
	patterns := spec.GlobList(value)
	selector := "paths: " + strings.Join(patterns, ", ")
	if len(patterns) == 0 {
		return fileContextItem{Status: contextUnknown, Selector: selector, Reason: "the planned paths value has no readable patterns"}
	}
	unknown := false
	for _, p := range patterns {
		if strings.ContainsAny(p, "{}[]!\\") || strings.HasPrefix(p, "./") || path.IsAbs(p) {
			unknown = true
			continue
		}
		pattern, err := compileGlob(p)
		if err != nil {
			unknown = true
			continue
		}
		if pattern.MatchString(rel) {
			return fileContextItem{Status: contextMatch, Selector: selector, Reason: "pattern " + p + " matches the file; Claude Code loads the rule when it reads or edits a matching file"}
		}
	}
	if unknown {
		return fileContextItem{Status: contextUnknown, Selector: selector, Reason: "a paths pattern uses syntax this command cannot evaluate with Claude Code's matching and expansion limits"}
	}
	return fileContextItem{Status: contextNoMatch, Selector: selector, Reason: "no paths pattern matches the file"}
}

var claudeImportPattern = regexp.MustCompile("(?:^|\\s)@((?:\\\\[ \t]|[^\\s`])+)")

func claudeImportRefs(content string) []string {
	source := []byte(content)
	masked := []byte(content)
	mask := func(start, stop int) {
		for i := start; i < stop; i++ {
			masked[i] = ' '
		}
	}
	doc := parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	).Parse(text.NewReader(source))
	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch code := node.(type) {
		case *ast.CodeSpan:
			for child := code.FirstChild(); child != nil; child = child.NextSibling() {
				if segment, ok := child.(*ast.Text); ok {
					mask(segment.Segment.Start, segment.Segment.Stop)
				}
			}
			return ast.WalkSkipChildren, nil
		case *ast.FencedCodeBlock:
			if code.Info != nil {
				mask(code.Info.Segment.Start, code.Info.Segment.Stop)
			}
		case *ast.CodeBlock:
		default:
			return ast.WalkContinue, nil
		}
		for i := 0; i < node.Lines().Len(); i++ {
			segment := node.Lines().At(i)
			mask(segment.Start, segment.Stop)
		}
		return ast.WalkSkipChildren, nil
	})
	var refs []string
	for _, match := range claudeImportPattern.FindAllStringSubmatch(string(masked), -1) {
		ref := strings.ReplaceAll(match[1], "\\ ", " ")
		refs = append(refs, strings.ReplaceAll(ref, "\\\t", "\t"))
	}
	return refs
}
