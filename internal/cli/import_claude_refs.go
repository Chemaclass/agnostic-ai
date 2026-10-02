package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// importWritten collects the paths importWriteFile writes while non-nil.
// Sequential use only, like importRunSources.
var importWritten map[string]bool

// trackImportWrites starts collecting written paths and returns the stop
// function, which hands back what was written. Calls after the first
// return nil, so a deferred call is safe.
func trackImportWrites() func() map[string]bool {
	prior := importWritten
	importWritten = map[string]bool{}
	stopped := false
	return func() map[string]bool {
		if stopped {
			return nil
		}
		stopped = true
		written := importWritten
		importWritten = prior
		for p := range written {
			if prior != nil {
				prior[p] = true
			}
		}
		return written
	}
}

// nativeRefPrefix is one Claude-native spec directory and the source
// directory import copied it into.
type nativeRefPrefix struct{ native, source string }

// claudeRefPrefixes pairs each native directory with its source path.
func claudeRefPrefixes(src config.Sources, layout claudeLayout) []nativeRefPrefix {
	var out []nativeRefPrefix
	for _, p := range [][2]string{
		{layout.skills, src.Skills},
		{layout.rules, src.Rules},
		{layout.agents, src.Agents},
		{layout.commands, src.Commands},
	} {
		if p[0] != "" && p[1] != "" {
			out = append(out, nativeRefPrefix{filepath.ToSlash(filepath.Clean(p[0])), filepath.ToSlash(filepath.Clean(p[1]))})
		}
	}
	return out
}

// nativeRefRE matches a project-relative path in prose: a run of path
// characters that does not continue a longer word or path.
var nativeRefRE = regexp.MustCompile(`(^|[^\w./-])((?:\./)?\.?[\w-][\w./-]*)`)

// resolveClaudeNativeRefs points the imported specs' references to
// Claude-native spec paths at the sources import wrote. An `@` line in
// the entry file that imports an imported rule is dropped, since sync
// delivers that rule to every target. A native path with no imported
// spec behind it is reported and left as is.
func resolveClaudeNativeRefs(root string, src config.Sources, layout claudeLayout, written map[string]bool) error {
	prefixes := claudeRefPrefixes(src, layout)
	entry := filepath.Clean(filepath.Join(root, agnosticMainFile))
	rulesPrefix := filepath.ToSlash(filepath.Clean(layout.rules)) + "/"
	paths := make([]string, 0, len(written))
	for p := range written {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if !strings.HasSuffix(path, ".md") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		text := string(data)
		if path == entry {
			text = dropImportedRuleLines(root, text, rulesPrefix, filepath.ToSlash(filepath.Clean(src.Rules)), written)
		}
		for _, p := range prefixes {
			if strings.Contains(text, p.native+"/") {
				text = rewriteNativeRefs(root, path, text, prefixes)
				break
			}
		}
		if text == string(data) {
			continue
		}
		if err := importWriteFile(path, []byte(text), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// dropImportedRuleLines removes each `@<rules dir>/<rule>` line whose rule
// this import wrote to the sources.
func dropImportedRuleLines(root, text, rulesPrefix, rulesSource string, written map[string]bool) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		ref := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(line), "@"), "./")
		if strings.HasPrefix(strings.TrimSpace(line), "@") && strings.HasPrefix(ref, rulesPrefix) {
			rule := filepath.Clean(filepath.Join(importSourcePath(root, rulesSource), strings.TrimPrefix(ref, rulesPrefix)))
			if written[rule] {
				if n := len(out); n > 0 && strings.TrimSpace(out[n-1]) == "" {
					out = out[:n-1]
				}
				continue
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// rewriteNativeRefs swaps the native directory of each reference whose
// source counterpart exists, and reports the rest.
func rewriteNativeRefs(root, path, text string, prefixes []nativeRefPrefix) string {
	return nativeRefRE.ReplaceAllStringFunc(text, func(m string) string {
		sub := nativeRefRE.FindStringSubmatch(m)
		lead, ref := sub[1], sub[2]
		ref, trail := strings.TrimRight(ref, "."), ref[len(strings.TrimRight(ref, ".")):]
		dot := ""
		bare := ref
		if strings.HasPrefix(bare, "./") {
			dot, bare = "./", bare[2:]
		}
		for _, p := range prefixes {
			if !strings.HasPrefix(bare, p.native+"/") || len(bare) == len(p.native)+1 {
				continue
			}
			mapped := p.source + strings.TrimPrefix(bare, p.native)
			if _, err := os.Stat(importSourcePath(root, filepath.FromSlash(mapped))); err != nil {
				rel, relErr := filepath.Rel(root, path)
				if relErr != nil {
					rel = path
				}
				summaryf("  ! %s names %s, which no imported spec replaces; left as is\n", filepath.ToSlash(rel), bare)
				return m
			}
			if filepath.IsAbs(filepath.FromSlash(mapped)) {
				dot = ""
			}
			return lead + dot + mapped + trail
		}
		return m
	})
}
