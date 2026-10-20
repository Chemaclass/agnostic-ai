package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

type explainLayerSource struct {
	Source    specSourceRef `json:"source"`
	Operation string        `json:"operation"`
	Role      string        `json:"role"`
}

type explainLayersOutput struct {
	Version string               `json:"version"`
	Command string               `json:"command"`
	Spec    specSourceRef        `json:"spec"`
	Sources []explainLayerSource `json:"sources"`
}

func runExplainLayers(cmd *cobra.Command, input string, jsonOut bool) error {
	var history spec.LayerHistory
	_, bundle, err := loadProjectWithLoader(".", func(layers []spec.Layer) (spec.Bundle, error) {
		b, h, err := spec.LoadLayeredWithHistory(layers)
		history = h
		return b, err
	})
	if err != nil {
		return err
	}
	candidates := make([]spec.Entry, 0, len(history))
	for _, candidate := range history {
		candidates = append(candidates, candidate.Entry)
	}
	selected, err := findLayerSpecEntry(input, candidates)
	if err != nil {
		return err
	}
	report := explainLayersOutput{Version: "1", Command: "explain", Sources: []explainLayerSource{}}
	for _, e := range bundle.All() {
		if e.Kind == selected.Kind && e.Name == selected.Name {
			report.Spec = entrySourceRef(e)
			break
		}
	}
	for _, candidate := range history {
		if candidate.Entry.Kind == selected.Kind && candidate.Entry.Name == selected.Name {
			report.Sources = append(report.Sources, explainLayerSource{Source: entrySourceRef(candidate.Entry), Operation: candidate.Operation, Role: candidate.Role})
		}
	}
	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "%s %s\n", selected.Kind, selected.Name)
	for _, candidate := range history {
		if candidate.Entry.Kind == selected.Kind && candidate.Entry.Name == selected.Name {
			_, _ = fmt.Fprintf(out, "  %s [%s] %s (%s)\n", entrySourceText(candidate.Entry), candidate.Entry.Layer, candidate.Operation, candidate.Role)
		}
	}
	for _, e := range bundle.All() {
		if e.Kind == selected.Kind && e.Name == selected.Name {
			_, _ = fmt.Fprintf(out, "effective source: %s\n", entrySourceText(e))
			break
		}
	}
	return nil
}

func findLayerSpecEntry(input string, candidates []spec.Entry) (spec.Entry, error) {
	selected, err := findSpecEntry(input, spec.NewBundle(candidates))
	if err == nil {
		return selected, nil
	}
	// Source aliases keep their lexical paths, but absolute selectors can use the real path.
	info, statErr := os.Stat(input)
	if statErr == nil {
		for _, candidate := range candidates {
			sourceInfo, sourceErr := os.Stat(candidate.Path)
			if sourceErr == nil && os.SameFile(info, sourceInfo) {
				return candidate, nil
			}
		}
	}
	return spec.Entry{}, err
}
