package cli

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func codexFileItems(cfg *config.Config, b spec.Bundle, adapter adapters.Adapter, docs []instructionDoc, rel, projectRoot string, reached map[string]bool) ([]fileContextItem, error) {
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
	byPath := make(map[string]instructionDoc, len(docs))
	for _, d := range docs {
		byPath[identity(d.Path)] = d
	}
	var items []fileContextItem
	seen := map[string]bool{}
	add := func(d instructionDoc, source string) {
		key := source + "\x00" + d.Path
		if seen[key] {
			return
		}
		seen[key] = true
		reached[source] = true
		nativePath, err := filepath.Rel(root, identity(d.Path))
		if err != nil {
			nativePath = d.Path
		}
		override, present := byPath[identity(filepath.Join(filepath.Dir(identity(d.Path)), "AGENTS.override.md"))]
		shadowed := present && strings.TrimSpace(override.Content) != ""
		item := classifyCodexDocument(filepath.ToSlash(nativePath), rel, shadowed)
		item.Source, item.Output = source, d.Path
		items = append(items, item)
	}
	for _, d := range docs {
		if d.CanonicalBody {
			add(d, adapters.AgnosticEntryPointPath)
		}
		for _, marker := range sourceMarkerRE.FindAllStringSubmatch(d.Content, -1) {
			add(d, marker[1])
		}
		for _, r := range b.Rules {
			source := filepath.ToSlash(adapters.EntrySourcePath(r))
			if !strings.ContainsAny(source, " \t") {
				continue
			}
			for _, line := range strings.Split(d.Content, "\n") {
				line = strings.TrimSpace(line)
				remainder, matches := strings.CutPrefix(line, "<!-- source: "+source+" ")
				if matches && (remainder == "-->" || strings.HasPrefix(remainder, "headings:") || strings.HasPrefix(remainder, "body-lines:")) {
					add(d, source)
					break
				}
			}
		}
	}
	for _, r := range b.For("codex").Rules {
		files, err := captureEmit(adapter, singleEntryBundle(r), cfg)
		if err != nil {
			return nil, fmt.Errorf("codex rule %s: %w", adapters.EntrySourcePath(r), err)
		}
		for _, f := range files {
			if d, ok := byPath[identity(f.Path)]; ok {
				add(d, filepath.ToSlash(adapters.EntrySourcePath(r)))
			}
		}
	}
	return items, nil
}

func classifyCodexDocument(output, rel string, shadowed bool) fileContextItem {
	base, dir := path.Base(output), path.Dir(output)
	if base != "AGENTS.md" && base != "AGENTS.override.md" || output == ".." || strings.HasPrefix(output, "../") || filepath.IsAbs(filepath.FromSlash(output)) {
		return fileContextItem{Status: contextUnknown, Reason: "the planned output is outside native project AGENTS.md discovery; loading needs a fallback filename or session setting that this command cannot establish"}
	}
	if base == "AGENTS.md" && shadowed {
		return fileContextItem{Status: contextNoMatch, Selector: "filename priority", Reason: "the planned AGENTS.override.md in the same directory takes priority over this AGENTS.md during Codex instruction discovery"}
	}
	if dir == "." {
		return fileContextItem{Status: contextAlways, Selector: "project " + base, Reason: "Codex discovers project-root " + base + " at startup, with no file condition"}
	}
	relation := "outside "
	if strings.HasPrefix(rel, dir+"/") {
		relation = "under "
	}
	return fileContextItem{Status: contextUnknown, Selector: "directory: " + dir, Reason: "the file is " + relation + dir + "/; Codex discovers instructions from the project root to the session launch directory, which this command cannot establish"}
}
