package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
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
	var scope, dir string
	cmd := &cobra.Command{
		Use:   "new <kind> <name>",
		Short: "Create a spec file for any supported kind.",
		Long: "Creates one spec file under the directory configured for <kind> " +
			"in agnostic-ai.yaml. Supported kinds: " + strings.Join(newSpecKinds, ", ") +
			". Pass --dry-run to preview the path and content without writing. " +
			"--scope local writes under .agnostic-ai/local/ instead; when the project already has " +
			"that kind and name, the file starts with the name only, since local fields merge into " +
			"the shared spec. --scope user writes to the user home from any directory and takes " +
			"the kinds sync --scope user loads: " + strings.Join(userScopeKinds(), ", ") + ".",
		Example: `  # Add a new rule
  agnostic-ai new rule no-console-log

  # Preview the scaffold without writing it
  agnostic-ai new rule no-console-log --dry-run

  # Add a new agent
  agnostic-ai new agent code-reviewer

  # Add a new MCP server config
  agnostic-ai new mcp filesystem

  # Preview a settings spec
  agnostic-ai new settings project-defaults --dry-run

  # Add a rule only you get in this project, ignored by Git
  agnostic-ai new rule scratch-notes --scope local

  # Change a shared skill for yourself: starts with its name only
  agnostic-ai new skill review --scope local

  # Add a rule to every project of yours
  agnostic-ai new rule my-style --scope user`,
		Args: cobra.ExactArgs(2),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return newSpecKinds, cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			kind, name := strings.ToLower(args[0]), args[1]
			dirSet := cmd.Flags().Changed("dir")
			if cmd.Flags().Changed(scopeFlagName) {
				switch scope {
				case specScopeProject, specScopeUser, specScopeLocal:
				default:
					// Before --scope meant where a spec lives, it named
					// a rule's directory here.
					if dirSet {
						return errs.Coded(errs.CodeFlagConflict, "--scope %s is the older spelling of --dir and cannot be combined with it", scope)
					}
					if verbosity >= levelDefault {
						cmd.PrintErrf("warning: --scope for a rule directory is deprecated; use --dir %s\n", scope)
					}
					dir, dirSet, scope = scope, true, specScopeProject
				}
			}
			if dirSet && scope == specScopeUser {
				return errs.Coded(errs.CodeFlagConflict, "--dir cannot be combined with --scope %s: user-level rules apply everywhere", specScopeUser)
			}
			if dirSet {
				if kind != "rule" {
					return errors.New("--dir is supported only for rules")
				}
				var err error
				dir, err = spec.NormalizeScope(dir)
				if err != nil {
					return err
				}
				if dir == "" {
					return errors.New("--dir requires a project-relative subdirectory")
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
			place, err := resolveNewPlace(scope, kind, name)
			if err != nil {
				return err
			}
			path, cfg := place.path, place.cfg
			body := newSpecTemplate(kind, name)
			if place.scope == specScopeUser {
				// A user-level rule applies everywhere; sync --scope user
				// rejects one with a condition.
				body = strings.Replace(body, "globs: \"**/*\"\nalwaysApply: true\n", "", 1)
			}
			if place.override {
				if dir != "" {
					return errs.Coded(errs.CodeFlagConflict, "--dir cannot be combined with a local override of an existing %s; a local file applies to the whole project", kind)
				}
				body = newOverrideTemplate(path, name)
			}
			if dir != "" {
				body = strings.Replace(body, "globs: \"**/*\"\nalwaysApply: true", yamlFrontmatterLine("scope", dir), 1)
			}
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("%s already exists", path)
			}
			if dryRun {
				summaryf("would create %s\n\n%s", path, body)
				return nil
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
			}
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				return fmt.Errorf("write %s: %w", path, err)
			}
			summaryf("wrote %s\n", path)
			switch {
			case place.scope == specScopeUser:
				summaryf("→ edit it, then run `agnostic-ai sync --scope user --dry-run` to preview, or `agnostic-ai sync --scope user` to install it.\n")
			case place.override:
				summaryf("→ add the fields you want to change, then run `agnostic-ai sync`. The shared %s keeps the rest.\n", kind)
			default:
				summaryf("→ edit it, then run `agnostic-ai render %s --target <name>` to preview, or `agnostic-ai sync` to fan out.\n", newRenderPathArg(path))
			}
			if kind == string(spec.KindMCP) && !place.override {
				summaryf("→ real servers to copy: https://agnostic-ai.org/docs/spec-format/mcp-recipes/\n")
			}
			if place.scope == specScopeLocal {
				if warning := localIgnoreWarning("."); warning != "" && verbosity >= levelDefault {
					cmd.PrintErrf("warning: %s\n", warning)
				}
			}
			if cfg == nil {
				return nil
			}
			support := projectKindSupport(cfg)
			if readers := support[spec.Kind(kind)]; len(readers) > 0 && len(cfg.Targets) > 0 && !anyTargetSupports(spec.Kind(kind), setOf(cfg.Targets...), support) {
				summaryf("→ no enabled target reads %s specs; enable one of: %s\n", kind, commaList(sortedKeys(readers)))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Project-relative directory for a rule and its descendants.")
	// Any other value is the older spelling of --dir.
	cmd.Flags().StringVar(&scope, scopeFlagName, specScopeProject, "Where the spec lives: project, shared with the team; local, yours in this project and ignored by Git; or user, yours in every project ($AGNOSTIC_AI_HOME, default ~/.agnostic-ai).")
	_ = cmd.RegisterFlagCompletionFunc(scopeFlagName, func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{specScopeProject, specScopeLocal, specScopeUser}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().BoolVar(&dryRun, "dry-run", false,
		"Print the spec that would be scaffolded (path plus rendered frontmatter and body) without writing it.")
	return cmd
}

func validKind(k string) bool {
	return slices.Contains(newSpecKinds, k)
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
description: TODO when to hand a task to this agent (say "use proactively" to encourage delegation).
can: [read, Grep, shell]
---

TODO: agent system prompt body. Put detail here; it loads only when the agent runs.
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
