package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

type contextSize struct {
	Words int `json:"words"`
	Bytes int `json:"bytes"`
}

type contextContribution struct {
	Source   string `json:"source"`
	Category string `json:"category"`
	Load     string `json:"load"`
	contextSize
}

type explainContextOutput struct {
	Version       string                `json:"version"`
	Command       string                `json:"command"`
	Target        string                `json:"target"`
	File          string                `json:"file,omitempty"`
	Note          string                `json:"note"`
	Startup       contextSize           `json:"startup"`
	FileScope     contextSize           `json:"file_scope"`
	Contributions []contextContribution `json:"contributions"`
}

const contextEstimateNote = "Estimates from generated instruction layers and spec text, using lint's word accounting, not model tokens or live context. " +
	"On-demand bodies are excluded from startup totals. Runtime context, user-owned files, external imports, and host truncation are unknown."

func measuredContext(category, load, source, text string) contextContribution {
	return contextContribution{Source: filepath.ToSlash(source), Category: category, Load: load, contextSize: contextSize{Words: wordsIn(text), Bytes: len(text)}}
}

func (l *sessionLoad) addEntryContext(entries []spec.Entry, kind string) {
	for _, e := range entries {
		desc, _ := adapters.ResolveMeta(e.Meta, l.target)["description"].(string)
		l.context = append(l.context, measuredContext(kind+" discovery", "startup", adapters.EntrySourcePath(e), desc))
		l.context = append(l.context, measuredContext(kind+" body", "on-demand", adapters.EntrySourcePath(e), e.Body))
	}
}

// addFileContext counts disjoint spans from the rendered document. Source
// identities come from the renderer, never comments in an authored body.
func (l *sessionLoad) addFileContext(output, text string, layers []instructionLayer) {
	remaining := contextSize{Words: wordsIn(text), Bytes: len(text)}
	cursor := 0
	original := text
	copied := []header.CopiedRange{{Length: len(text)}}
	if len(layers) > 0 && layers[0].Rendering != nil {
		original = layers[0].Rendering.Text
		copied = layers[0].Rendering.Copied
	}
	renderedSpan := func(start, end int) string {
		var out strings.Builder
		for _, r := range copied {
			left, right := max(start, r.Input), min(end, r.Input+r.Length)
			stop := min(r.Output+right-r.Input, len(text))
			begin := r.Output + left - r.Input
			if left < right && begin < stop {
				out.WriteString(text[begin:stop])
			}
		}
		return out.String()
	}
	for _, layer := range layers {
		sources := layer.Sources
		if sources == nil {
			source := filepath.Join(".agnostic-ai", strings.TrimSuffix(layer.Name, " (Claude Code only)"))
			if layer.Name == "shared memory" {
				source = ".agnostic-ai/memory"
			}
			sources = []instructionSource{{Path: source, Text: layer.Text}}
		}
		category := "entry-point layer"
		if layer.Name == "rules" || layer.Name == "reviews" {
			category = layer.Name + " section"
		}
		for _, source := range sources {
			span := strings.TrimRight(source.Text, "\n")
			if span == "" {
				continue
			}
			prefixTrim := 0
			at := strings.Index(original[cursor:], span)
			if at < 0 {
				trimmed := strings.TrimSpace(span)
				prefixTrim = strings.Index(span, trimmed)
				span = trimmed
				if span == "" {
					continue
				}
				at = strings.Index(original[cursor:], span)
			}
			if at < 0 {
				continue
			}
			sourceStart := cursor + at
			cursor = sourceStart + len(span)
			if category == "entry-point layer" {
				var parent strings.Builder
				end := 0
				for _, imported := range source.Imports {
					start, stop := imported.Start-prefixTrim, imported.End-prefixTrim
					if start < end || stop > len(span) {
						continue
					}
					parent.WriteString(renderedSpan(sourceStart+end, sourceStart+start))
					c := measuredContext("inline import", "startup", imported.Path, renderedSpan(sourceStart+start, sourceStart+stop))
					l.context = append(l.context, c)
					remaining.Words -= c.Words
					remaining.Bytes -= c.Bytes
					end = stop
				}
				parent.WriteString(renderedSpan(sourceStart+end, sourceStart+len(span)))
				span = parent.String()
			} else {
				span = renderedSpan(sourceStart, sourceStart+len(span))
			}
			c := measuredContext(category, "startup", source.Path, span)
			l.context = append(l.context, c)
			remaining.Words -= c.Words
			remaining.Bytes -= c.Bytes
		}
	}
	if remaining.Words != 0 || remaining.Bytes != 0 {
		l.context = append(l.context, contextContribution{Source: filepath.ToSlash(output), Category: "generated framing", Load: "startup", contextSize: remaining})
	}
}

func explainContext(cfg *config.Config, b spec.Bundle, target, file string) (explainContextOutput, error) {
	if !slices.Contains(cfg.Targets, target) {
		return explainContextOutput{}, fmt.Errorf("explain --context: %s is not a configured target", target)
	}
	loads, err := projectSessionLoads(cfg, projectKindSupport(cfg), b)
	if err != nil {
		return explainContextOutput{}, err
	}
	report := explainContextOutput{Version: "1", Command: "explain", Target: target, Note: contextEstimateNote, Contributions: []contextContribution{}}
	for _, load := range loads {
		if load.target == target {
			report.Contributions = append(report.Contributions, load.context...)
		}
	}
	startupRules := map[string]bool{}
	for _, c := range report.Contributions {
		if c.Load == "startup" {
			startupRules[c.Source] = true
		}
	}
	outputs, err := contextRuleOutputs(cfg, b, target, startupRules)
	if err != nil {
		return explainContextOutput{}, err
	}
	var scope explainFileOutput
	if file != "" {
		scope, err = explainFile(file, target, cfg, b, ".")
		if err != nil {
			return explainContextOutput{}, err
		}
	}
	for _, r := range b.For(target).Rules {
		source := filepath.ToSlash(adapters.EntrySourcePath(r))
		if !startupRules[source] && len(outputs[source]) > 0 {
			report.Contributions = append(report.Contributions, measuredContext("conditional rule body", "on-demand", source, r.Body))
		}
	}
	if file != "" {
		if !slices.Contains(fileContextTargets, target) {
			return explainContextOutput{}, fmt.Errorf("explain --file: target %q is unsupported", target)
		}
		report.File = scope.File
		matched := map[string]bool{}
		for _, item := range scope.Instructions {
			if item.Status == contextMatch && slices.Contains(outputs[item.Source], filepath.ToSlash(item.Output)) {
				matched[item.Source] = true
			}
		}
		for i := range report.Contributions {
			c := &report.Contributions[i]
			if c.Category == "conditional rule body" && matched[c.Source] {
				c.Category, c.Load = "scoped rule", "file-scope"
			}
		}
	}
	for _, c := range report.Contributions {
		if c.Load == "startup" {
			report.Startup.Words += c.Words
			report.Startup.Bytes += c.Bytes
		}
		if c.Load == "file-scope" {
			report.FileScope.Words += c.Words
			report.FileScope.Bytes += c.Bytes
		}
	}
	slices.SortFunc(report.Contributions, func(a, b contextContribution) int {
		if contextLoadRank(a.Load) != contextLoadRank(b.Load) {
			return contextLoadRank(a.Load) - contextLoadRank(b.Load)
		}
		if a.Words != b.Words {
			return b.Words - a.Words
		}
		if a.Bytes != b.Bytes {
			return b.Bytes - a.Bytes
		}
		if a.Source != b.Source {
			return strings.Compare(a.Source, b.Source)
		}
		return strings.Compare(a.Category, b.Category)
	})
	return report, nil
}

func contextLoadRank(load string) int {
	switch load {
	case "startup":
		return 0
	case "file-scope":
		return 1
	default:
		return 2
	}
}

func runExplainContext(cmd *cobra.Command, target, file string, jsonOut bool) error {
	cfg, b, err := loadProject(".")
	if err != nil {
		return err
	}
	report, err := explainContext(cfg, b, target, file)
	if err != nil {
		return err
	}
	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "%s estimated context\n%s\n\nStartup: %d words, %d bytes\nFile scope (additional): %d words, %d bytes\n", target, report.Note, report.Startup.Words, report.Startup.Bytes, report.FileScope.Words, report.FileScope.Bytes)
	if report.File != "" {
		_, _ = fmt.Fprintf(out, "File: %s\n", report.File)
	}
	for _, c := range report.Contributions {
		_, _ = fmt.Fprintf(out, "  [%s] %d words, %d bytes: %s (%s)\n", c.Load, c.Words, c.Bytes, c.Source, c.Category)
	}
	return nil
}
