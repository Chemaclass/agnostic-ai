package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
)

type resultSource struct {
	Kind     string   `json:"kind"`
	URL      string   `json:"url"`
	Evidence artifact `json:"evidence"`
	Outcome  outcome  `json:"outcome"`
}
type resultTiming struct {
	Queued  *int64 `json:"queued"`
	Start   *int64 `json:"start"`
	End     *int64 `json:"end"`
	Attempt int    `json:"attempt,omitempty"`
}
type targetResult struct {
	Version int            `json:"version"`
	RunID   string         `json:"run_id"`
	Target  string         `json:"target"`
	Owner   string         `json:"owner"`
	Outcome outcome        `json:"outcome"`
	Sources []resultSource `json:"sources"`
	Timing  *resultTiming  `json:"timing,omitempty"`
}

func (a app) ingest(m *manifest, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	var r targetResult
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err = dec.Decode(&r); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	var extra any
	if err = dec.Decode(&extra); err != io.EOF {
		return "", fmt.Errorf("parse %s: trailing JSON", path)
	}
	if err = a.validateEvidence(m, m.LockBaseline); err != nil {
		return "", err
	}
	if err = validateTargetResult(m, &r); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	for _, prior := range m.Results {
		if prior.Target == r.Target {
			if !reflect.DeepEqual(prior, r) {
				return "", fmt.Errorf("%s: conflicting accepted result", r.Target)
			}
			return resultSummary(m), nil
		}
	}
	var assigned *assignment
	for i := range m.Assignments {
		if m.Assignments[i].Target == r.Target {
			if assigned != nil {
				return "", fmt.Errorf("%s: duplicate assignment", r.Target)
			}
			assigned = &m.Assignments[i]
		}
	}
	if assigned == nil || assigned.Owner != r.Owner || assigned.Outcome != nil {
		return "", fmt.Errorf("%s: requires matching unfinished assignment", r.Target)
	}
	byKey := map[string]outcome{}
	for _, s := range r.Sources {
		byKey[s.Kind+"\t"+s.URL] = s.Outcome
	}
	for i := range m.Sources {
		s := &m.Sources[i]
		if s.Target != r.Target {
			continue
		}
		if s.Outcome != nil {
			return "", fmt.Errorf("%s: source already recorded", s.key())
		}
		out := byKey[s.Kind+"\t"+s.URL]
		s.Outcome = &out
	}
	assigned.Outcome = &r.Outcome
	m.Results = append(m.Results, r)
	if err = save(m); err != nil {
		return "", err
	}
	return resultSummary(m), nil
}
func validateTargetResult(m *manifest, r *targetResult) error {
	if r.Version != 1 || r.RunID != m.RunID || !slices.Contains(m.Targets, r.Target) || strings.TrimSpace(r.Owner) == "" {
		return errors.New("invalid result version, run, target or owner")
	}
	if err := validateOutcome(m, &r.Outcome); err != nil {
		return err
	}
	report, err := getArtifact(m.Directory, checkpointReport(r.Target))
	if err != nil {
		return err
	}
	marker, err := os.ReadFile(filepath.Join(m.Directory, checkpointMarker(r.Target)))
	if err != nil {
		return fmt.Errorf("checkpoint %s: %w", r.Target, err)
	}
	if r.Outcome.Artifact != report || string(marker) != report.SHA256+"  "+report.Path+"\n" {
		return errors.New("result must reference an unchanged checkpointed target report")
	}
	deep := slices.Contains(m.BuiltinDeep, r.Target)
	if deep && r.Outcome.Disposition == "fast-path" {
		return errors.New("changed built-ins require deep review")
	}
	expected := map[string]source{}
	for _, s := range m.Sources {
		if s.Target == r.Target {
			key := s.Kind + "\t" + s.URL
			if _, exists := expected[key]; exists {
				return fmt.Errorf("%s: duplicate captured source", key)
			}
			expected[key] = s
		}
	}
	seen := map[string]bool{}
	for _, s := range r.Sources {
		key := s.Kind + "\t" + s.URL
		current, ok := expected[key]
		if !ok || seen[key] {
			return fmt.Errorf("%s: unknown or duplicate source", key)
		}
		seen[key] = true
		if current.Fetch == nil || len(current.Fetch.Row) != 10 || s.Evidence != current.Fetch.Evidence {
			return fmt.Errorf("%s: evidence differs from capture", key)
		}
		if err := checkArtifact(m.Directory, s.Evidence); err != nil {
			return err
		}
		if err := validateOutcome(m, &s.Outcome); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		if current.Fetch.Row[7] == "failed" && s.Outcome.Disposition != "research-limit" {
			return fmt.Errorf("%s: failed fetch requires research-limit", key)
		}
		if deep && s.Outcome.Disposition == "fast-path" {
			return fmt.Errorf("%s: changed built-ins require deep review", key)
		}
	}
	if len(seen) != len(expected) {
		return errors.New("result does not cover every target source")
	}
	if r.Timing != nil {
		v := r.Timing
		var prev *int64
		for _, n := range []*int64{v.Queued, v.Start, v.End} {
			if n == nil {
				continue
			}
			if *n < 0 || (prev != nil && *n < *prev) {
				return errors.New("invalid observed timing order")
			}
			prev = n
		}
		if v.Attempt < 0 {
			return errors.New("negative attempt")
		}
	}
	return nil
}
func resultSummary(m *manifest) string {
	var b strings.Builder
	b.WriteString("Target | Owner | Outcome | Reviewed | Fast path | Research limit | Queued | Start | End\n--- | --- | --- | --- | --- | --- | --- | --- | ---\n")
	epoch := func(n *int64) string {
		if n == nil {
			return "unknown"
		}
		return fmt.Sprint(*n)
	}
	for _, r := range m.Results {
		timing := r.Timing
		if timing == nil {
			timing = &resultTiming{}
		}
		counts := map[string]int{}
		for _, source := range r.Sources {
			counts[source.Outcome.Disposition]++
		}
		fmt.Fprintf(&b, "%s | %s | %s | %d | %d | %d | %s | %s | %s\n", r.Target, r.Owner, r.Outcome.Disposition, counts["reviewed"], counts["fast-path"], counts["research-limit"], epoch(timing.Queued), epoch(timing.Start), epoch(timing.End))
	}
	return b.String()
}

func resultTemplate(m *manifest, target string) (string, error) {
	if !slices.Contains(m.Targets, target) {
		return "", fmt.Errorf("%s: not a requested target", target)
	}
	report, err := getArtifact(m.Directory, checkpointReport(target))
	if err != nil {
		return "", err
	}
	r := targetResult{Version: 1, RunID: m.RunID, Target: target, Outcome: outcome{Artifact: report}}
	for _, as := range m.Assignments {
		if as.Target == target {
			r.Owner = as.Owner
		}
	}
	if r.Owner == "" {
		return "", fmt.Errorf("%s: target has no owner", target)
	}
	for _, s := range m.Sources {
		if s.Target == target {
			if s.Fetch == nil {
				return "", fmt.Errorf("%s: source not captured", s.key())
			}
			r.Sources = append(r.Sources, resultSource{Kind: s.Kind, URL: s.URL, Evidence: s.Fetch.Evidence, Outcome: outcome{Artifact: report}})
		}
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("target result: %w", err)
	}
	return string(b) + "\n", nil
}

func validateStoredResults(m *manifest) error {
	seen := map[string]bool{}
	for i := range m.Results {
		r := &m.Results[i]
		if seen[r.Target] {
			return fmt.Errorf("%s: duplicate stored result", r.Target)
		}
		seen[r.Target] = true
		if err := validateTargetResult(m, r); err != nil {
			return fmt.Errorf("stored result %s: %w", r.Target, err)
		}
		count := 0
		for _, as := range m.Assignments {
			if as.Target == r.Target {
				count++
				if as.Owner != r.Owner || as.Outcome == nil || !reflect.DeepEqual(*as.Outcome, r.Outcome) {
					return fmt.Errorf("%s: stored result differs from assignment", r.Target)
				}
			}
		}
		if count != 1 {
			return fmt.Errorf("%s: stored result requires one assignment", r.Target)
		}
		for _, rs := range r.Sources {
			current := findSource(m.Sources, strings.Join([]string{r.Target, rs.Kind, rs.URL}, "\t"))
			if current == nil || current.Outcome == nil || !reflect.DeepEqual(*current.Outcome, rs.Outcome) {
				return fmt.Errorf("%s: stored result differs from source coverage", r.Target)
			}
		}
	}
	return nil
}
