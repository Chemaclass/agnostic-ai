package cli

import (
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// codexChainFile is one AGENTS.md in the chain Codex reads for a scope.
type codexChainFile struct {
	path    string
	bytes   int
	reviews int
}

// lintCodexChain sums, per scope, the AGENTS.md bytes Codex reads from
// the project root down to that scope, and warns where the total passes
// the budget (LINT011, warn). A scope under one that already passes
// stays quiet, so each branch reports once, at the first scope to cross.
// The root alone is LINT011's own per-file check.
func lintCodexChain(cfg *config.Config, b spec.Bundle, loads []sessionLoad, budget int) ([]lintFinding, error) {
	if !slices.Contains(cfg.Targets, "codex") {
		return nil, nil
	}
	reviews := adapters.ReviewSections(b, cfg, "codex")
	var root *codexChainFile
	for _, l := range loads {
		if l.target == "codex" && l.file != "" {
			root = &codexChainFile{path: l.path, bytes: l.fileBytes, reviews: len(reviews[""])}
		}
	}
	scoped, err := codexScopedDocuments(cfg, b, reviews)
	if err != nil {
		return nil, err
	}

	dirs := make([]string, 0, len(scoped))
	for dir := range scoped {
		dirs = append(dirs, dir)
	}
	slices.SortFunc(dirs, func(a, c string) int {
		if d := strings.Count(a, "/") - strings.Count(c, "/"); d != 0 {
			return d
		}
		return strings.Compare(a, c)
	})

	var rootChain []codexChainFile
	rootBytes := 0
	if root != nil {
		rootChain, rootBytes = []codexChainFile{*root}, root.bytes
	}
	over := map[string]bool{"": rootBytes > budget}
	var out []lintFinding
	if root != nil && rootBytes > budget && rootBytes <= config.DefaultCodexChainBytes {
		out = append(out, chainFinding("the project root", rootChain, budget))
	}
	for _, dir := range dirs {
		chain, parent := append([]codexChainFile(nil), rootChain...), ""
		var ancestors []codexChainFile
		for up := path.Dir(dir); up != "."; up = path.Dir(up) {
			if f, ok := scoped[up]; ok {
				ancestors = append([]codexChainFile{f}, ancestors...)
				if parent == "" {
					parent = up
				}
			}
		}
		chain = append(append(chain, ancestors...), scoped[dir])
		total := 0
		for _, f := range chain {
			total += f.bytes
		}
		over[dir] = total > budget
		if total > budget && !over[parent] {
			out = append(out, chainFinding(dir, chain, budget))
		}
	}
	return out, nil
}

func chainFinding(scope string, chain []codexChainFile, budget int) lintFinding {
	files := make([]string, 0, len(chain))
	total := 0
	for _, f := range chain {
		entry := fmt.Sprintf("%s %d", f.path, f.bytes)
		if f.reviews > 0 {
			entry += fmt.Sprintf(" (reviews %d)", f.reviews)
		}
		files = append(files, entry)
		total += f.bytes
	}
	if !strings.HasPrefix(scope, "the ") {
		scope = "scope " + scope
	}
	return lintFinding{
		Code:     "LINT011",
		Severity: lintWarn,
		Path:     chain[len(chain)-1].path,
		Message: fmt.Sprintf("Codex reads %d bytes in %s, past %d (lint.codex-chain-bytes; Codex project_doc_max_bytes defaults to %d): %s. Codex drops what passes its limit.",
			total, scope, budget, config.DefaultCodexChainBytes, strings.Join(files, ", ")),
	}
}

// codexScopedDocuments returns the scoped AGENTS.md files sync writes for
// Codex, keyed by scope directory. It captures the adapter's output, so
// the sizes are the bytes on disk.
func codexScopedDocuments(cfg *config.Config, b spec.Bundle, reviews map[string]string) (map[string]codexChainFile, error) {
	adapter, err := adapters.Resolve("codex")
	if err != nil {
		return nil, err
	}
	sess := adapters.NewSession()
	sess.StartCapture()
	err = adapters.EmitWithProvenance(sess, adapter, b, cfg, false)
	files := sess.StopCapture()
	if err != nil {
		return nil, err
	}
	scoped := map[string]codexChainFile{}
	for _, f := range files {
		p := filepath.ToSlash(f.Path)
		if path.Base(p) != "AGENTS.md" || path.Dir(p) == "." {
			continue
		}
		dir := path.Dir(p)
		scoped[dir] = codexChainFile{path: p, bytes: len(f.Content), reviews: len(reviews[dir])}
	}
	return scoped, nil
}
