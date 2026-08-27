package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

type rulesDirCounts struct {
	rules, agents, skills int
}

// add folds one directory's counts into c, so an importer that reads
// several rules directories reports a single total.
func (c *rulesDirCounts) add(o rulesDirCounts) {
	c.rules += o.rules
	c.agents += o.agents
	c.skills += o.skills
}

// rulesDirImportOpts tunes importRulesDirectoryWith for a target whose
// rules directory is not a plain root-level tree. Zero value reproduces
// importRulesDirectory exactly.
type rulesDirImportOpts struct {
	// ScopePrefix is prepended to every imported spec's destination
	// path. Set when srcDir is a copy of the tool directory living in a
	// project sub-directory, so `backend/.devin/rules/auth.md` imports
	// back to `rules/backend/auth.md` (#628).
	ScopePrefix string
	// NormalizeMeta rewrites one file's parsed frontmatter before
	// rulesDirFileContent renders the spec, so a target-native
	// activation key translates back into the generic
	// `description` / `globs` / `alwaysApply` fields.
	NormalizeMeta func(map[string]any)
}

// importRulesDirectory walks srcDir for .md files and reclassifies each
// as a rule, agent, or skill based on filename prefix:
//
//	agent-<name>.md → <agentsDst>/<name>.md
//	skill-<name>.md → <skillsDst>/<name>.md
//	<name>.md       → <rulesDst>/<name>.md
//
// Subdirectories under srcDir are preserved as scope. A leading YAML
// frontmatter block, when present (Trae's rule and agent files carry
// `description` / `globs` / `alwaysApply`; Cline, Windsurf, and
// Continue's plain files carry none), is parsed with splitMdcFrontmatter
// and its recognized fields captured back into the imported spec via
// rulesDirFileContent rather than left stuck in the body (#607: without
// this, importing a Trae file whose body now starts with its own
// frontmatter block doubled that block on the next sync). The leading
// `# <heading>\n\n` line the RulesDirectory family writes after any
// frontmatter (re-emitted on sync) is stripped from the body either way.
func importRulesDirectory(root, srcDir string, src config.Sources) (rulesDirCounts, error) {
	return importRulesDirectoryWith(root, srcDir, src, rulesDirImportOpts{})
}

// importRulesDirectoryWith is importRulesDirectory with the per-target
// knobs in rulesDirImportOpts applied.
func importRulesDirectoryWith(root, srcDir string, src config.Sources, opts rulesDirImportOpts) (rulesDirCounts, error) {
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

		kind, baseName := classifyRulesDirFile(rel)
		dstDir := pickKindDir(kind, src)
		meta, rest := splitMdcFrontmatter([]byte(header.Strip(string(data))))
		if opts.NormalizeMeta != nil {
			opts.NormalizeMeta(meta)
		}
		body := stripLeadingHeading(rest)
		out := filepath.Join(root, dstDir, opts.ScopePrefix, scopeDir(rel), baseName+".md")
		if err := importMkdirAll(filepath.Dir(out), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(out), err)
		}
		content := rulesDirFileContent(baseName, meta, body)
		if err := importWriteFile(out, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", out, err)
		}
		switch kind {
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

// rulesDirFileContent renders one imported rule/agent/skill spec:
// `name` from the filename, plus `description` / `globs` /
// `alwaysApply` when the source file carried its own activation
// frontmatter. A catch-all globs (empty, `**/*`, `*`) and an empty
// description carry no scoping intent, so they drop rather than
// round-trip into the source spec as noise, the same choice
// normalizeCursorRuleMeta makes for Cursor's .mdc import. meta is empty
// for a source file with no frontmatter (Cline, Windsurf, Continue,
// and Kilo's plain `# <heading>` rule files), so the output there is
// unchanged from before this function existed.
func rulesDirFileContent(name string, meta map[string]any, body string) string {
	var sb strings.Builder
	sb.WriteString("---\nname: " + name + "\n")
	if desc, ok := meta["description"].(string); ok && desc != "" {
		sb.WriteString(yamlFrontmatterLine("description", desc))
	}
	if globs, ok := meta["globs"].(string); ok && !isCatchAllGlobs(globs) {
		sb.WriteString(yamlFrontmatterLine("globs", globs))
	}
	if always, ok := meta["alwaysApply"].(bool); ok {
		fmt.Fprintf(&sb, "alwaysApply: %t\n", always)
	}
	sb.WriteString("---\n\n")
	sb.WriteString(body)
	content := sb.String()
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content
}

// scopeDir returns the directory portion of rel, normalized so root-level
// files produce "" instead of the "." that filepath.Dir yields.
func scopeDir(rel string) string {
	d := filepath.Dir(rel)
	if d == "." {
		return ""
	}
	return d
}

func classifyRulesDirFile(rel string) (kind, baseName string) {
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

func pickKindDir(kind string, src config.Sources) string {
	switch kind {
	case "agents":
		return src.Agents
	case "skills":
		return src.Skills
	default:
		return src.Rules
	}
}

func stripLeadingHeading(content string) string {
	if !strings.HasPrefix(content, "# ") {
		return content
	}
	nl := strings.IndexByte(content, '\n')
	if nl < 0 {
		return ""
	}
	return strings.TrimLeft(content[nl+1:], "\n")
}

// importFromRulesDir reads an existing rules directory (cline/windsurf/
// continue) and writes specs into the configured source directories.
func importFromRulesDir(root, target, srcDir string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills); err != nil {
		return err
	}
	c, err := importRulesDirectory(root, srcDir, src)
	if err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills (from %s)\n",
		c.rules, c.agents, c.skills, target)
	printImportNextSteps(root, target)
	return nil
}
