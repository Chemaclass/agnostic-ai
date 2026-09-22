package cline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// replaceClinerulesFile clears a single-file `.clinerules` out of the
// way of the directory this adapter writes there. Cline still reads the
// file form, and converts it to the directory layout on its own
// (rule-helpers.ts in cline/cline), so
// sync does the same instead of failing on `mkdir: not a directory`.
//
// The file is removed only when its content already lives in a rule
// spec, which is what `import cline` writes, or when it is empty or
// generated. Anything else fails with the import command to run, so
// sync never destroys a rule nobody imported (#1060). Nothing happens
// when no output resolves under `.clinerules` or the path is user-owned
// through sync.unmanaged.
func replaceClinerulesFile(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	path := defaultRulesDir
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || !writesUnderClinerules(cfg) || sess.IsUnmanaged(path) {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if !header.Has(string(data)) && !imported(string(data), b.Rules) {
		return fmt.Errorf("%s is a file whose content no rule spec carries; run `agnostic-ai import cline` to keep it as a rule, then sync again", path)
	}
	_, err = sess.RemoveOwned(path, emit.ContentSum(string(data)), dryRun)
	return err
}

// writesUnderClinerules reports whether any output directory resolves
// to `.clinerules` or below it.
func writesUnderClinerules(cfg *config.Config) bool {
	dirs := []string{
		emit.OutputRulesDir(cfg, target, defaultRulesDir),
		emit.OutputAgentsDir(cfg, target, defaultAgentsDir),
		emit.OutputSkillsDir(cfg, target, defaultSkillsDir),
		emit.OutputHooksDir(cfg, target, defaultHooksDir),
		emit.OutputWorkflowsDir(cfg, target, ""),
	}
	for _, d := range dirs {
		d = filepath.ToSlash(filepath.Clean(d))
		if d == defaultRulesDir || strings.HasPrefix(d, defaultRulesDir+"/") {
			return true
		}
	}
	return false
}

// imported reports whether the body of a `.clinerules` file matches a
// rule spec body, compared the way `import cline` reads it: frontmatter
// and a leading `# ` heading dropped, surrounding whitespace ignored. An
// empty body has nothing to lose.
func imported(content string, rules []spec.Entry) bool {
	e, err := spec.ParseMarkdownBytes(spec.KindRule, []byte(content))
	if err != nil {
		return false
	}
	body := ruleBody(e.Body)
	if body == "" {
		return true
	}
	for _, r := range rules {
		if ruleBody(r.Body) == body {
			return true
		}
	}
	return false
}

// ruleBody trims s and drops a leading `# ` heading line, mirroring the
// heading strip `import cline` applies.
func ruleBody(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "# ") {
		return s
	}
	nl := strings.IndexByte(s, '\n')
	if nl < 0 {
		return ""
	}
	return strings.TrimSpace(s[nl+1:])
}
