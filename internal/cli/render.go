package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func newRenderCmd() *cobra.Command {
	var targetFlags []string
	cmd := &cobra.Command{
		Use:   "render <spec>",
		Short: "Print the emission for a single spec to stdout, per target.",
		Long: "Loads one spec by path and prints what each chosen target would " +
			"write, without touching disk. Useful while iterating on a single " +
			"rule, agent, skill, hook, or MCP. <spec> is resolved relative to " +
			"the current directory.",
		Example: `  # Render a single rule for cursor
  agnostic-ai render rules/no-console-log.md --target cursor

  # Multi-target preview
  agnostic-ai render rules/no-console-log.md --target claude,codex

  # Default targets come from agnostic-ai.yaml
  agnostic-ai render rules/no-console-log.md`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, bundle, err := loadProject(".")
			if err != nil {
				return err
			}
			entry, err := findSpecEntry(args[0], bundle)
			if err != nil {
				return err
			}
			targets := targetFlags
			if len(targets) == 0 {
				targets = cfg.Targets
			}
			single := singleEntryBundle(entry)
			out := cmd.OutOrStdout()
			anyOutput := false
			for _, t := range targets {
				adapter, err := adapters.Resolve(t)
				if err != nil {
					return err
				}
				captured, err := captureEmit(adapter, single, cfg)
				if err != nil {
					return fmt.Errorf("%s: %w", t, err)
				}
				if extra, ok := entryPointRuleFile(cfg, t, entry, single); ok {
					captured = append(captured, extra)
				}
				if len(captured) == 0 {
					_, _ = fmt.Fprintf(out, "# target: %s — (no output for kind %s)\n", t, entry.Kind)
					continue
				}
				anyOutput = true
				for _, f := range captured {
					_, _ = fmt.Fprintf(out, "# target: %s — %s\n", t, filepath.ToSlash(f.Path))
					_, _ = fmt.Fprint(out, f.Content)
					if !strings.HasSuffix(f.Content, "\n") {
						_, _ = fmt.Fprintln(out)
					}
					_, _ = fmt.Fprintln(out)
				}
			}
			if !anyOutput {
				return fmt.Errorf("no targets produced output for spec %s", entry.Path)
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVarP(&targetFlags, "target", "t", nil, "Target(s) to render (default: all in config). Repeat or comma-separate.")
	registerTargetCompletion(cmd)
	return cmd
}

// findSpecEntry resolves the user-supplied path to a loaded spec entry.
// Tries direct equality first, then absolute-path equality, then falls
// back to a "did you mean" hint based on file basename.
func findSpecEntry(input string, b spec.Bundle) (spec.Entry, error) {
	all := b.All()
	for _, e := range all {
		if e.Path == input {
			return e, nil
		}
	}
	abs, _ := filepath.Abs(input)
	if abs != "" {
		for _, e := range all {
			ea, _ := filepath.Abs(e.Path)
			if ea == abs {
				return e, nil
			}
		}
	}
	if _, err := os.Stat(input); err == nil {
		return spec.Entry{}, fmt.Errorf("%s exists but is not loaded as a spec; ensure it lives under a configured source directory and uses a recognized extension", input)
	}
	base := filepath.Base(input)
	var hints []string
	for _, e := range all {
		if filepath.Base(e.Path) == base {
			hints = append(hints, e.Path)
		}
	}
	sort.Strings(hints)
	if len(hints) > 0 {
		return spec.Entry{}, fmt.Errorf("spec %q not found. Did you mean: %s", input, strings.Join(hints, ", "))
	}
	return spec.Entry{}, fmt.Errorf("spec %q not found", input)
}

// entryPointRuleFile returns the entry-point contribution a rule spec
// makes for a target that inlines rules into its entry-point file
// (codex, amp, warp, gemini, aider, opencode). The adapter's Emit does
// not produce this output: rule inlining happens in the central sync
// layer, so render and graph reconstruct it here to mirror what `sync`
// actually writes. Returns ok=false for non-rule specs, non-inlining
// targets, and targets on the legacy concatenated rules-file layout
// (where the adapter owns the entry-point write instead).
func entryPointRuleFile(cfg *config.Config, target string, entry spec.Entry, single spec.Bundle) (adapters.CapturedFile, bool) {
	if entry.Kind != spec.KindRule {
		return adapters.CapturedFile{}, false
	}
	path := adapters.EntryPointPath(cfg, target)
	if path == "" {
		return adapters.CapturedFile{}, false
	}
	var appendix string
	switch {
	case adapters.InlinesRulesIntoEntryPoint(target) && !adapters.HasLegacyRulesFile(cfg, target):
		appendix = adapters.RenderRulesAppendix(single)
	case adapters.ImportsRulesIntoEntryPoint(cfg, target):
		appendix = adapters.RenderRulesImportAppendix(cfg, target, single)
	}
	if appendix == "" {
		return adapters.CapturedFile{}, false
	}
	return adapters.CapturedFile{Path: path, Content: appendix}, true
}

// singleEntryBundle returns a Bundle that contains only the given entry,
// dispatched into the correct kind bucket.
func singleEntryBundle(e spec.Entry) spec.Bundle {
	var b spec.Bundle
	switch e.Kind {
	case spec.KindAgent:
		b.Agents = []spec.Entry{e}
	case spec.KindSkill:
		b.Skills = []spec.Entry{e}
	case spec.KindRule:
		b.Rules = []spec.Entry{e}
	case spec.KindHook:
		b.Hooks = []spec.Entry{e}
	case spec.KindMCP:
		b.MCPs = []spec.Entry{e}
	case spec.KindCommand:
		b.Commands = []spec.Entry{e}
	}
	return b
}

// captureEmit runs an adapter's Emit under capture mode so output stays
// in memory instead of writing to disk. dryRun is false because dryRun
// prints to stdout, which would defeat the purpose of capturing.
func captureEmit(a adapters.Adapter, b spec.Bundle, cfg *config.Config) ([]adapters.CapturedFile, error) {
	adapters.StartCapture()
	err := adapters.EmitWithProvenance(a, b, cfg, false)
	captured := adapters.StopCapture()
	if err != nil {
		return nil, err
	}
	return captured, nil
}
