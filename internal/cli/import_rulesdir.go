package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// rulesDirCounts is the per-kind import tally for rules-directory imports.
type rulesDirCounts struct {
	rules, agents, skills int
}

// importRulesDirectory walks srcDir for .md files and reclassifies each
// as a rule, agent, or skill based on filename prefix:
//
//	agent-<name>.md → agents/<name>.md
//	skill-<name>.md → skills/<name>.md
//	<name>.md       → rules/<name>.md
//
// Subdirectories under srcDir are preserved as scope. Body is the file
// content with any leading `# <heading>\n\n` stripped (the heading is
// re-emitted on sync).
func importRulesDirectory(root, srcDir string) (rulesDirCounts, error) {
	var c rulesDirCounts
	full := filepath.Join(root, srcDir)
	if _, err := os.Stat(full); errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}

	err := filepath.WalkDir(full, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, err := filepath.Rel(full, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		kindDir, baseName := classifyRulesDirFile(rel)
		body := stripLeadingHeading(string(data))
		out := filepath.Join(root, kindDir, filepath.Dir(rel), baseName+".md")
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(out), err)
		}
		content := fmt.Sprintf("---\nname: %s\n---\n\n%s", baseName, body)
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		if err := os.WriteFile(out, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", out, err)
		}
		switch kindDir {
		case "agents":
			c.agents++
		case "skills":
			c.skills++
		default:
			c.rules++
		}
		return nil
	})
	if err != nil {
		return c, err
	}
	return c, nil
}

// classifyRulesDirFile maps a relative `.md` path under a rules
// directory to (kindDir, baseName) by inspecting the filename prefix.
func classifyRulesDirFile(rel string) (kindDir, baseName string) {
	base := strings.TrimSuffix(filepath.Base(rel), ".md")
	switch {
	case strings.HasPrefix(base, "agent-"):
		return "agents", strings.TrimPrefix(base, "agent-")
	case strings.HasPrefix(base, "skill-"):
		return "skills", strings.TrimPrefix(base, "skill-")
	default:
		return "rules", base
	}
}

// stripLeadingHeading removes a `# <text>\n\n` block at the start of
// content, if present. The adapter prepends one on emit; removing it
// here keeps round-trips clean.
func stripLeadingHeading(content string) string {
	if !strings.HasPrefix(content, "# ") {
		return content
	}
	nl := strings.IndexByte(content, '\n')
	if nl < 0 {
		return ""
	}
	rest := content[nl+1:]
	rest = strings.TrimLeft(rest, "\n")
	return rest
}

// rulesDirImportConfig builds a minimal agnostic.config.yaml string with
// only the given target enabled.
func rulesDirImportConfig(target string) string {
	return fmt.Sprintf(`version: 1

sources:
  agents: agents
  skills: skills
  rules: rules
  hooks: hooks

targets:
  - %s

on-unsupported: warn
`, target)
}

// importFromRulesDir is the shared entry point for cline/windsurf/continue
// importers. It refuses to overwrite an existing config, scaffolds the
// source dirs, walks the adapter's rules directory, and writes a
// single-target config.
func importFromRulesDir(root, target, srcDir string) error {
	cfgPath := filepath.Join(root, "agnostic.config.yaml")
	if _, err := os.Stat(cfgPath); err == nil {
		return fmt.Errorf("agnostic.config.yaml already exists")
	}
	if err := ensureSourceDirs(root); err != nil {
		return err
	}
	c, err := importRulesDirectory(root, srcDir)
	if err != nil {
		return err
	}
	if err := os.WriteFile(cfgPath, []byte(rulesDirImportConfig(target)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", cfgPath, err)
	}
	fmt.Printf("imported %d rules, %d agents, %d skills\n", c.rules, c.agents, c.skills)
	return nil
}
