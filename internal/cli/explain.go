package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// contribution describes one downstream output that a single source
// spec contributes to.
type contribution struct {
	Target  string `json:"target"`
	Path    string `json:"path"`
	Section string `json:"section,omitempty"`
	Mode    string `json:"mode"` // "full" or "section"
}

// explainOutput is the JSON envelope for `explain --json`.
type explainOutput struct {
	Version       string         `json:"version"`
	Command       string         `json:"command"`
	Spec          explainSpecRef `json:"spec"`
	Contributions []contribution `json:"contributions"`
	// WouldEmitIfEnabled lists outputs from adapters that are NOT in the
	// project's configured targets. Helps authors anticipate the impact
	// of enabling a new target without having to edit and re-sync.
	WouldEmitIfEnabled []contribution `json:"would_emit_if_enabled"`
}

type explainSpecRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	Path string `json:"path"`
}

func newExplainCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "explain <spec | AAI-NNN>",
		Short: "List every output file and section a spec contributes to, or describe an error code.",
		Long: "Reverse provenance: takes one spec and shows where it lands in " +
			"each target's emission. Pairs with the `<!-- source: ... -->` " +
			"forward markers adapters write into merged documents.\n\n" +
			"When the argument matches an `AAI-NNN` error code, prints the " +
			"code's title, cause, and suggested fix instead.",
		Example: `  # Human-readable
  agnostic-ai explain rules/conventional-commits.md

  # Machine-readable, for editor extensions or scripts
  agnostic-ai explain rules/conventional-commits.md --json

  # Look up an error code
  agnostic-ai explain AAI-001`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if errs.IsCode(args[0]) {
				return runExplainCode(cmd, errs.Code(args[0]), jsonOut)
			}
			cfg, bundle, err := loadProject(".")
			if err != nil {
				return err
			}
			entry, err := findSpecEntry(args[0], bundle)
			if err != nil {
				return err
			}
			configured, extra, err := computeContributions(entry, bundle, cfg)
			if err != nil {
				return err
			}
			if jsonOut {
				if configured == nil {
					configured = []contribution{}
				}
				if extra == nil {
					extra = []contribution{}
				}
				return emitExplainJSON(cmd, explainOutput{
					Version: "1",
					Command: "explain",
					Spec: explainSpecRef{
						Kind: string(entry.Kind),
						Name: entry.Name,
						Path: filepath.ToSlash(entry.Path),
					},
					Contributions:      configured,
					WouldEmitIfEnabled: extra,
				})
			}
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "%s →\n", filepath.ToSlash(entry.Path))
			if len(configured) == 0 {
				_, _ = fmt.Fprintln(out, "  (no configured target emits output for this spec)")
			}
			for _, c := range configured {
				_, _ = fmt.Fprintf(out, "  %s\n", formatContribution(c))
			}
			if len(extra) > 0 {
				_, _ = fmt.Fprintln(out, "")
				_, _ = fmt.Fprintln(out, "would emit if enabled:")
				for _, c := range extra {
					_, _ = fmt.Fprintf(out, "  %s\n", formatContribution(c))
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON for editor extensions and scripts.")
	return cmd
}

func formatContribution(c contribution) string {
	path := filepath.ToSlash(c.Path)
	if c.Mode == "full" {
		return fmt.Sprintf("[%s] %s (full file)", c.Target, path)
	}
	if c.Section != "" {
		return fmt.Sprintf("[%s] %s (section %q)", c.Target, path, c.Section)
	}
	return fmt.Sprintf("[%s] %s (partial)", c.Target, path)
}

// computeContributions classifies each emitted file as either fully
// owned by the spec or as one section among many. Returns two slices:
// contributions from configured targets and contributions from
// not-configured-but-built-in adapters.
//
// Strategy: render every adapter twice — once with the full bundle,
// once with the bundle minus the spec — and compare. If the path
// disappears, the spec owns the file; if the content shrinks, the spec
// contributes a section. Avoids parsing each adapter's output format.
func computeContributions(e spec.Entry, b spec.Bundle, cfg *config.Config) ([]contribution, []contribution, error) {
	// Silence per-adapter capability warnings while we render every
	// adapter twice. Without this, hooks/MCPs would emit "X not
	// supported" lines repeatedly even though they're informational.
	adapters.SetWarner(io.Discard)
	defer adapters.SetWarner(os.Stderr)

	withoutSpec := bundleWithout(b, e)
	configuredSet := make(map[string]struct{}, len(cfg.Targets))
	for _, t := range cfg.Targets {
		configuredSet[t] = struct{}{}
	}

	var configured, extra []contribution
	for _, name := range adapters.Names() {
		adapter, err := adapters.Resolve(name)
		if err != nil {
			continue
		}
		full, err := captureEmit(adapter, b, cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", name, err)
		}
		minus, err := captureEmit(adapter, withoutSpec, cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", name, err)
		}
		minusByPath := make(map[string]string, len(minus))
		for _, f := range minus {
			minusByPath[f.Path] = f.Content
		}
		for _, f := range full {
			before, present := minusByPath[f.Path]
			switch {
			case !present:
				addContribution(&configured, &extra, configuredSet, contribution{
					Target: name, Path: f.Path, Mode: "full",
				})
			case before != f.Content:
				addContribution(&configured, &extra, configuredSet, contribution{
					Target: name, Path: f.Path, Section: e.Name, Mode: "section",
				})
			}
		}
	}
	// Rule bodies inlined into an entry-point file (AGENTS.md, GEMINI.md,
	// CONVENTIONS.md, ...) are written by sync's entry-point distribution,
	// not by any adapter's Emit, so the capture sweep above never sees
	// them. Credit them explicitly.
	if e.Kind == spec.KindRule && ruleReachesAppendix(b, e) {
		for path, tgts := range inlinedRuleEntryPoints(cfg, adapters.Names()) {
			for _, t := range tgts {
				addContribution(&configured, &extra, configuredSet, contribution{
					Target: t, Path: path, Section: e.Name, Mode: "section",
				})
			}
		}
	}

	sortContribs(configured)
	sortContribs(extra)
	return configured, extra, nil
}

// ruleReachesAppendix reports whether removing e changes the inlined rules
// appendix, i.e. the rule contributes a body to entry-point inliners. A
// rule with neither globs nor scope always does; the diff guards against
// a spec somehow absent from the bundle.
func ruleReachesAppendix(b spec.Bundle, e spec.Entry) bool {
	return adapters.RenderRulesAppendix(b) != adapters.RenderRulesAppendix(bundleWithout(b, e))
}

// inlinedRuleEntryPoints maps each entry-point path to the targets that
// inline rule bodies into it (codex/gemini/aider/amp/warp/opencode). A
// shared path like AGENTS.md lists every consumer. Targets on the legacy
// rules-file layout are skipped: the adapter owns that write, so the
// capture sweep already credits it.
func inlinedRuleEntryPoints(cfg *config.Config, targets []string) map[string][]string {
	out := map[string][]string{}
	for _, t := range targets {
		if !adapters.InlinesRulesIntoEntryPoint(t) || adapters.HasLegacyRulesFile(cfg, t) {
			continue
		}
		p := adapters.EntryPointPath(cfg, t)
		if p == "" || p == adapters.AgnosticEntryPointPath {
			continue
		}
		out[p] = append(out[p], t)
	}
	return out
}

func addContribution(configured, extra *[]contribution, configuredSet map[string]struct{}, c contribution) {
	if _, ok := configuredSet[c.Target]; ok {
		*configured = append(*configured, c)
		return
	}
	*extra = append(*extra, c)
}

func sortContribs(c []contribution) {
	sort.SliceStable(c, func(i, j int) bool {
		if c[i].Target != c[j].Target {
			return c[i].Target < c[j].Target
		}
		return c[i].Path < c[j].Path
	})
}

// bundleWithout returns a copy of b with the given entry removed.
// Identity is by Path, which is unique within a bundle.
func bundleWithout(b spec.Bundle, e spec.Entry) spec.Bundle {
	return spec.Bundle{
		Agents:       filterEntries(b.Agents, e.Path),
		Skills:       filterEntries(b.Skills, e.Path),
		Rules:        filterEntries(b.Rules, e.Path),
		Hooks:        filterEntries(b.Hooks, e.Path),
		MCPs:         filterEntries(b.MCPs, e.Path),
		Commands:     filterEntries(b.Commands, e.Path),
		Settings:     filterEntries(b.Settings, e.Path),
		Reviews:      filterEntries(b.Reviews, e.Path),
		Environments: filterEntries(b.Environments, e.Path),
	}
}

func filterEntries(entries []spec.Entry, excludePath string) []spec.Entry {
	out := make([]spec.Entry, 0, len(entries))
	for _, e := range entries {
		if e.Path == excludePath {
			continue
		}
		out = append(out, e)
	}
	return out
}

// explainCodeOutput is the JSON envelope for `explain <AAI-NNN>`.
type explainCodeOutput struct {
	Version string `json:"version"`
	Command string `json:"command"`
	Code    string `json:"code"`
	Title   string `json:"title"`
	Cause   string `json:"cause"`
	Fix     string `json:"fix"`
}

// runExplainCode prints the registry entry for an error code, or
// errors when the code is not registered.
func runExplainCode(cmd *cobra.Command, code errs.Code, jsonOut bool) error {
	entry, ok := errs.Lookup(code)
	if !ok {
		return fmt.Errorf("unknown error code: %s (see docs/user/errors.md for the canonical list)", code)
	}
	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(explainCodeOutput{
			Version: "1",
			Command: "explain",
			Code:    string(entry.Code),
			Title:   entry.Title,
			Cause:   entry.Cause,
			Fix:     entry.Fix,
		})
	}
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "%s: %s\n", entry.Code, entry.Title)
	_, _ = fmt.Fprintf(out, "\nCause:\n  %s\n", entry.Cause)
	_, _ = fmt.Fprintf(out, "\nFix:\n  %s\n", entry.Fix)
	return nil
}

// emitExplainJSON serializes an explainOutput with stable indentation.
// `explain` carries a different envelope than sync/revert/doctor (which
// share jsonOutput), so it gets its own emitter.
func emitExplainJSON(cmd *cobra.Command, v explainOutput) error {
	for i := range v.Contributions {
		v.Contributions[i].Path = filepath.ToSlash(v.Contributions[i].Path)
	}
	for i := range v.WouldEmitIfEnabled {
		v.WouldEmitIfEnabled[i].Path = filepath.ToSlash(v.WouldEmitIfEnabled[i].Path)
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
