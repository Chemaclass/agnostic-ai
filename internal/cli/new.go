package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/suggest"
)

// newSpecKinds is the canonical list shown in `new --help` and used for
// argument validation. Order matches spec.AllKinds.
var newSpecKinds = []string{
	string(spec.KindAgent),
	string(spec.KindSkill),
	string(spec.KindRule),
	string(spec.KindHook),
	string(spec.KindMCP),
	string(spec.KindCommand),
	string(spec.KindSettings),
	string(spec.KindReview),
	string(spec.KindEnvironment),
	string(spec.KindIgnore),
}

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func newNewCmd() *cobra.Command {
	var dryRun bool
	var scope string
	cmd := &cobra.Command{
		Use:   "new <kind> <name>",
		Short: "Create a spec file for any supported kind.",
		Long: "Creates one spec file under the directory configured for <kind> " +
			"in agnostic-ai.yaml. Supported kinds: " + strings.Join(newSpecKinds, ", ") +
			". Pass --dry-run to preview the path and content without writing.",
		Example: `  # Add a new rule
  agnostic-ai new rule no-console-log

  # Preview the scaffold without writing it
  agnostic-ai new rule no-console-log --dry-run

  # Add a new agent
  agnostic-ai new agent code-reviewer

  # Add a new MCP server config
  agnostic-ai new mcp filesystem

  # Preview a settings spec
  agnostic-ai new settings project-defaults --dry-run`,
		Args: cobra.ExactArgs(2),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return newSpecKinds, cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := refuseGlobalHome(".", globalHomeSpecsRemedy); err != nil {
				return err
			}
			kind, name := strings.ToLower(args[0]), args[1]
			if cmd.Flags().Changed("scope") {
				if kind != "rule" {
					return fmt.Errorf("--scope is supported only for rules")
				}
				var err error
				scope, err = spec.NormalizeScope(scope)
				if err != nil {
					return err
				}
				if scope == "" {
					return fmt.Errorf("--scope requires a project-relative subdirectory")
				}
			}
			if !validKind(kind) {
				if s := suggest.Name(kind, newSpecKinds); s != "" {
					return fmt.Errorf("unknown kind %q (did you mean %s?); expected one of: %s", kind, s, strings.Join(newSpecKinds, ", "))
				}
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
			body := newSpecTemplate(kind, name)
			if scope != "" {
				body = strings.Replace(body, "globs: \"**/*\"\nalwaysApply: true", yamlFrontmatterLine("scope", scope), 1)
			}
			if dryRun {
				summaryf("would create %s\n\n%s", path, body)
				return nil
			}
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("%s already exists", path)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
			}
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				return fmt.Errorf("write %s: %w", path, err)
			}
			summaryf("wrote %s\n", path)
			summaryf("→ edit it, then run `agnostic-ai render %s --target <name>` to preview, or `agnostic-ai sync` to fan out.\n", newRenderPathArg(path))
			if kind == string(spec.KindMCP) {
				summaryf("→ real servers to copy: https://agnostic-ai.org/docs/spec-format/mcp-recipes/\n")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "", "Project-relative directory for a rule and its descendants.")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false,
		"Print the spec that would be scaffolded (path plus rendered frontmatter and body) without writing it.")
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
// Hooks, MCPs, settings, and environments use YAML; other kinds use Markdown.
func newSpecPath(cfg *config.Config, kind, name string) (string, error) {
	dir, err := sourceDirForKind(cfg, kind)
	if err != nil {
		return "", err
	}
	ext := ".md"
	if kind == string(spec.KindHook) || kind == string(spec.KindMCP) ||
		kind == string(spec.KindSettings) || kind == string(spec.KindEnvironment) {
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
	case string(spec.KindCommand):
		return cfg.Sources.Commands, nil
	case string(spec.KindSettings):
		return cfg.Sources.Settings, nil
	case string(spec.KindReview):
		return cfg.Sources.Reviews, nil
	case string(spec.KindEnvironment):
		return cfg.Sources.Environments, nil
	case string(spec.KindIgnore):
		return cfg.Sources.Ignore, nil
	}
	return "", fmt.Errorf("unknown kind %q", kind)
}

func newSpecTemplate(kind, name string) string {
	switch kind {
	case string(spec.KindAgent):
		return fmt.Sprintf(`---
name: %s
description: TODO short description shown in agent pickers.
can: [read, Grep, shell]
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
	case string(spec.KindCommand):
		return fmt.Sprintf(`---
name: %s
description: TODO describe when to run this command.
---

TODO: Write the prompt this command should send.
`, name)
	case string(spec.KindSettings):
		return fmt.Sprintf(`name: %s
description: TODO describe this settings group.
# TODO add the permission rules or model settings your project needs.
`, name)
	case string(spec.KindReview):
		return fmt.Sprintf(`---
name: %s
description: TODO describe what the review should check.
---

TODO: Write the checks the reviewer should apply.
`, name)
	case string(spec.KindEnvironment):
		return fmt.Sprintf(`name: %s
description: TODO describe this environment.
# TODO add reviewed setup, install, or dev server commands when needed.
`, name)
	case string(spec.KindIgnore):
		return fmt.Sprintf("---\nname: %s\ndescription: TODO describe the paths to exclude.\n---\n\n```gitignore\n# TODO add paths to exclude, one per line.\n```\n", name)
	}
	return ""
}

func newRenderPathArg(path string) string {
	if strings.HasPrefix(path, "-") {
		path = "./" + path
	}
	if strings.IndexFunc(path, func(r rune) bool {
		plain := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_./:-", r) || runtime.GOOS == "windows" && r == '\\'
		return !plain
	}) < 0 {
		return path
	}
	if runtime.GOOS == "windows" {
		return "'" + strings.ReplaceAll(path, "'", "''") + "'"
	}
	return adapters.ShellQuote(path)
}
