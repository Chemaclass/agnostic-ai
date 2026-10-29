package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Checkpoints are written by auditors while the coordinator may hold the
// run's write guard, so they live beside the manifest instead of inside it.
func checkpointMarker(target string) string { return target + ".done" }

func checkpointReport(target string) string { return target + ".md" }

func (a app) checkpoint(args []string) (string, error) {
	if len(args) != 2 {
		return "", errors.New("checkpoint requires run and target")
	}
	m, err := load(args[0])
	if err != nil {
		return "", err
	}
	if m.Finalized != "" {
		return "", errors.New("run already finalized")
	}
	target := args[1]
	if !slices.Contains(m.Targets, target) {
		return "", fmt.Errorf("%s: not a requested target", target)
	}
	report, err := getArtifact(m.Directory, checkpointReport(target))
	if err != nil {
		return "", err
	}
	line := report.SHA256 + "  " + report.Path + "\n"
	return "", atomicWrite(filepath.Join(m.Directory, checkpointMarker(target)), []byte(line))
}

func checkpointState(m *manifest, target string) (string, error) {
	for _, as := range m.Assignments {
		if as.Target == target && as.Outcome != nil {
			return "completed", nil
		}
	}
	marker := filepath.Join(m.Directory, checkpointMarker(target))
	b, err := os.ReadFile(marker)
	if errors.Is(err, os.ErrNotExist) {
		return "pending", nil
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", marker, err)
	}
	report, err := getArtifact(m.Directory, checkpointReport(target))
	if err != nil || string(b) != report.SHA256+"  "+report.Path+"\n" {
		return "stale", nil
	}
	return "done", nil
}

func (a app) checkpoints(args []string) (string, error) {
	pendingOnly := len(args) > 0 && args[0] == "--pending"
	if pendingOnly {
		args = args[1:]
	}
	if len(args) == 0 {
		return "", errors.New("checkpoints requires [--pending], a run directory and optional targets")
	}
	m, err := load(args[0])
	if err != nil {
		return "", err
	}
	targets := m.Targets
	if len(args) > 1 {
		targets = args[1:]
		for _, target := range targets {
			if !slices.Contains(m.Targets, target) {
				return "", fmt.Errorf("%s: not a requested target", target)
			}
		}
	}
	var rows, pending []string
	for _, target := range targets {
		state, stateErr := checkpointState(m, target)
		if stateErr != nil {
			return "", stateErr
		}
		rows = append(rows, target+"\t"+state)
		if state == "pending" || state == "stale" {
			pending = append(pending, target)
		}
	}
	if pendingOnly {
		return strings.Join(pending, " ") + "\n", nil
	}
	return strings.Join(rows, "\n") + "\n", nil
}
