package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// viewCommands builds the subcommand of each view. A view kept in its
// own file adds its constructor here from an init function.
var viewCommands = []func() *cobra.Command{newViewAuthorityCmd, newViewOverviewCmd}

// newViewCmd is the parent of the read-only views.
func newViewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "view",
		Short: "Show a read-only view of the configuration.",
		Long:  "Builds a view from the loaded specs without writing anything. Each subcommand is one view.",
	}
	for _, sub := range viewCommands {
		cmd.AddCommand(sub())
	}
	return cmd
}

func newViewAuthorityCmd() *cobra.Command {
	var (
		agent, target, dir           string
		jsonOut, global, noInstalled bool
	)
	cmd := &cobra.Command{
		Use:   "authority",
		Short: "Show which agent may do what on which resource, per coding tool.",
		Long: "Resolves every operation the specs name for one subject (an agent, or the tool " +
			"session with no agent) on one coding tool: the declared decision, what the tool " +
			"does with it, whether the generated file is on disk, and every uncertainty. " +
			"Runtime enforcement is never observed. Writes nothing.",
		Example: `  # The tool session on the first configured target
  agnostic-ai view authority

  # One agent on Codex, as JSON
  agnostic-ai view authority --agent reviewer --target codex --json

  # Another project, or the user-level configuration
  agnostic-ai view authority --dir ../service
  agnostic-ai view authority --global`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if global && cmd.Flags().Changed("dir") {
				return errs.Coded(errs.CodeFlagConflict, "--global and --dir cannot be combined")
			}
			src, err := openAuthoritySource(dir, global)
			if err != nil {
				return err
			}
			defer src.close()
			v, err := src.view(agent, target, !noInstalled, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				enc.SetEscapeHTML(false)
				return enc.Encode(v)
			}
			return printAuthorityView(cmd.OutOrStdout(), v)
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "", "Agent to resolve the view for (default: the tool session, with no agent).")
	cmd.Flags().StringVar(&target, "target", "", "Coding tool to resolve effects for (default: the first configured target).")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print the view as JSON.")
	cmd.Flags().StringVar(&dir, "dir", ".", "Project directory to read.")
	cmd.Flags().BoolVar(&global, "global", false, "Read the user-level configuration sync --global uses, in $AGNOSTIC_AI_HOME (default ~/.agnostic-ai).")
	cmd.Flags().BoolVar(&noInstalled, "no-installed", false, "Skip comparing the generated files with disk.")
	registerTargetCompletion(cmd)
	return cmd
}

// authoritySource is what the Authority view reads: a project, entered
// so that paths read as sync reads them, or the user-level
// configuration.
type authoritySource struct {
	root   string
	global bool
	// close returns to the directory the command started in.
	close func()
}

// openAuthoritySource enters dir for a project, since sync and the
// generated files it compares name paths from the project root.
func openAuthoritySource(dir string, global bool) (*authoritySource, error) {
	if global {
		root, err := globalSourceRoot()
		if err != nil {
			return nil, err
		}
		return &authoritySource{root: root, global: true, close: func() {}}, nil
	}
	src := &authoritySource{root: ".", close: func() {}}
	if dir == "" || filepath.Clean(dir) == "." {
		return src, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolve working directory: %w", err)
	}
	if err := os.Chdir(dir); err != nil {
		return nil, fmt.Errorf("enter %s: %w", dir, err)
	}
	src.close = func() { _ = os.Chdir(cwd) }
	return src, nil
}

// load reads the configuration and specs. The global configuration has
// no project config: its targets come from the home config, or are
// every target sync --global writes.
func (s *authoritySource) load() (*config.Config, spec.Bundle, error) {
	if !s.global {
		return loadProject(s.root)
	}
	scope, err := loadCheckScope(true)
	if err != nil {
		return nil, spec.Bundle{}, err
	}
	return &config.Config{Targets: scope.targets}, scope.bundle, nil
}

// configFiles lists the config files the source reads, relative to its
// root.
func (s *authoritySource) configFiles() []string {
	var out []string
	if s.global {
		for _, path := range globalConfigPaths(s.root) {
			if _, err := os.Stat(path); err == nil {
				if rel, err := filepath.Rel(s.root, path); err == nil {
					out = append(out, filepath.ToSlash(rel))
				}
			}
		}
		return out
	}
	if path, _, err := config.ResolveConfigPath(s.root); err == nil {
		out = append(out, filepath.Base(path))
	}
	if _, err := os.Stat(filepath.Join(s.root, config.LocalOverrideFileName)); err == nil {
		out = append(out, config.LocalOverrideFileName)
	}
	return out
}

// options returns the view options for the source. The installed
// comparison covers the files a project sync writes; sync --global
// writes user-level files this view does not compare.
func (s *authoritySource) options(agent, target string) authority.Options {
	return authority.Options{
		Root:        s.root,
		AgentID:     agent,
		Target:      target,
		FileMatches: !s.global,
		ConfigFiles: s.configFiles(),
		Global:      s.global,
	}
}

func (s *authoritySource) view(agent, target string, compare bool, warn io.Writer) (*authority.View, error) {
	cfg, b, err := s.load()
	if err != nil {
		return nil, err
	}
	opts := s.options(agent, target)
	if compare && !s.global {
		resolved := target
		if resolved == "" && len(cfg.Targets) > 0 {
			resolved = cfg.Targets[0]
		}
		if slices.Contains(cfg.Targets, resolved) {
			state, err := installedState(resolved, cfg, b, s.root)
			if err != nil {
				_, _ = fmt.Fprintf(warn, "! could not compare the generated files for %s with disk: %v\n", resolved, err)
			} else {
				opts.Installed = map[string]authority.InstalledState{resolved: state}
			}
		}
	}
	return authority.Build(cfg, b, opts)
}

// effectWords words an effect in the text view.
var effectWords = map[authority.Effect]string{
	authority.EffectAllowed:     "permitted",
	authority.EffectAsk:         "permitted with approval",
	authority.EffectDenied:      "blocked",
	authority.EffectUnavailable: "unavailable",
	authority.EffectDefault:     "tool default",
	authority.EffectUnresolved:  "unresolved",
}

// printAuthorityView writes a readable summary of the view.
func printAuthorityView(w io.Writer, v *authority.View) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Authority: %s on %s\n", v.Agent.Label, v.Harness.Label)
	fmt.Fprintf(&b, "Project: %s · revision %s\n", v.Snapshot.Project.Name, v.Snapshot.Revision)
	fmt.Fprintf(&b, "Harness: %s (%s) · permission support: %s · installed: %s\n", v.Harness.Label, v.Harness.Target, v.Harness.PermissionSupport, v.Harness.Installed.State)
	if v.Harness.Note != "" {
		fmt.Fprintf(&b, "  %s\n", v.Harness.Note)
	}
	fmt.Fprintf(&b, "Subject: %s (%s)%s\n", v.Agent.Label, v.Agent.Kind, subjectScope(v.Agent))

	providers := map[string]authority.Provider{}
	for _, p := range v.Providers {
		providers[p.ID] = p
	}
	b.WriteString("\nDomains\n")
	for _, d := range v.Domains {
		fmt.Fprintf(&b, "  %s: %s, %s%s\n", d.Label, countOf(len(d.ProviderIDs), "provider"), countOf(len(d.ResourceIDs), "resource"), effectCounts(d.Summary))
	}
	b.WriteString("\nResources\n")
	for _, r := range v.Resources {
		fmt.Fprintf(&b, "  %s · %s (%s)\n", providers[r.ProviderID].Label, r.Label, r.Kind)
		for _, op := range r.Operations {
			fmt.Fprintf(&b, "    %s\n", operationLine(op, v.Harness.Label))
		}
	}
	if len(v.Secrets) > 0 {
		b.WriteString("\nSecrets\n")
		for _, s := range v.Secrets {
			fmt.Fprintf(&b, "  %s: discover %s · use %s · read value %s · manage %s\n", s.Label, s.Rights.Discover, s.Rights.Use, s.Rights.ReadValue, s.Rights.Manage)
		}
	}
	b.WriteString("\nUncertainties\n")
	for _, u := range v.Uncertainties {
		fmt.Fprintf(&b, "  - [%s] %s\n", u.Severity, u.Message)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func subjectScope(s authority.Subject) string {
	var parts []string
	if s.ToolScope.Mode == "listed" {
		parts = append(parts, s.ToolScope.Field+": "+strings.Join(s.ToolScope.Declared, ", "))
	}
	if s.Readonly {
		parts = append(parts, "readonly")
	}
	if s.MCPServers.Mode == "listed" {
		parts = append(parts, "mcpServers: "+strings.Join(s.MCPServers.Names, ", "))
	}
	if len(parts) == 0 {
		return ""
	}
	return " · " + strings.Join(parts, " · ")
}

// operationLine reads like "Deploy: permitted (allow mcp:railway/deploy
// · settings/permissions.yaml) · Claude Code: native rule
// mcp__railway__deploy · installed: in-sync · runtime: not observed".
func operationLine(op authority.Operation, harness string) string {
	effect := effectWords[op.Effect]
	if op.Hypothetical {
		effect = "would be " + effect
	}
	return fmt.Sprintf("%s: %s (%s) · %s: %s · installed: %s · runtime: not observed",
		op.Label, effect, decidedBy(op), harness, carriedBy(op), op.Installed)
}

func decidedBy(op authority.Operation) string {
	switch {
	case op.Resolution == authority.ResolutionDisabled || op.Decided == nil && op.Declared == authority.EffectUnavailable:
		return "server disabled"
	case op.Decided == nil:
		return "no rule"
	}
	path := strings.TrimPrefix(op.Decided.Source.Path, config.SourceBaseDir+"/")
	if op.Decided.Scope == "agent" {
		return op.Decided.List + " · " + path
	}
	return op.Decided.List + " " + op.Decided.Rule + " · " + path
}

func carriedBy(op authority.Operation) string {
	native := strings.Join(op.Native, ", ")
	switch op.Resolution {
	case authority.ResolutionTranslated:
		return "native rule " + native
	case authority.ResolutionTranslatedUnverified:
		return "rule written, matching unverified: " + native
	case authority.ResolutionHook:
		return "hook " + native
	case authority.ResolutionNotExpressed:
		return "not expressed"
	case authority.ResolutionAgentScope:
		if op.Effect == authority.EffectUnavailable {
			return "agent scope"
		}
		return "agent scope not carried"
	case authority.ResolutionDisabled:
		return "server disabled"
	}
	return "no rule"
}

func effectCounts(c authority.EffectCounts) string {
	var parts []string
	for _, item := range []struct {
		n     int
		label string
	}{{c.Allowed, "permitted"}, {c.Ask, "ask"}, {c.Denied, "blocked"}, {c.Unavailable, "unavailable"}, {c.Default, "default"}, {c.Unresolved, "unresolved"}} {
		if item.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", item.n, item.label))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return " · " + strings.Join(parts, ", ")
}

func countOf(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
