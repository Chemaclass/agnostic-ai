package cli

import (
	"errors"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// lintTargetRefusals flags the first spec or setting each selected target
// refuses, which stops sync for every target (LINT053, error). The check
// is the adapter's own, so each rule lives in one place. User scope has
// no project config and is not checked.
func lintTargetRefusals(cfg *config.Config, targets []string, b spec.Bundle, configPath string) []lintFinding {
	if cfg == nil {
		return nil
	}
	var out []lintFinding
	for _, target := range targets {
		err := adapters.RefuseSpecs(target, b, cfg)
		if err == nil {
			continue
		}
		path, reason := configPath, err
		if refusal, ok := errors.AsType[*adapters.Refusal](err); ok {
			reason = refusal.Err
			if refusal.Path != "" {
				path = refusal.Path
			}
		}
		message := reason.Error()
		if !strings.HasPrefix(message, target+":") && !strings.HasPrefix(message, target+" ") {
			message = target + ": " + message
		}
		out = append(out, lintFinding{
			Code: "LINT053", Severity: lintError, Path: path,
			Message: message + "; sync stops for every tool until this is fixed",
		})
	}
	return out
}
