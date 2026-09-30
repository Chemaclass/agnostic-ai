package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters/codex"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

func collectCodexHookTrust(cfg *config.Config, targets []string) []codex.HookTrustFinding {
	if len(targets) == 0 {
		targets = cfg.Targets
	}
	if !slices.Contains(targets, "codex") {
		return nil
	}
	project := codex.HooksFilePath(cfg)
	paths := []string{project}
	user, err := codex.UserHooksPath()
	if err != nil {
		return []codex.HookTrustFinding{{Path: project, Status: "unknown", Problem: err.Error()}}
	}
	paths = append(paths, user)
	var findings []codex.HookTrustFinding
	seen := map[string]bool{}
	for _, path := range paths {
		absolute, err := filepath.Abs(path)
		if err != nil {
			findings = append(findings, codex.HookTrustFinding{Path: path, Status: "unknown", Problem: err.Error()})
			continue
		}
		if seen[absolute] {
			continue
		}
		seen[absolute] = true
		body, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			findings = append(findings, codex.HookTrustFinding{Path: path, Status: "unknown", Problem: err.Error()})
			continue
		}
		inactive, err := codex.HookTrustFindings(path, body)
		if err != nil {
			findings = append(findings, codex.HookTrustFinding{Path: path, Status: "unknown", Problem: err.Error()})
			continue
		}
		for _, finding := range inactive {
			if !slices.Contains(findings, finding) {
				findings = append(findings, finding)
			}
		}
	}
	return findings
}

func reportCodexHookTrust(cmd *cobra.Command, findings []codex.HookTrustFinding) {
	if len(findings) == 0 {
		return
	}
	cmd.Println()
	cmd.Println("Codex hook trust:")
	for _, finding := range findings {
		cmd.Printf("  ! %s\n", finding)
	}
}

func codexHookTrustErr(findings []codex.HookTrustFinding) error {
	count := 0
	for _, finding := range findings {
		if finding.Status != "disabled" {
			count++
		}
	}
	if count == 0 {
		return nil
	}
	return fmt.Errorf("%d Codex hook trust finding(s); open /hooks in Codex to review hook status", count)
}
