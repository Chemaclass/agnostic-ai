package cli

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/adapters/codex"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

type doctorTrustCheck struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

func validateDoctorChecks(checks string) error {
	if checks != "all" && checks != "project" {
		return fmt.Errorf("unknown doctor --checks %q: use all or project", checks)
	}
	return nil
}

func doctorTrustForScope(cfg *config.Config, targets []string, scope string) ([]codex.HookTrustFinding, *doctorTrustCheck) {
	if scope == "project" {
		return nil, &doctorTrustCheck{Status: "skipped", Reason: "project scope skips local runtime trust"}
	}
	return collectCodexHookTrust(cfg, targets), nil
}
