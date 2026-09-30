package cli

import (
	"fmt"
	"io"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// explainModel is the model and effort a spec resolves to on one target.
// An empty Model leaves the tool's default; a nil Effort writes none.
type explainModel struct {
	Target string `json:"target"`
	Model  string `json:"model,omitempty"`
	Effort any    `json:"effort,omitempty"`
}

// explainModels resolves the spec's `model` and `effort` for each target
// in contributions, in order, the way adapters read them. Nil when the
// spec sets neither.
func explainModels(e spec.Entry, contributions []contribution) []explainModel {
	_, hasModel := e.Meta["model"]
	_, hasEffort := e.Meta["effort"]
	if !hasModel && !hasEffort {
		return nil
	}
	var out []explainModel
	seen := map[string]bool{}
	for _, c := range contributions {
		if seen[c.Target] {
			continue
		}
		seen[c.Target] = true
		resolved := adapters.ResolveMeta(e.Meta, c.Target)
		model, _ := resolved["model"].(string)
		out = append(out, explainModel{Target: c.Target, Model: model, Effort: resolved["effort"]})
	}
	return out
}

func printExplainModels(out io.Writer, tier string, models []explainModel) {
	if len(models) == 0 {
		return
	}
	header := "model:"
	if tier != "" {
		header = fmt.Sprintf("model (tier %s):", tier)
	}
	_, _ = fmt.Fprintf(out, "\n%s\n", header)
	for _, m := range models {
		model := m.Model
		if model == "" {
			model = "tool default"
		}
		if m.Effort != nil {
			model += fmt.Sprintf(", effort %v", m.Effort)
		}
		_, _ = fmt.Fprintf(out, "  [%s] %s\n", m.Target, model)
	}
}
