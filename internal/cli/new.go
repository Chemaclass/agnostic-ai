package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// newSpecKinds is the canonical list shown in `new --help` and used for
// argument validation. Order matches spec.AllKinds.
var newSpecKinds = []string{
	string(spec.KindAgent),
	string(spec.KindSkill),
	string(spec.KindRule),
	string(spec.KindHook),
	string(spec.KindMCP),
}

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func newNewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new <kind> <name>",
		Short: "Scaffold a single spec file with kind-appropriate frontmatter.",
		Long: "Creates one spec file under the directory configured for <kind> " +
			"in agnostic.config.yaml. Replaces 'copy from --demo and edit' as " +
			"the starting point for a single new agent, skill, rule, hook, or MCP.",
		Example: `  # Add a new rule
  agnostic-ai new rule no-console-log

  # Add a new agent
  agnostic-ai new agent code-reviewer

  # Add a new MCP server config
  agnostic-ai new mcp filesystem`,
		Args: cobra.ExactArgs(2),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return newSpecKinds, cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			kind, name := strings.ToLower(args[0]), args[1]
			if !validKind(kind) {
				return fmt.Errorf("unknown kind %q; expected one of: %s", kind, strings.Join(newSpecKinds, ", "))
			}
			if !slugRe.MatchString(name) {
				return fmt.Errorf("invalid name %q; use lowercase letters, digits, and hyphens (e.g. no-console-log)", name)
			}
			cfg, err := config.Load(".")
			if err != nil {
				return err
			}
			path, err := newSpecPath(cfg, kind, name)
			if err != nil {
				return err
			}
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("%s already exists", path)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
			}
			body := newSpecTemplate(kind, name)
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				return fmt.Errorf("write %s: %w", path, err)
			}
			summaryf("wrote %s\n", path)
			summaryf("→ edit it, then run `agnostic-ai render %s --target <name>` to preview, or `agnostic-ai sync` to fan out.\n", path)
			return nil
		},
	}
	return cmd
}

func validKind(k string) bool {
	for _, v := range newSpecKinds {
		if v == k {
			return true
		}
	}
	return false
}

// newSpecPath resolves the destination file for a `new` invocation by
// joining the source directory configured for that kind with the slug.
// Hooks and MCPs are pure YAML; everything else is Markdown.
func newSpecPath(cfg *config.Config, kind, name string) (string, error) {
	dir, err := sourceDirForKind(cfg, kind)
	if err != nil {
		return "", err
	}
	ext := ".md"
	if kind == string(spec.KindHook) || kind == string(spec.KindMCP) {
		ext = ".yaml"
	}
	return filepath.Join(dir, name+ext), nil
}

func sourceDirForKind(cfg *config.Config, kind string) (string, error) {
	switch kind {
	case string(spec.KindAgent):
		return cfg.Sources.Agents, nil
	case string(spec.KindSkill):
		return cfg.Sources.Skills, nil
	case string(spec.KindRule):
		return cfg.Sources.Rules, nil
	case string(spec.KindHook):
		return cfg.Sources.Hooks, nil
	case string(spec.KindMCP):
		return cfg.Sources.MCPs, nil
	}
	return "", fmt.Errorf("unknown kind %q", kind)
}

// newSpecTemplate returns kind-specific frontmatter pre-filled with the
// canonical fields each adapter consumes. Bodies are intentionally tiny
// so the user replaces them rather than editing around boilerplate.
func newSpecTemplate(kind, name string) string {
	switch kind {
	case string(spec.KindAgent):
		return fmt.Sprintf(`---
name: %s
description: TODO short description shown in agent pickers.
tools: [Read, Grep, Bash]
model: sonnet
---

TODO: agent system prompt body.
`, name)
	case string(spec.KindSkill):
		return fmt.Sprintf(`---
name: %s
description: TODO when this skill should fire (mention the trigger words).
---

# %s

TODO: skill body. Describe steps, inputs, outputs.
`, name, name)
	case string(spec.KindRule):
		return fmt.Sprintf(`---
name: %s
description: TODO short description.
globs: "**/*"
alwaysApply: true
---

TODO: rule body.
`, name)
	case string(spec.KindHook):
		return fmt.Sprintf(`name: %s
description: TODO short description.
event: PostToolUse
matcher: "Edit|Write"
command: "echo TODO"
`, name)
	case string(spec.KindMCP):
		return fmt.Sprintf(`name: %s
description: TODO short description.
command: npx
args:
  - -y
  - "@example/server"
`, name)
	}
	return ""
}
