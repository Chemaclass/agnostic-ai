package cli

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

type packagingIgnoreFinding struct {
	Path      string   `json:"path"`
	Uncovered []string `json:"uncovered,omitempty"`
	Problem   string   `json:"problem,omitempty"`
}

func collectPackagingIgnoreFindings(reports []driftReport) []packagingIgnoreFinding {
	generated := map[string]bool{}
	root, err := os.Getwd()
	if err != nil {
		return []packagingIgnoreFinding{{Problem: fmt.Sprintf("project directory: %v", err)}}
	}
	for _, report := range reports {
		for _, files := range [][]adapters.CapturedFile{report.Current, report.Missing, report.Stale, report.Edited} {
			for _, file := range files {
				absolute, err := filepath.Abs(file.Path)
				if err != nil {
					continue
				}
				rel, err := filepath.Rel(root, absolute)
				if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					continue
				}
				generated[filepath.ToSlash(rel)] = true
			}
		}
	}
	paths := make([]string, 0, len(generated))
	for name := range generated {
		paths = append(paths, name)
	}
	slices.Sort(paths)
	if len(paths) == 0 {
		return nil
	}
	var findings []packagingIgnoreFinding
	for _, name := range []string{".npmignore", ".vscodeignore", ".dockerignore"} {
		data, err := os.ReadFile(name)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			findings = append(findings, packagingIgnoreFinding{Path: name, Problem: fmt.Sprintf("read %s: %v", name, err)})
			continue
		}
		rules, err := parsePackagingIgnore(name, string(data))
		if err != nil {
			findings = append(findings, packagingIgnoreFinding{Path: name, Problem: err.Error()})
			continue
		}
		var uncovered []string
		for _, file := range paths {
			if !packagingIgnored(name, rules, file) {
				uncovered = append(uncovered, file)
			}
		}
		if len(uncovered) > 0 {
			findings = append(findings, packagingIgnoreFinding{Path: name, Uncovered: uncovered})
		}
	}
	return findings
}

func reportPackagingIgnoreFindings(cmd *cobra.Command, findings []packagingIgnoreFinding) {
	if len(findings) == 0 {
		return
	}
	cmd.Println()
	cmd.Println("Packaging ignores:")
	for _, finding := range findings {
		if finding.Problem != "" {
			cmd.Printf("  ! %s: cannot check generated-path coverage: %s\n", finding.Path, finding.Problem)
			continue
		}
		cmd.Printf("  ! %s does not cover generated paths:\n", finding.Path)
		for _, file := range finding.Uncovered {
			cmd.Printf("      %s\n", file)
		}
	}
	cmd.Println("  Review these ignore files, then inspect the package or build context before publishing.")
}

type packagingIgnoreRule struct {
	pattern                     string
	negate, anchored, directory bool
}

func parsePackagingIgnore(format, text string) ([]packagingIgnoreRule, error) {
	var rules []packagingIgnoreRule
	for index, line := range strings.Split(strings.TrimPrefix(text, "\ufeff"), "\n") {
		if format == ".dockerignore" && strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" || format != ".dockerignore" && strings.HasPrefix(line, "#") {
			continue
		}
		rule := packagingIgnoreRule{}
		if format == ".vscodeignore" && strings.HasPrefix(line, "!!") {
			return nil, fmt.Errorf("line %d uses unsupported pattern %q", index+1, line)
		}
		if format == ".dockerignore" {
			rule.negate = strings.HasPrefix(line, "!")
			if rule.negate {
				line = strings.TrimSpace(line[1:])
			}
		} else {
			for strings.HasPrefix(line, "!") {
				rule.negate = !rule.negate
				line = line[1:]
			}
		}
		rule.anchored = strings.HasPrefix(line, "/")
		rule.directory = strings.HasSuffix(line, "/")
		switch format {
		case ".dockerignore":
			line = strings.TrimPrefix(path.Clean(line), "/")
			rule.directory = false
		case ".npmignore":
			line = strings.TrimPrefix(line, "/")
			line = strings.TrimSuffix(line, "/")
		}
		if line == "" {
			return nil, fmt.Errorf("line %d has an empty pattern", index+1)
		}
		if line == "." {
			continue
		}
		if strings.ContainsAny(line, "{}") || strings.Contains(line, "[!") || strings.Contains(line, "[:") || strings.Contains(line, "\\") || strings.ContainsAny(line, "()") {
			return nil, fmt.Errorf("line %d uses unsupported pattern %q", index+1, line)
		}
		for _, part := range strings.Split(line, "/") {
			if format == ".dockerignore" && strings.Contains(part, "**") && part != "**" {
				return nil, fmt.Errorf("line %d uses unsupported pattern %q", index+1, line)
			}
			if _, err := path.Match(part, ""); err != nil {
				return nil, fmt.Errorf("line %d pattern %q: %w", index+1, line, err)
			}
		}
		if format == ".npmignore" {
			line = strings.ToLower(line)
		}
		rule.pattern = line
		rules = append(rules, rule)
	}
	return rules, nil
}

func packagingIgnored(format string, rules []packagingIgnoreRule, file string) bool {
	if format == ".npmignore" {
		file = strings.ToLower(file)
		parts := strings.Split(file, "/")
		for depth := 1; depth <= len(parts); depth++ {
			prefix := strings.Join(parts[:depth], "/")
			directory := depth < len(parts)
			ignored := false
			for _, rule := range rules {
				matched := packagingRuleMatches(rule, prefix, directory)
				if rule.negate && directory && !matched && strings.Contains(rule.pattern, "/") {
					matched = packagingGlob(rule.pattern, prefix, true)
				}
				if matched {
					ignored = !rule.negate
				}
			}
			if ignored {
				return true
			}
		}
		return false
	}
	ignored := false
	for _, rule := range rules {
		matched := false
		if format == ".dockerignore" {
			parts := strings.Split(file, "/")
			for depth := 1; depth <= len(parts); depth++ {
				if packagingGlob(rule.pattern, strings.Join(parts[:depth], "/"), false) {
					matched = true
					break
				}
			}
		} else {
			// VSCE patterns are root-relative; any matching negation wins regardless of order.
			if strings.HasPrefix(rule.pattern, "/") {
				continue
			}
			matched = packagingVSCEGlob(rule.pattern, file)
			last := path.Base(rule.pattern)
			if !matched && (rule.directory || !strings.Contains(last, "*")) {
				matched = packagingVSCEGlob(strings.TrimSuffix(rule.pattern, "/")+"/**", file)
			}
		}
		if matched {
			if format == ".vscodeignore" && rule.negate {
				return false
			}
			ignored = !rule.negate
		}
	}
	return ignored
}

func packagingRuleMatches(rule packagingIgnoreRule, file string, directory bool) bool {
	if rule.directory && !directory {
		return false
	}
	if !rule.anchored && !strings.Contains(rule.pattern, "/") {
		file = path.Base(file)
	}
	return packagingGlob(rule.pattern, file, false)
}

func packagingGlob(pattern, file string, partial bool) bool {
	return packagingGlobMatch(pattern, file, partial, false)
}

func packagingVSCEGlob(pattern, file string) bool {
	return packagingGlobMatch(pattern, file, false, true)
}

func packagingGlobMatch(pattern, file string, partial, separatorRequired bool) bool {
	patterns, names := strings.Split(pattern, "/"), strings.Split(file, "/")
	memo := map[[2]int]bool{}
	var match func(int, int) bool
	match = func(pi, ni int) (matched bool) {
		key := [2]int{pi, ni}
		if cached, ok := memo[key]; ok {
			return cached
		}
		defer func() { memo[key] = matched }()
		if ni == len(names) {
			if partial {
				return true
			}
			// Minimatch requires a slash before a trailing globstar after a plain segment.
			if separatorRequired && pi > 0 && pi < len(patterns) && patterns[pi-1] != "**" {
				return false
			}
			for pi < len(patterns) && patterns[pi] == "**" {
				pi++
			}
			return pi == len(patterns)
		}
		if pi == len(patterns) {
			return false
		}
		if patterns[pi] == "**" {
			if separatorRequired && pi == len(patterns)-1 {
				return true
			}
			return match(pi+1, ni) || match(pi, ni+1)
		}
		return config.MatchUnmanaged([]string{patterns[pi]}, names[ni]) && match(pi+1, ni+1)
	}
	return match(0, 0)
}
