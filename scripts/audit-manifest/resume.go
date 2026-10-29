package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const researchVersion = 1

type reuse struct {
	Target   string   `json:"target"`
	RunID    string   `json:"prior_run_id"`
	Manifest artifact `json:"prior_manifest"`
}

func (a app) resume(m *manifest, priorDir string) (string, error) {
	if m.RowsSHA256 == "" {
		return "", errors.New("resume requires a new run with fresh captured fetches")
	}
	if len(m.Assignments) != 0 || len(m.Reused) != 0 || m.ResumeFrom != "" {
		return "", errors.New("resume requires a fresh run without assignments")
	}
	for _, s := range m.Sources {
		if s.Outcome != nil {
			return "", errors.New("resume requires a fresh run without dispositions")
		}
	}
	if err := a.validateEvidence(m, m.LockBaseline); err != nil {
		return "", err
	}
	prior, err := load(priorDir)
	if err != nil {
		return "", err
	}
	if prior.RunID == m.RunID || prior.Directory == m.Directory {
		return "", errors.New("resume requires a distinct prior run")
	}
	guard := filepath.Join(prior.Directory, ".audit-writing")
	if err = os.Mkdir(guard, 0o700); err != nil {
		return "", fmt.Errorf("%s: prior run is busy or was interrupted: %w", guard, err)
	}
	defer func() { _ = os.Remove(guard) }()
	prior, err = load(priorDir)
	if err != nil {
		return "", err
	}
	if err = a.validateEvidence(prior, prior.LockBaseline); err != nil {
		return "", fmt.Errorf("prior run: %w", err)
	}
	if prior.Commit != m.Commit || prior.Context != m.Context || prior.BuiltinsSince != m.BuiltinsSince || prior.LockBaseline != m.LockBaseline || prior.SourceList != m.SourceList || prior.BuiltinsInventory.SHA256 != m.BuiltinsInventory.SHA256 || !equalSources(prior.Inventory, m.Inventory) || prior.ResearchVersion != m.ResearchVersion {
		return "", errors.New("prior research context differs; no research reused")
	}
	seen := map[string]bool{}
	for _, as := range prior.Assignments {
		if !slices.Contains(prior.Targets, as.Target) || seen[as.Target] || strings.TrimSpace(as.Owner) == "" {
			return "", errors.New("invalid prior assignment")
		}
		seen[as.Target] = true
		if as.Outcome != nil {
			if err = validateOutcome(prior, as.Outcome); err != nil {
				return "", err
			}
		}
	}
	for _, s := range prior.Sources {
		if s.Outcome != nil {
			if err = validateOutcome(prior, s.Outcome); err != nil {
				return "", err
			}
		}
	}
	for _, as := range prior.Assignments {
		if !slices.Contains(m.Targets, as.Target) || !reusableOutcome(prior, as.Target, as.Outcome) {
			continue
		}
		eligible := true
		for _, s := range m.Sources {
			if s.Target != as.Target {
				continue
			}
			previous := findSource(prior.Sources, s.key())
			if previous == nil || !reusableOutcome(prior, s.Target, previous.Outcome) || !sameEvidence(previous.Fetch, s.Fetch) || (previous.Outcome.Disposition == "fast-path" && slices.Contains(m.BuiltinDeep, s.Target)) {
				eligible = false
				break
			}
		}
		if !eligible || (as.Outcome.Disposition == "fast-path" && slices.Contains(m.BuiltinDeep, as.Target)) {
			continue
		}
		copied, copyErr := copyOutcome(m, prior, as.Outcome)
		if copyErr != nil {
			return "", copyErr
		}
		m.Assignments = append(m.Assignments, assignment{Target: as.Target, Owner: as.Owner, Outcome: copied})
		for i := range m.Sources {
			if m.Sources[i].Target != as.Target {
				continue
			}
			m.Sources[i].Outcome, err = copyOutcome(m, prior, findSource(prior.Sources, m.Sources[i].key()).Outcome)
			if err != nil {
				return "", err
			}
		}
		manifestArtifact, copyErr := copyArtifact(m, prior, artifact{Path: "audit.json"})
		if copyErr != nil {
			return "", copyErr
		}
		m.Reused = append(m.Reused, reuse{Target: as.Target, RunID: prior.RunID, Manifest: manifestArtifact})
	}
	m.ResumeFrom = prior.RunID
	if err = save(m); err != nil {
		return "", err
	}
	return fmt.Sprintf("reused %d completed targets; %d targets remain pending\n", len(m.Reused), len(m.Targets)-len(m.Reused)), nil
}

func findSource(sources []source, key string) *source {
	for i := range sources {
		if sources[i].key() == key {
			return &sources[i]
		}
	}
	return nil
}

func reusableOutcome(m *manifest, target string, out *outcome) bool {
	return out != nil && out.Disposition != "research-limit" && (out.Disposition != "fast-path" || !slices.Contains(m.BuiltinDeep, target))
}

func sameEvidence(a, b *fetch) bool {
	if a == nil || b == nil || a.Row[7] == "failed" || b.Row[7] == "failed" || a.ComparisonSHA256 != b.ComparisonSHA256 || a.Evidence.SHA256 != b.Evidence.SHA256 || a.Row[4] != b.Row[4] || a.Row[8] != b.Row[8] || len(a.Artifacts) != len(b.Artifacts) {
		return false
	}
	for i := range a.Artifacts {
		if a.Artifacts[i].SHA256 != b.Artifacts[i].SHA256 {
			return false
		}
	}
	return true
}

func copyArtifact(m, prior *manifest, entry artifact) (artifact, error) {
	clean := filepath.Clean(entry.Path)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return artifact{}, fmt.Errorf("%s: copied artifact escapes run directory", entry.Path)
	}
	actual, err := getArtifact(prior.Directory, entry.Path)
	if err != nil {
		return artifact{}, err
	}
	if entry.SHA256 != "" && entry != actual {
		return artifact{}, fmt.Errorf("%s: prior artifact changed", entry.Path)
	}
	b, err := os.ReadFile(filepath.Join(prior.Directory, entry.Path))
	if err != nil {
		return artifact{}, fmt.Errorf("%s: %w", entry.Path, err)
	}
	if digest(b) != actual.SHA256 {
		return artifact{}, fmt.Errorf("%s: prior artifact changed during copy", entry.Path)
	}
	path := filepath.Join("resumed", prior.RunID, clean)
	parent := m.Directory
	for _, component := range strings.Split(filepath.Dir(path), string(filepath.Separator)) {
		parent = filepath.Join(parent, component)
		info, statErr := os.Lstat(parent)
		if errors.Is(statErr, os.ErrNotExist) {
			break
		}
		if statErr != nil {
			return artifact{}, fmt.Errorf("%s: %w", parent, statErr)
		}
		if !info.IsDir() {
			return artifact{}, fmt.Errorf("%s: copied artifact parent must be a directory without symlinks", parent)
		}
	}
	if err = os.MkdirAll(filepath.Dir(filepath.Join(m.Directory, path)), 0o755); err != nil {
		return artifact{}, fmt.Errorf("%s: %w", path, err)
	}
	if err = atomicWrite(filepath.Join(m.Directory, path), b); err != nil {
		return artifact{}, err
	}
	return artifact{Path: filepath.ToSlash(path), SHA256: actual.SHA256}, nil
}

func copyOutcome(m, prior *manifest, out *outcome) (*outcome, error) {
	entry, err := copyArtifact(m, prior, out.Artifact)
	if err != nil {
		return nil, err
	}
	return &outcome{Disposition: out.Disposition, Reason: out.Reason, Artifact: entry}, nil
}
