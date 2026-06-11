package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// graphEdge is one spec → target → file relationship.
type graphEdge struct {
	Spec   string `json:"spec"`
	Kind   string `json:"kind"`
	Target string `json:"target"`
	Path   string `json:"path"`
}

const (
	graphFormatText    = "text"
	graphFormatMermaid = "mermaid"
	graphFormatDOT     = "dot"
	graphFormatJSON    = "json"
)

func newGraphCmd() *cobra.Command {
	var (
		format       string
		targetFilter string
		specFilter   string
		kindFilter   string
	)
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Render the spec → target → file dependency graph.",
		Long: "Walks the loaded spec bundle, asks every configured target adapter " +
			"which files each spec would produce, and prints the resulting graph. " +
			"Read-only: no Emit side effects, no disk writes.",
		Example: `  # Default aligned matrix
  agnostic-ai graph

  # Mermaid for embedding in docs
  agnostic-ai graph --format mermaid

  # Graphviz for rendering with dot(1)
  agnostic-ai graph --format dot | dot -Tsvg > graph.svg

  # Machine-readable
  agnostic-ai graph --format json

  # Narrow the view
  agnostic-ai graph --target claude
  agnostic-ai graph --spec no-console-log
  agnostic-ai graph --kind rule`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateGraphFormat(format); err != nil {
				return err
			}
			cfg, bundle, err := loadProject(".")
			if err != nil {
				return err
			}
			edges, err := computeGraphEdges(bundle, cfg)
			if err != nil {
				return err
			}
			edges = filterGraphEdges(edges, specFilter, targetFilter, kindFilter)
			sortGraphEdges(edges)
			return renderGraph(cmd.OutOrStdout(), format, edges, cfg.Targets, specFilter, targetFilter, kindFilter)
		},
	}
	cmd.Flags().StringVar(&format, "format", graphFormatText, "Output format: text, mermaid, dot, json.")
	cmd.Flags().StringVar(&targetFilter, "target", "", "Restrict to one target.")
	cmd.Flags().StringVar(&specFilter, "spec", "", "Restrict to one spec name.")
	cmd.Flags().StringVar(&kindFilter, "kind", "", "Restrict to one spec kind (agent, skill, rule, hook, mcp, command).")
	registerTargetCompletion(cmd)
	return cmd
}

func validateGraphFormat(f string) error {
	switch f {
	case graphFormatText, graphFormatMermaid, graphFormatDOT, graphFormatJSON:
		return nil
	}
	return fmt.Errorf("unknown --format %q (want text, mermaid, dot, or json)", f)
}

// computeGraphEdges asks every configured target which files each spec
// in the bundle would produce. Strategy mirrors `explain`: render each
// adapter once per spec under capture mode, never touching disk.
func computeGraphEdges(b spec.Bundle, cfg *config.Config) ([]graphEdge, error) {
	// Suppress per-adapter "X not supported" warnings; the matrix
	// already conveys what each target emits.
	adapters.SetWarner(io.Discard)
	defer adapters.SetWarner(os.Stderr)

	targets := cfg.Targets
	if len(targets) == 0 {
		targets = adapters.Names()
	}

	var edges []graphEdge
	for _, t := range targets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			// Skip unknown targets in the user's config rather than failing
			// the whole graph; an unknown target is a config issue better
			// surfaced by `validate`.
			continue
		}
		for _, e := range b.All() {
			single := singleEntryBundle(e)
			captured, err := captureEmit(adapter, single, cfg)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", t, err)
			}
			if extra, ok := entryPointRuleFile(cfg, t, e, single); ok {
				captured = append(captured, extra)
			}
			seen := make(map[string]struct{}, len(captured))
			for _, f := range captured {
				p := filepath.ToSlash(f.Path)
				if _, dup := seen[p]; dup {
					continue
				}
				seen[p] = struct{}{}
				edges = append(edges, graphEdge{
					Spec:   e.Name,
					Kind:   string(e.Kind),
					Target: t,
					Path:   p,
				})
			}
		}
	}
	return edges, nil
}

func filterGraphEdges(edges []graphEdge, specFilter, targetFilter, kindFilter string) []graphEdge {
	if specFilter == "" && targetFilter == "" && kindFilter == "" {
		return edges
	}
	out := make([]graphEdge, 0, len(edges))
	for _, e := range edges {
		if specFilter != "" && e.Spec != specFilter {
			continue
		}
		if targetFilter != "" && e.Target != targetFilter {
			continue
		}
		if kindFilter != "" && e.Kind != kindFilter {
			continue
		}
		out = append(out, e)
	}
	return out
}

func sortGraphEdges(edges []graphEdge) {
	sort.SliceStable(edges, func(i, j int) bool {
		if edges[i].Spec != edges[j].Spec {
			return edges[i].Spec < edges[j].Spec
		}
		if edges[i].Target != edges[j].Target {
			return edges[i].Target < edges[j].Target
		}
		return edges[i].Path < edges[j].Path
	})
}

func renderGraph(w io.Writer, format string, edges []graphEdge, configuredTargets []string, specFilter, targetFilter, kindFilter string) error {
	switch format {
	case graphFormatJSON:
		return renderGraphJSON(w, edges)
	case graphFormatMermaid:
		return renderGraphMermaid(w, edges)
	case graphFormatDOT:
		return renderGraphDOT(w, edges)
	default:
		return renderGraphText(w, edges, configuredTargets, specFilter, targetFilter, kindFilter)
	}
}

func renderGraphJSON(w io.Writer, edges []graphEdge) error {
	if edges == nil {
		edges = []graphEdge{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(edges)
}

// renderGraphMermaid prints a Mermaid LR graph. Each edge becomes
//
//	S_<spec>["spec"] --> T_<target>["target"] --> F_<n>["path"]
//
// Node IDs are sanitized; labels keep the original text.
func renderGraphMermaid(w io.Writer, edges []graphEdge) error {
	if _, err := fmt.Fprintln(w, "graph LR"); err != nil {
		return err
	}
	if len(edges) == 0 {
		return nil
	}
	pathID := make(map[string]string, len(edges))
	nextID := 0
	for _, e := range edges {
		sNode := "S_" + sanitizeMermaidID(e.Spec)
		tNode := "T_" + sanitizeMermaidID(e.Target)
		id, ok := pathID[e.Path]
		if !ok {
			id = fmt.Sprintf("F_%d", nextID)
			nextID++
			pathID[e.Path] = id
		}
		if _, err := fmt.Fprintf(w, "  %s[%q] --> %s[%q] --> %s[%q]\n",
			sNode, e.Spec, tNode, e.Target, id, e.Path); err != nil {
			return err
		}
	}
	return nil
}

// renderGraphDOT prints a graphviz directed graph. No external deps;
// just plain text the user can pipe to `dot`.
func renderGraphDOT(w io.Writer, edges []graphEdge) error {
	if _, err := fmt.Fprintln(w, "digraph agnostic_ai {"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "  rankdir=LR;"); err != nil {
		return err
	}
	for _, e := range edges {
		if _, err := fmt.Fprintf(w, "  %q -> %q -> %q;\n", e.Spec, e.Target, e.Path); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "}"); err != nil {
		return err
	}
	return nil
}

// renderGraphText prints an aligned matrix:
//
//	spec | target1 target2 ...
//	name | <kind>  -        ...
//
// A cell holds the spec kind when that target emits the spec, or `-`
// when nothing was produced.
func renderGraphText(w io.Writer, edges []graphEdge, configuredTargets []string, specFilter, targetFilter, kindFilter string) error {
	specs, targets := graphAxes(edges, configuredTargets, specFilter, targetFilter)
	if len(specs) == 0 || len(targets) == 0 {
		_, err := fmt.Fprintln(w, "(no spec → target edges to display)")
		return err
	}

	// kind-by-(spec,target) lookup. A spec has one kind, so the matrix
	// cell is either that kind or `-`.
	cell := make(map[string]map[string]string, len(specs))
	for _, e := range edges {
		row, ok := cell[e.Spec]
		if !ok {
			row = make(map[string]string, len(targets))
			cell[e.Spec] = row
		}
		row[e.Target] = e.Kind
	}

	specCol := "spec"
	specWidth := len(specCol)
	for _, s := range specs {
		if len(s) > specWidth {
			specWidth = len(s)
		}
	}
	colWidth := make([]int, len(targets))
	for i, t := range targets {
		colWidth[i] = len(t)
	}
	for _, s := range specs {
		row := cell[s]
		for i, t := range targets {
			v := row[t]
			if v == "" {
				v = "-"
			}
			if kindFilter != "" && v != kindFilter {
				v = "-"
			}
			if len(v) > colWidth[i] {
				colWidth[i] = len(v)
			}
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%-*s |", specWidth, specCol)
	for i, t := range targets {
		fmt.Fprintf(&b, " %-*s", colWidth[i], t)
	}
	b.WriteByte('\n')
	for _, s := range specs {
		row := cell[s]
		fmt.Fprintf(&b, "%-*s |", specWidth, s)
		for i, t := range targets {
			v := row[t]
			if v == "" {
				v = "-"
			}
			if kindFilter != "" && v != kindFilter {
				v = "-"
			}
			fmt.Fprintf(&b, " %-*s", colWidth[i], v)
		}
		b.WriteByte('\n')
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// graphAxes derives the spec rows and target columns for the matrix.
// Targets honor the user's configured order so the matrix matches the
// project layout; specs are sorted alphabetically. Filters narrow each
// axis so a `--target X` view does not show empty columns for the rest.
func graphAxes(edges []graphEdge, configuredTargets []string, specFilter, targetFilter string) (specs []string, targets []string) {
	specSet := make(map[string]struct{})
	targetSet := make(map[string]struct{})
	for _, e := range edges {
		specSet[e.Spec] = struct{}{}
		targetSet[e.Target] = struct{}{}
	}
	for s := range specSet {
		if specFilter != "" && s != specFilter {
			continue
		}
		specs = append(specs, s)
	}
	sort.Strings(specs)
	for _, t := range configuredTargets {
		if _, ok := targetSet[t]; !ok {
			continue
		}
		if targetFilter != "" && t != targetFilter {
			continue
		}
		targets = append(targets, t)
	}
	return specs, targets
}

// sanitizeMermaidID returns a Mermaid-safe node ID for s. Replaces any
// character outside [A-Za-z0-9_] with `_`. Empty input maps to `_`.
func sanitizeMermaidID(s string) string {
	if s == "" {
		return "_"
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
